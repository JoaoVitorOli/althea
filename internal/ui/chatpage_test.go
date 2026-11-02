package ui

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func TestChatSaysTheFirstMessageAndShowsTheAnswerAsItComes(t *testing.T) {
	r, tt, _ := pushing(t)
	var said []string
	var shows []func(string)
	r.Push("test", &sdk.Chat{
		Title: "Quick AI",
		First: "why is the sky blue",
		Messages: []sdk.ChatMessage{
			{Mine: true, Text: "earlier question"}, {Text: "earlier answer"},
		},
		Send: func(_ sdk.Context, text string, show func(string)) error {
			said = append(said, text)
			shows = append(shows, show)
			show("Because")
			show("Because of Rayleigh")
			return nil
		},
	})
	tt.Frame()
	if !slices.Equal(said, []string{"why is the sky blue"}) {
		t.Fatalf("said %q", said)
	}
	for _, want := range []string{"earlier question", "why is the sky blue"} {
		if !tt.HasText(want) {
			t.Errorf("does not show %q; texts %q", want, tt.Texts())
		}
	}
	chat := r.stack[0].(*chatPage)
	if last := chat.messages[len(chat.messages)-1]; last.text != "Because of Rayleigh" || last.writing {
		t.Errorf("answer %+v", last)
	}
	if chat.sending != nil {
		t.Error("an answer that came is still on its way")
	}
}

func TestChatSaysWhatIsTypedOnEnter(t *testing.T) {
	r, tt, _ := pushing(t)
	var said []string
	r.Push("test", &sdk.Chat{Title: "Chat", Send: func(_ sdk.Context, text string, show func(string)) error {
		said = append(said, text)
		show("ok")
		return nil
	}})
	tt.Frame()
	tt.Type("hello")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !slices.Equal(said, []string{"hello"}) {
		t.Fatalf("said %q", said)
	}
	if chat := r.stack[0].(*chatPage); chat.draft != "" {
		t.Errorf("the field still holds %q", chat.draft)
	}
	tt.Key(0, ui.KeyEnter) // nothing typed: nothing said
	if len(said) != 1 {
		t.Errorf("said %q", said)
	}
}

func TestChatDropsAnAnswerThatFailedBeforeAWordCame(t *testing.T) {
	r, tt, h := pushing(t)
	r.Push("test", &sdk.Chat{Title: "Chat", First: "hi", Send: func(sdk.Context, string, func(string)) error {
		return errors.New("Server did not answer")
	}})
	tt.Frame()
	if len(h.errs) != 1 {
		t.Errorf("errors %v", h.errs)
	}
	chat := r.stack[0].(*chatPage)
	if len(chat.messages) != 1 || !chat.messages[0].mine {
		t.Errorf("messages %+v", chat.messages)
	}
}

// heldHost runs extensions' code on a goroutine of its own, and their updates when flushed.
type heldHost struct {
	syncHost
	updates chan func()
	done    chan struct{}
}

func (h *heldHost) Go(_ string, fn func(ctx sdk.Context) error) func() {
	ctx, cancel := context.WithCancel(context.Background())
	sc := sdktest.New(h.t, testExt{}, "en")
	sc.Context = ctx
	go func() {
		defer close(h.done)
		_ = fn(sc)
	}()
	return cancel
}

func (h *heldHost) Update(fn func()) { h.updates <- fn }

func TestEscapeStopsAnAnswerKeepingWhatCameThenLeaves(t *testing.T) {
	r, tt, _ := pushing(t)
	h := &heldHost{updates: make(chan func(), 8), done: make(chan struct{})}
	h.t, h.r = t, r
	r.Host = h
	started := make(chan func(string))
	r.Push("test", &sdk.Chat{Title: "Chat", First: "hi", Send: func(ctx sdk.Context, _ string, show func(string)) error {
		started <- show
		<-ctx.Done()
		return ctx.Err()
	}})
	show := <-started
	show("Hel")
	(<-h.updates)()
	tt.Frame()
	chat := r.stack[0].(*chatPage)
	if chat.sending == nil || !chat.messages[1].writing {
		t.Fatalf("not on its way: %+v", chat.messages[1])
	}

	tt.Key(0, ui.KeyEscape)
	<-h.done
	if len(r.stack) != 1 {
		t.Fatal("Escape left instead of stopping the answer")
	}
	if chat.sending != nil || chat.messages[1].writing || chat.messages[1].text != "Hel" {
		t.Errorf("after stopping: %+v", chat.messages[1])
	}
	(<-h.updates)() // the stopped answer's last word changes nothing
	if chat.messages[1].text != "Hel" {
		t.Errorf("text %q", chat.messages[1].text)
	}

	tt.Key(0, ui.KeyEscape)
	if len(r.stack) != 0 {
		t.Error("a second Escape did not leave")
	}
}

func TestDetailWritesWhatItStreams(t *testing.T) {
	r, tt, h := pushing(t)
	r.Push("test", &sdk.Detail{Title: "Summary", Stream: func(_ sdk.Context, show func(string)) error {
		show("one")
		show("one two")
		return errors.New("cut off")
	}})
	tt.Frame()
	d := r.stack[0].(*detailPage)
	if d.text != "one two" || d.streaming {
		t.Errorf("text %q, streaming %v", d.text, d.streaming)
	}
	if !tt.HasText("one two") {
		t.Errorf("texts %q", tt.Texts())
	}
	if len(h.errs) != 1 {
		t.Errorf("errors %v", h.errs)
	}
}

func TestTabHandsTheQueryToWhoTakesIt(t *testing.T) {
	r, tt, _ := pushing(t)
	var taken []string
	r.OnTake = func(q string) { taken = append(taken, q) }
	tt.Frame()
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}

	tt.Type("what is go")
	tt.Key(0, ui.KeyTab)
	if len(taken) != 0 {
		t.Fatalf("taken %q before anyone said they take it", taken)
	}

	r.SetTake("something older", "Ask AI")
	tt.Frame()
	if tt.HasText("Ask AI") {
		t.Error("the hint of an older query shows")
	}

	r.SetResults("what is go", []registry.Result{{Key: "x/1", Item: sdk.Item{Title: "One"}}})
	r.SetTake("what is go", "Ask AI")
	tt.Frame()
	if !tt.HasText("Ask AI") {
		t.Errorf("no hint; texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyTab)
	if !slices.Equal(taken, []string{"what is go"}) {
		t.Errorf("taken %q", taken)
	}

	tt.Type("!")
	tt.Frame()
	if tt.HasText("Ask AI") {
		t.Error("the hint outlives its query")
	}
}
