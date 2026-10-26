package scripts

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// maxOutput bounds what is kept of a script's output; the rest is dropped.
const maxOutput = 1 << 20

// ErrNoInterpreter means Althea does not know how to run a file.
var ErrNoInterpreter = errors.New("no program to run it: give it a known extension or a #! line")

// command returns the program and arguments that run a script with args, and the environment
// it needs on top of Althea's.
func (e *Extension) command(path string, args []string) (name string, argv, env []string, err error) {
	look := e.p.LookPath
	if look == nil {
		look = exec.LookPath
	}
	first := func(names ...string) (string, error) {
		for _, n := range names {
			if p, err := look(n); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("%s is not installed or not on PATH", names[0])
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ps1":
		ps, err := first("pwsh", "powershell")
		if err != nil {
			return "", nil, nil, err
		}
		// Through -Command, so that the output is UTF-8 and the script's exit code comes out.
		call := "& " + psQuote(path)
		for _, a := range args {
			call += " " + psQuote(a)
		}
		script := "[Console]::OutputEncoding = [Text.Encoding]::UTF8; $ErrorActionPreference = 'Stop'; " +
			"try { " + call + " } catch { [Console]::Error.WriteLine($_); exit 1 }; exit $LASTEXITCODE"
		return ps, []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}, nil, nil
	case ".bat", ".cmd":
		// "call" first, so that cmd keeps the quotes around a path with spaces.
		return comspec(), append([]string{"/d", "/c", "call", path}, args...), nil, nil
	case ".py":
		py, err := first("python3", "python", "py")
		if err != nil {
			return "", nil, nil, err
		}
		return py, append([]string{path}, args...), []string{"PYTHONUTF8=1", "PYTHONIOENCODING=utf-8"}, nil
	case ".js", ".mjs", ".cjs":
		node, err := first("node")
		return node, append([]string{path}, args...), nil, err
	case ".rb":
		ruby, err := first("ruby")
		return ruby, append([]string{path}, args...), nil, err
	case ".sh":
		bash, err := first("bash")
		return bash, append([]string{path}, args...), nil, err
	case ".exe", ".com":
		return path, args, nil, nil
	}
	if prog, extra, ok := shebang(path); ok {
		p, err := first(prog)
		return p, append(append(extra, path), args...), nil, err
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(path); err == nil && fi.Mode()&0o111 != 0 {
			return path, args, nil, nil
		}
	}
	return "", nil, nil, ErrNoInterpreter
}

// comspec is the path of cmd.exe.
func comspec() string {
	if c := os.Getenv("ComSpec"); c != "" {
		return c
	}
	return "cmd.exe"
}

// psQuote quotes a string for PowerShell, in single quotes.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// shebang reads the program a script's #! line names, by its base name so that it is found
// on PATH on Windows too: "#!/usr/bin/env python3" gives python3.
func shebang(path string) (prog string, args []string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, false
	}
	defer func() { _ = f.Close() }()
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil && line == "" || !strings.HasPrefix(line, "#!") {
		return "", nil, false
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return "", nil, false
	}
	prog, args = filepath.Base(filepath.ToSlash(fields[0])), fields[1:]
	if prog == "env" {
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			args = args[1:] // env's own flags, as -S
		}
		if len(args) == 0 {
			return "", nil, false
		}
		prog, args = args[0], args[1:]
	}
	return prog, args, true
}

// Output is what a script printed.
type Output struct {
	Stdout string
	Stderr string
}

// Run runs a script with args until it ends or its timeout, in its own folder.
func (e *Extension) Run(ctx context.Context, s Script, args []string) (Output, error) {
	name, argv, env, err := e.command(s.Path, args)
	if err != nil {
		return Output{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	cmd := e.p.Command(ctx, name, argv...)
	cmd.Dir = filepath.Dir(s.Path)
	cmd.Env = append(os.Environ(), append(env, "ALTHEA=1")...)
	var stdout, stderr limited
	stdout.max, stderr.max = maxOutput, 64<<10
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// A process the script started may hold its output open after it is stopped.
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	out := Output{Stdout: clean(stdout.String()), Stderr: clean(stderr.String())}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return out, fmt.Errorf("%s did not finish within %s", s.Title, s.Timeout)
	case err != nil:
		if msg := lastLine(out.Stderr); msg != "" {
			return out, fmt.Errorf("%s: %s", s.Title, msg)
		}
		return out, fmt.Errorf("%s: %w", s.Title, err)
	}
	return out, nil
}

// limited keeps the first max bytes written to it.
type limited struct {
	bytes.Buffer
	max int
}

func (l *limited) Write(p []byte) (int, error) {
	if room := l.max - l.Len(); room > 0 {
		l.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// ansi matches terminal escape sequences, as of colors.
var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[\x20-\x2f]*[\x40-\x7e]`)

// clean drops colors and carriage returns from output.
func clean(s string) string {
	s = ansi.ReplaceAllString(s, "")
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// lines are the lines of output that are not blank.
func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimRight(l, " \t\r"); strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func lastLine(s string) string {
	ls := lines(s)
	if len(ls) == 0 {
		return ""
	}
	return strings.TrimSpace(ls[len(ls)-1])
}

func firstLine(s string) string {
	ls := lines(s)
	if len(ls) == 0 {
		return ""
	}
	return strings.TrimSpace(ls[0])
}
