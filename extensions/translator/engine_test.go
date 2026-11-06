package translator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fake engine of translator_test.go answers in this test binary too.
func TestAnEngineThatDiesSaysWhy(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "en-pt"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "en-pt", "config.yml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	g := &engine{command: func() (*exec.Cmd, error) {
		cmd := exec.CommandContext(context.Background(), os.Args[0])
		cmd.Env = append(os.Environ(), "ALTHEA_FAKE_BERGAMOT=1")
		cmd.Dir = dir
		return cmd, nil
	}}
	defer g.close()
	_, err := g.translate(t.Context(), "en-pt", "die")
	if err == nil || !strings.Contains(err.Error(), "Error: Model file is corrupt") {
		t.Errorf("err %v", err)
	}
}

func TestTailKeepsTheEnd(t *testing.T) {
	var tl tail
	_, _ = tl.Write([]byte(strings.Repeat("x", tailSize)))
	_, _ = tl.Write([]byte("\nfirst\nlast line\n\n"))
	if len(tl.buf) != tailSize || tl.last() != "last line" {
		t.Errorf("kept %d bytes, last %q", len(tl.buf), tl.last())
	}
	if (*tail)(nil).last() != "" {
		t.Error("a nil tail has words")
	}
}
