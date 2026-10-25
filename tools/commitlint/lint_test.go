package main

import (
	"strings"
	"testing"
)

func TestCheckAccepts(t *testing.T) {
	for _, msg := range []string{
		"feat: add calculator",
		"fix(search): rank exact prefixes first",
		"refactor!: restructure packages",
		"feat(platform/windows): add keyboard hook",
		"docs: spec\n\n- details\n- more",
		"chore: bump deps\n\n# Please enter the commit message\n",
		"Merge branch 'main' into feature",
		"fixup! feat: add calculator",
	} {
		if err := Check(msg); err != nil {
			t.Errorf("Check(%q) = %v, want nil", msg, err)
		}
	}
}

func TestCheckRejects(t *testing.T) {
	for _, tc := range []struct{ msg, want string }{
		{"", "empty"},
		{"# only a comment\n", "empty"},
		{"add calculator", "is not"},
		{"feature: add calculator", "is not"},
		{"feat:add calculator", "is not"},
		{"feat(Search): rank", "is not"},
		{"feat: " + strings.Repeat("x", 70), "characters long"},
		{"feat: add calculator\nbody right below", "blank line"},
	} {
		err := Check(tc.msg)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Check(%q) = %v, want an error containing %q", tc.msg, err, tc.want)
		}
	}
}
