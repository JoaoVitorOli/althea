package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRootShowsTitleAndHint(t *testing.T) {
	r := &Root{}
	tt := ui.NewTester(r.View, 720, 420)
	for _, want := range []string{"Althea", "Esc to close"} {
		if !tt.HasText(want) {
			t.Errorf("root view does not show %q; texts %q", want, tt.Texts())
		}
	}
}
