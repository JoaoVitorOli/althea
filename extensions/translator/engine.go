package translator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// idleAfter is how long the engine stays loaded after its last translation. A model takes a moment
// to load and tens of megabytes to keep, which a launcher that sits hidden should not hold on to.
const idleAfter = 5 * time.Minute

// errEngineStopped is the error of a request the engine died on.
var errEngineStopped = errors.New("the translation engine stopped")

// engine keeps the Bergamot subprocess: started on the first translation, stopped when idle or when
// the extension's service stops. It answers one request at a time, over its stdio; see
// tools/bergamot/main.cpp for the protocol.
type engine struct {
	// command makes the command that runs the engine, in the models folder.
	command func() (*exec.Cmd, error)

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	idle   *time.Timer
	closed bool
	// stderr keeps the end of what the engine says on stderr: why it stopped, when it did.
	stderr *tail
}

// translate translates text with the model of pair. Canceling ctx stops the engine, the only way
// to stop a translation on its way; the next one starts it again.
func (g *engine) translate(ctx context.Context, pair, text string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return "", errEngineStopped
	}
	if err := g.start(); err != nil {
		return "", err
	}
	if g.idle != nil {
		g.idle.Stop()
	}
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	stdin, stdout := g.stdin, g.stdout
	go func() {
		out, err := exchange(stdin, stdout, pair, text)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		if errors.Is(r.err, errEngineStopped) {
			said := g.stderr
			g.halt() // waits for the process, and so for all it said
			if why := said.last(); why != "" {
				return "", fmt.Errorf("%w: %s", errEngineStopped, why)
			}
			return "", r.err
		}
		g.idle = time.AfterFunc(idleAfter, g.stop)
		return r.text, r.err
	case <-ctx.Done():
		g.halt()
		<-done
		return "", ctx.Err()
	}
}

// maxAnswer is the longest answer the engine may give: far more than any text it is sent, and
// little enough that a confused engine cannot make Althea allocate the machine's memory.
const maxAnswer = 16 << 20

// exchange sends a request and reads its answer. A refusal is the engine's own error; anything else
// wrong with the pipe means it is gone.
func exchange(stdin io.Writer, stdout *bufio.Reader, pair, text string) (string, error) {
	if _, err := fmt.Fprintf(stdin, "%s %d\n%s", pair, len(text), text); err != nil {
		return "", errEngineStopped
	}
	header, err := stdout.ReadString('\n')
	if err != nil {
		return "", errEngineStopped
	}
	status, size, ok := strings.Cut(strings.TrimSpace(header), " ")
	n, atoiErr := strconv.Atoi(size)
	if !ok || atoiErr != nil || n < 0 || n > maxAnswer {
		return "", errEngineStopped
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(stdout, body); err != nil {
		return "", errEngineStopped
	}
	if status != "ok" {
		return "", fmt.Errorf("the translation engine failed: %s", body)
	}
	return string(body), nil
}

func (g *engine) start() error {
	if g.cmd != nil {
		return nil
	}
	cmd, err := g.command()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	g.stderr = &tail{}
	cmd.Stderr = g.stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start the translation engine: %w", err)
	}
	g.cmd, g.stdin, g.stdout = cmd, stdin, bufio.NewReader(stdout)
	return nil
}

// stop stops the engine, for going idle.
func (g *engine) stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.halt()
}

// close stops the engine for good, for quitting.
func (g *engine) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.halt()
	g.closed = true
}

// reopen lets the engine start again, as after the service restarts.
func (g *engine) reopen() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = false
}

// halt ends the process, if there is one, with the lock held.
func (g *engine) halt() {
	if g.idle != nil {
		g.idle.Stop()
		g.idle = nil
	}
	if g.cmd == nil {
		return
	}
	_ = g.stdin.Close()
	_ = g.cmd.Process.Kill()
	_ = g.cmd.Wait()
	g.cmd, g.stdin, g.stdout = nil, nil, nil
}

// tailSize is how much of the engine's stderr is kept.
const tailSize = 4 << 10

// tail keeps the last tailSize bytes written to it.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - tailSize; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	return len(p), nil
}

// last returns the last line with text, "" for none.
func (t *tail) last() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
