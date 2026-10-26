package app

import (
	"sync"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/snippets"
	"github.com/JoaoVitorOli/althea/internal/hotkey"
)

type fakeHook struct {
	mu      sync.Mutex
	handle  func(hotkey.KeyEvent)
	starts  int
	stopped int
}

func (h *fakeHook) Start(handle func(hotkey.KeyEvent)) (func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handle, h.starts = handle, h.starts+1
	return func() { h.mu.Lock(); h.stopped++; h.mu.Unlock() }, nil
}

func (*fakeHook) Mask() {}

func TestKeyboardRunsTheHookWhileSomethingListens(t *testing.T) {
	h := &fakeHook{}
	k := &keyboard{hook: h}
	var a, b []uint32
	stopA, err := k.listen(func(e hotkey.KeyEvent) { a = append(a, e.VK) })
	if err != nil {
		t.Fatal(err)
	}
	stopB, _ := k.listen(func(e hotkey.KeyEvent) { b = append(b, e.VK) })
	h.handle(hotkey.KeyEvent{VK: 1})
	stopA()
	h.handle(hotkey.KeyEvent{VK: 2})
	stopB()
	if h.starts != 1 || h.stopped != 1 {
		t.Errorf("hook started %d, stopped %d times; want once each", h.starts, h.stopped)
	}
	if len(a) != 1 || len(b) != 2 {
		t.Errorf("a heard %v, b heard %v", a, b)
	}
	if _, err := (&keyboard{}).listen(func(hotkey.KeyEvent) {}); err == nil {
		t.Error("listening without a hook works")
	}
}

func TestTypedKeys(t *testing.T) {
	for _, c := range []struct {
		e    hotkey.KeyEvent
		want snippets.Key
		ok   bool
	}{
		{hotkey.KeyEvent{VK: 0x41, Down: true, Text: "a"}, snippets.Key{Text: "a"}, true},
		{hotkey.KeyEvent{VK: 0x41, Down: false}, snippets.Key{}, false},
		{hotkey.KeyEvent{VK: 0x41, Down: true, Injected: true, Text: "a"}, snippets.Key{}, false},
		{hotkey.KeyEvent{VK: 0xA0, Down: true}, snippets.Key{}, false}, // Shift
		{hotkey.KeyEvent{VK: 0x08, Down: true}, snippets.Key{Backspace: true}, true},
		{hotkey.KeyEvent{VK: 0x0D, Down: true}, snippets.Key{}, true}, // Enter ends a word
	} {
		if got, ok := typedKey(c.e); got != c.want || ok != c.ok {
			t.Errorf("typedKey(%+v) = %+v, %v", c.e, got, ok)
		}
	}
}
