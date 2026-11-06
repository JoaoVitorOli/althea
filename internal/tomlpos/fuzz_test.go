package tomlpos

import "testing"

// FuzzParse reads anything as TOML with positions.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"a = 1\n[b]\nc = \"d\"\n", "[a.b.c]\n", "x = [1,\n2]", "a = { b = 1 }", "[[t]]\n"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(_ *testing.T, data []byte) { _, _ = Parse(data) })
}
