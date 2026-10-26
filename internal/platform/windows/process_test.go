//go:build windows

package windows

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCommandStopsTheWholeTree(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := Command(ctx, os.Getenv("ComSpec"), "/d", "/c", "ping -n 30 127.0.0.1")
	var out strings.Builder
	cmd.Stdout = &out
	start := time.Now()
	_ = cmd.Run()
	// Without the tree killed, ping holds the output open for 30 seconds.
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("took %v", took)
	}
}
