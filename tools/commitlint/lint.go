// Command commitlint checks that a commit message follows Conventional Commits, as the
// commit-msg git hook runs it: commitlint <message-file>.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// maxHeader is the longest header allowed, so it fits in one line of git log.
const maxHeader = 72

var header = regexp.MustCompile(
	`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9][a-z0-9/-]*\))?!?: \S`)

// generated are messages git writes itself, which are left alone.
var generated = regexp.MustCompile(`^(Merge|Revert|fixup!|squash!|amend!) `)

// Check reports what is wrong with a commit message, or nil.
func Check(msg string) error {
	first, rest, _ := strings.Cut(stripComments(msg), "\n")
	first = strings.TrimRight(first, " \r")
	switch {
	case first == "":
		return errors.New("empty commit message")
	case generated.MatchString(first):
		return nil
	case !header.MatchString(first):
		return fmt.Errorf("header %q is not \"<type>(<scope>): <description>\"; "+
			"types: feat fix docs style refactor perf test build ci chore revert", first)
	case len([]rune(first)) > maxHeader:
		return fmt.Errorf("header is %d characters long, at most %d", len([]rune(first)), maxHeader)
	case rest != "" && !strings.HasPrefix(rest, "\n") && strings.TrimSpace(rest) != "":
		return errors.New("leave a blank line between the header and the body")
	}
	return nil
}

// stripComments removes the lines git shows as comments in the editor.
func stripComments(msg string) string {
	lines := strings.Split(msg, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if !strings.HasPrefix(l, "#") {
			kept = append(kept, l)
		}
	}
	return strings.TrimLeft(strings.Join(kept, "\n"), "\n")
}

// resolve finds a message file given relative to the repository root, as git hooks pass it,
// while the tool runs from the tools directory.
func resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return path
	}
	return filepath.Join(strings.TrimSpace(string(top)), path)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: commitlint <message-file>")
		os.Exit(2)
	}
	msg, err := os.ReadFile(resolve(os.Args[1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, "commitlint:", err)
		os.Exit(2)
	}
	if err := Check(string(msg)); err != nil {
		fmt.Fprintln(os.Stderr, "commitlint:", err)
		os.Exit(1)
	}
}
