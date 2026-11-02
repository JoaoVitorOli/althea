package registry_test

import (
	"context"
	"testing"

	"github.com/JoaoVitorOli/althea/sdk"
)

func TestCommandFindsCommandsAloneAsLastRefreshed(t *testing.T) {
	ext := source{&fake{
		id:       "x",
		commands: []sdk.Command{{ID: "run", Title: "Run", Confirm: "Sure?"}},
		items:    func() ([]sdk.Item, error) { return []sdk.Item{{ID: "doc", Title: "Doc"}}, nil },
	}}
	r := newRegistry(t, ext)
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	res, ok := r.Command("x/run")
	if !ok || res.Extension != "x" || res.Item.Actions[0].Confirm != "Sure?" {
		t.Errorf("Command = %+v, %v", res, ok)
	}
	if _, ok := r.Command("x/doc"); ok {
		t.Error("an item counted as a command")
	}
	if _, ok := r.Command("x/none"); ok {
		t.Error("found a command that does not exist")
	}
	if all := r.Commands(); len(all) != 1 || all[0].Key != "x/run" {
		t.Errorf("Commands = %+v", all)
	}

	ext.commands = nil
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Command("x/run"); ok {
		t.Error("a command gone after a refresh is still found")
	}
}
