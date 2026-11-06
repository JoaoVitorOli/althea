package scripts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/extensions/scripts"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestParseReadsMetadataInAnyCommentStyle(t *testing.T) {
	for _, src := range []string{
		"# @althea.title Say Hello\n# @althea.mode detail\n# @althea.icon 👋\n# @althea.keywords greet, oi\n",
		"// @althea.title Say Hello\n// @althea.mode detail\n// @althea.icon 👋\n// @althea.keywords greet, oi\n",
		"@echo off\r\nREM @althea.title Say Hello\r\n:: @althea.mode detail\r\n:: @althea.icon 👋\r\n:: @althea.keywords greet, oi\r\n",
		"-- @althea.title Say Hello\n-- @althea.mode DETAIL\n-- @althea.icon 👋\n-- @althea.keywords greet,oi\n",
	} {
		s, err := scripts.Parse(`C:\s\hello.ps1`, []byte(src))
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if s.Title != "Say Hello" || s.Mode != scripts.Detail || s.Icon.Text != "👋" ||
			strings.Join(s.Keywords, "|") != "greet|oi" {
			t.Errorf("%q gives %+v", src, s)
		}
	}
}

func TestParseArgumentsAndOptions(t *testing.T) {
	s, err := scripts.Parse("x.py", []byte(`#!/usr/bin/env python3
# @althea.title Search
# @althea.argument1 {"placeholder": "Query"}
# @althea.argument2 { "placeholder": "Token", "secure": true, "optional": true }
# @althea.argument4 Ignored, no third
# @althea.confirm true
# @althea.timeout 5s
# @althea.refresh 1s
`))
	if err == nil || !strings.Contains(err.Error(), "argument4") {
		t.Fatalf("err = %v, want argument4 refused", err)
	}
	if len(s.Arguments) != 2 || !s.Arguments[1].Secure || !s.Arguments[1].Optional || s.Arguments[0].Placeholder != "Query" {
		t.Errorf("arguments %+v", s.Arguments)
	}
	if !s.Confirm || s.Timeout != 5*time.Second || s.Refresh != 10*time.Second {
		t.Errorf("confirm %v, timeout %v, refresh %v (at least 10s)", s.Confirm, s.Timeout, s.Refresh)
	}
	if _, err := scripts.Parse("x.sh", []byte("# @althea.mode toast\n")); err == nil {
		t.Error("a script without a title is accepted")
	}
	if _, err := scripts.Parse("x.sh", []byte("# @althea.title X\n# @althea.mode loud\n")); err == nil {
		t.Error("an unknown mode is accepted")
	}
}

// script writes a script of the system's shell printing lines, with metadata. A line starting
// with "$ " is a command rather than a line to print.
func script(t *testing.T, dir, name string, meta []string, lines ...string) string {
	t.Helper()
	var b strings.Builder
	ext, comment, nl := ".sh", "#", "\n"
	if runtime.GOOS == "windows" {
		ext, comment, nl = ".cmd", "::", "\r\n"
		b.WriteString("@echo off" + nl)
	}
	for _, m := range meta {
		b.WriteString(comment + " @althea." + m + nl)
	}
	for _, l := range lines {
		if cmd, ok := strings.CutPrefix(l, "$ "); ok {
			b.WriteString(cmd + nl)
			continue
		}
		b.WriteString("echo " + l + nl)
	}
	path := filepath.Join(dir, name+ext)
	if err := os.WriteFile(path, []byte(b.String()), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func newExt(t *testing.T) (*scripts.Extension, string) {
	t.Helper()
	dir := t.TempDir()
	return scripts.New(scripts.Platform{Dir: dir, Command: command}), dir
}

// command stops a script's whole tree when cancelled, as the host's does, so that nothing
// holds the test's folder after it.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if runtime.GOOS == "windows" {
		cmd.Cancel = func() error {
			_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
			return cmd.Process.Kill()
		}
	}
	return cmd
}

func TestItemsAreTheScriptsWithMetadata(t *testing.T) {
	e, dir := newExt(t)
	script(t, dir, "hello", []string{"title Say Hello"}, "hi")
	script(t, dir, "_helper", []string{"title Hidden"}, "no")
	script(t, dir, "plain", nil, "no metadata")
	script(t, dir, "broken", []string{"mode toast"}, "no title")
	ctx := sdktest.New(t, e, "en")
	items, err := e.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "Say Hello" || items[0].Accessory != "Script" {
		t.Fatalf("items %+v", items)
	}
}

func run(t *testing.T, e *scripts.Extension, ctx *sdktest.Context) {
	t.Helper()
	items, err := e.Items(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("items %v, %v", items, err)
	}
	if err := items[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestToastModeShowsTheLastLine(t *testing.T) {
	e, dir := newExt(t)
	script(t, dir, "hello", []string{"title Say Hello"}, "first", "Olá, mundo")
	ctx := sdktest.New(t, e, "en")
	run(t, e, ctx)
	if len(ctx.Toasts) != 1 || ctx.Toasts[0] != "Olá, mundo" {
		t.Errorf("toasts %q", ctx.Toasts)
	}
}

func TestDetailAndListModesPushViews(t *testing.T) {
	e, dir := newExt(t)
	path := script(t, dir, "show", []string{"title Show", "mode detail"}, "one", "two")
	ctx := sdktest.New(t, e, "en")
	run(t, e, ctx)
	d, ok := ctx.Pushed[0].(*sdk.Detail)
	if !ok || !strings.Contains(d.Markdown, "one\ntwo") || !strings.HasPrefix(d.Markdown, "```") {
		t.Fatalf("pushed %#v", ctx.Pushed)
	}

	_ = os.Remove(path)
	script(t, dir, "lines", []string{"title Lines", "mode list"}, "alpha", "beta")
	ctx = sdktest.New(t, e, "en")
	run(t, e, ctx)
	l, ok := ctx.Pushed[0].(*sdk.List)
	if !ok || len(l.Items) != 2 || l.Items[1].Title != "beta" {
		t.Fatalf("pushed %#v", ctx.Pushed)
	}
	if err := l.Items[1].Actions[0].Run(ctx); err != nil || ctx.Copied[0] != "beta" {
		t.Errorf("copy: %v, copied %q", err, ctx.Copied)
	}
}

func TestSilentModeHidesAndFailuresShowStderr(t *testing.T) {
	e, dir := newExt(t)
	path := script(t, dir, "quiet", []string{"title Quiet", "mode silent"}, "ignored")
	ctx := sdktest.New(t, e, "en")
	run(t, e, ctx)
	if ctx.Hidden != 1 || len(ctx.Toasts) != 0 {
		t.Errorf("hidden %d, toasts %q", ctx.Hidden, ctx.Toasts)
	}

	_ = os.Remove(path)
	exit := "$ exit 3"
	if runtime.GOOS == "windows" {
		exit = "$ exit /b 3"
	}
	script(t, dir, "fail", []string{"title Fail"}, "$ echo disk full 1>&2", exit)
	ctx = sdktest.New(t, e, "en")
	items, _ := e.Items(ctx)
	err := items[0].Actions[0].Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("err = %v, want stderr's last line", err)
	}
}

func TestArgumentsAreAskedInAForm(t *testing.T) {
	e, dir := newExt(t)
	arg := "$1"
	if runtime.GOOS == "windows" {
		arg = "%~1"
	}
	script(t, dir, "greet", []string{"title Greet", `argument1 {"placeholder": "Name"}`}, "Hello "+arg)
	ctx := sdktest.New(t, e, "en")
	run(t, e, ctx)
	f, ok := ctx.Pushed[0].(*sdk.Form)
	if !ok || len(f.Fields) != 1 || f.Fields[0].Title != "Name" || !f.Fields[0].Required {
		t.Fatalf("pushed %#v", ctx.Pushed)
	}
	if err := f.Submit(ctx, sdk.Values{"1": "Ada Lovelace"}); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Toasts) != 1 || ctx.Toasts[0] != "Hello Ada Lovelace" {
		t.Errorf("toasts %q", ctx.Toasts)
	}
}

func TestTimeoutStopsAScript(t *testing.T) {
	e, dir := newExt(t)
	wait := "$ sleep 5"
	if runtime.GOOS == "windows" {
		wait = "$ ping -n 6 127.0.0.1 >nul"
	}
	script(t, dir, "slow", []string{"title Slow", "timeout 300ms"}, "start", wait)
	ctx := sdktest.New(t, e, "en")
	items, _ := e.Items(ctx)
	start := time.Now()
	err := items[0].Actions[0].Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("err = %v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
}

func TestPowerShellScriptsPrintUTF8(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell is the Windows shell")
	}
	e, dir := newExt(t)
	src := "# @althea.title Acentos\nparam($name)\nWrite-Output \"ação $name\"\n"
	if err := os.WriteFile(filepath.Join(dir, "acentos.ps1"), []byte("\uFEFF"+src), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := scripts.Parse(filepath.Join(dir, "acentos.ps1"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.Run(t.Context(), s, []string{"d'água"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.Stdout); got != "ação d'água" {
		t.Errorf("output %q", got)
	}
}

func TestCreateWritesARunnableScript(t *testing.T) {
	e, dir := newExt(t)
	ext := ".sh"
	if runtime.GOOS == "windows" {
		ext = ".cmd"
	}
	path, err := e.Create("Olá Mundo!", ext, scripts.Toast)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "olá-mundo"+ext {
		t.Errorf("path %s", path)
	}
	if _, err := e.Create("Olá Mundo!", ext, scripts.Toast); !os.IsExist(err) {
		t.Errorf("creating it twice: %v", err)
	}
	ctx := sdktest.New(t, e, "en")
	items, err := e.Items(ctx)
	if err != nil || len(items) != 1 || items[0].Title != "Olá Mundo!" {
		t.Fatalf("items %+v, %v", items, err)
	}
	if err := items[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Toasts) != 1 || !strings.HasPrefix(ctx.Toasts[0], "Hello from") {
		t.Errorf("toasts %q", ctx.Toasts)
	}
	_ = dir
}

func TestPowerShellArgumentsStayArguments(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell is the Windows shell")
	}
	e, dir := newExt(t)
	src := "# @althea.title Echo\nparam($name)\nWrite-Output \"[$name]\"\n"
	if err := os.WriteFile(filepath.Join(dir, "echo.ps1"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := scripts.Parse(filepath.Join(dir, "echo.ps1"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// Typographic quotes are quotes to PowerShell: unescaped, they would end the string.
	for _, arg := range []string{"x’; Write-Output PWNED; ’", "a'b", "‘‚‛"} {
		out, err := e.Run(t.Context(), s, []string{arg})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.Stdout); got != "["+arg+"]" {
			t.Errorf("argument %q came out as %q", arg, got)
		}
	}
}

func TestBatchArgumentsCannotRunCommands(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd is the Windows shell")
	}
	e, dir := newExt(t)
	path := script(t, dir, "echo", []string{"title Echo"}, "$ echo [%~1]")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := scripts.Parse(path, src)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{"a&echo PWNED", "a|more", "%PATH%", "a\"&echo PWNED&\"", "x\ny"} {
		if out, err := e.Run(t.Context(), s, []string{arg}); err == nil {
			t.Errorf("argument %q ran, giving %q", arg, out.Stdout)
		}
	}
	out, err := e.Run(t.Context(), s, []string{"Ada Lovelace"})
	if err != nil || strings.TrimSpace(out.Stdout) != "[Ada Lovelace]" {
		t.Errorf("plain argument: %q, %v", out.Stdout, err)
	}
}
