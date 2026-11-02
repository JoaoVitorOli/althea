package app

import (
	"errors"
	"slices"
	"testing"
)

// fakePaster copies what its selection says, recording the order of what is done.
type fakePaster struct {
	steps  *[]string
	copied bool
	err    error
}

func (fakePaster) Foreground() uintptr      { return 0 }
func (fakePaster) Paste(uintptr, int) error { return nil }

func (f fakePaster) CopySelection(window uintptr) (bool, error) {
	if window != 7 {
		return false, errors.New("the wrong window")
	}
	*f.steps = append(*f.steps, "copy")
	return f.copied, f.err
}

func steps(done *[]string, p fakePaster, showing bool) selectionSteps {
	note := func(s string) { *done = append(*done, s) }
	p.steps = done
	return selectionSteps{
		paster: p,
		target: func() uintptr { return 7 },
		hide:   func() bool { note("hide"); return showing },
		show:   func() { note("show") },
		save:   func() func() { note("save"); return func() { note("restore") } },
		text:   func() string { note("text"); return "selected" },
	}
}

func TestReadingTheSelectionPutsTheClipboardBackAndTheLauncherToo(t *testing.T) {
	var s selectionReader
	var done []string
	text, err := s.read(steps(&done, fakePaster{copied: true}, true))
	if err != nil || text != "selected" {
		t.Fatalf("text %q, err %v", text, err)
	}
	if want := []string{"hide", "save", "copy", "text", "restore", "show"}; !slices.Equal(done, want) {
		t.Errorf("steps %v, want %v", done, want)
	}
	if !s.reading() {
		t.Error("the clipboard changes it caused would reach the history")
	}
}

func TestNothingSelectedReadsNothingAndLeavesTheClipboard(t *testing.T) {
	var s selectionReader
	var done []string
	text, err := s.read(steps(&done, fakePaster{}, false))
	if err != nil || text != "" {
		t.Fatalf("text %q, err %v", text, err)
	}
	if want := []string{"hide", "save", "copy"}; !slices.Equal(done, want) {
		t.Errorf("steps %v, want %v", done, want)
	}
}

func TestASystemThatCannotPressKeysHasNoSelection(t *testing.T) {
	var s selectionReader
	text, err := s.read(selectionSteps{})
	if err != nil || text != "" {
		t.Errorf("text %q, err %v", text, err)
	}
}
