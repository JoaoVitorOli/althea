package sdk_test

import (
	"testing"

	"github.com/JoaoVitorOli/althea/sdk"
)

func TestNewIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 10000 {
		id := sdk.NewID()
		if seen[id] {
			t.Fatalf("%s came twice", id)
		}
		seen[id] = true
	}
}
