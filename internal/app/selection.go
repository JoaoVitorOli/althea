package app

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/sdk"
)

// selectionQuiet is how long after reading the selection a clipboard change is taken for its own
// doing: the system tells of changes a moment late.
const selectionQuiet = 500 * time.Millisecond

// selectionSteps are what reading the selection does, apart for testing.
type selectionSteps struct {
	// paster is nil where Althea cannot press keys in other apps.
	paster platform.Paster
	target func() uintptr
	// hide hides the launcher, reporting whether it showed; show shows it again.
	hide func() (showed bool)
	show func()
	// save remembers what the clipboard holds and returns what puts it back; text reads its text.
	save func() (restore func())
	text func() string
}

// selectionReader reads the text selected in other apps, which only pressing Ctrl+C tells on every
// kind of app, and keeps what that copies out of the clipboard history. The zero value is ready.
type selectionReader struct {
	mu    sync.Mutex
	quiet atomic.Int64 // Unix nanoseconds until which clipboard changes are its own
}

// reading reports whether a clipboard change now is its own.
func (s *selectionReader) reading() bool { return time.Now().UnixNano() < s.quiet.Load() }

func (s *selectionReader) hush() { s.quiet.Store(time.Now().Add(selectionQuiet).UnixNano()) }

// read hides the launcher, copies the selection, puts the clipboard back and shows the launcher
// again, one read at a time.
func (s *selectionReader) read(st selectionSteps) (string, error) {
	if st.paster == nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.hide() {
		defer st.show()
	}
	s.hush()
	restore := st.save()
	copied, err := st.paster.CopySelection(st.target())
	if err != nil || !copied {
		return "", err
	}
	text := st.text()
	s.hush()
	restore()
	s.hush()
	return text, nil
}

// selectionClipboard is how the running app reads the selection.
func (a *althea) selectionClipboard() selectionSteps {
	return selectionSteps{
		paster: a.services.Paster,
		target: a.pasteTarget.Load,
		hide: func() bool {
			showed := make(chan bool, 1)
			a.launcher.Window().Update(func() {
				showed <- a.launcher.Window().IsVisible()
				a.launcher.Hide()
			})
			return <-showed
		},
		show: a.reveal,
		save: func() func() {
			held, ok := readClipboard()
			host := hostContext{a: a}
			return func() {
				if !ok {
					mygo.Clipboard.Clear()
					return
				}
				if err := host.Copy(sdk.Clip{Text: held.Text, HTML: held.HTML, Image: held.Image, Files: held.Files}); err != nil {
					a.log.Warn("clipboard not put back", "err", err)
				}
			}
		},
		text: mygo.Clipboard.ReadText,
	}
}
