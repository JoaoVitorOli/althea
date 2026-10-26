package app

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/JoaoVitorOli/althea/extensions/snippets"
	"github.com/JoaoVitorOli/althea/internal/hotkey"
	"github.com/JoaoVitorOli/althea/internal/platform"
)

// keyboard shares the keyboard hook among what listens to it: the Win key hotkey and snippet
// expansion. The hook runs only while something listens (docs/adr/0009-hooks.md).
type keyboard struct {
	hook platform.KeyHook

	mu        sync.Mutex // guards listeners, next and stop
	listeners map[int]func(hotkey.KeyEvent)
	next      int
	stop      func()
	// current is what the hook calls, swapped whole so that the hook's thread never waits.
	current atomic.Pointer[[]func(hotkey.KeyEvent)]
}

// listen calls fn with every key event, on the hook's thread: fn must return at once. The hook
// starts with the first listener and stops with the last.
func (k *keyboard) listen(fn func(hotkey.KeyEvent)) (unlisten func(), err error) {
	if k == nil || k.hook == nil {
		return nil, errors.New("this system cannot watch the keyboard")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.listeners == nil {
		k.listeners = map[int]func(hotkey.KeyEvent){}
	}
	id := k.next
	k.next++
	k.listeners[id] = fn
	k.publish()
	if k.stop == nil {
		stop, err := k.hook.Start(k.dispatch)
		if err != nil {
			delete(k.listeners, id)
			k.publish()
			return nil, err
		}
		k.stop = stop
	}
	var once sync.Once
	return func() { once.Do(func() { k.unlisten(id) }) }, nil
}

func (k *keyboard) unlisten(id int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.listeners, id)
	k.publish()
	if len(k.listeners) == 0 && k.stop != nil {
		k.stop()
		k.stop = nil
	}
}

// publish hands the hook the listeners of now. Called with mu held.
func (k *keyboard) publish() {
	fns := make([]func(hotkey.KeyEvent), 0, len(k.listeners))
	for _, fn := range k.listeners {
		fns = append(fns, fn)
	}
	k.current.Store(&fns)
}

func (k *keyboard) dispatch(e hotkey.KeyEvent) {
	if fns := k.current.Load(); fns != nil {
		for _, fn := range *fns {
			fn(e)
		}
	}
}

// Virtual-key codes of the modifiers, which type nothing and end no word.
var modifiers = map[uint32]bool{
	0x10: true, 0xA0: true, 0xA1: true, // Shift
	0x11: true, 0xA2: true, 0xA3: true, // Ctrl
	0x12: true, 0xA4: true, 0xA5: true, // Alt
	0x14: true,             // Caps Lock
	0x5B: true, 0x5C: true, // Win
}

const vkBackspace = 0x08

// typedKey is what a key event means to snippet expansion, and false for none.
func typedKey(e hotkey.KeyEvent) (snippets.Key, bool) {
	switch {
	case !e.Down || e.Injected || modifiers[e.VK]:
		return snippets.Key{}, false
	case e.Text != "":
		return snippets.Key{Text: e.Text}, true
	case e.VK == vkBackspace:
		return snippets.Key{Backspace: true}, true
	default:
		return snippets.Key{}, true // a key that ends the word, as Enter or an arrow
	}
}

// typing hands snippet expansion the keys typed, on a goroutine of its own, so that the hook
// returns at once. Keys typed faster than expansion reads them are dropped, which only costs
// an expansion.
func (k *keyboard) typing(on func(snippets.Key)) (stop func(), err error) {
	keys := make(chan snippets.Key, 128)
	done := make(chan struct{})
	unlisten, err := k.listen(func(e hotkey.KeyEvent) {
		if key, ok := typedKey(e); ok {
			select {
			case keys <- key:
			default:
			}
		}
	})
	if err != nil {
		return nil, err
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case key := <-keys:
				on(key)
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			unlisten()
			close(done)
			<-finished
		})
	}, nil
}
