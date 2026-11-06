package ai

import "testing"

// FuzzParseCommand reads anything as an AI Command file.
func FuzzParseCommand(f *testing.F) {
	for _, s := range []string{
		"title = \"Fix\"\nprompt = \"Fix {text}\"", "title = { en = \"a\", pt-BR = \"b\" }\nprompt = \"x\"\noutput = \"paste\"",
		"prompt = 1", "input = \"nope\"\nprompt = \"x\"",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		if c, err := parseCommand("fuzz", data); err == nil {
			_, _ = localized(c.Title, "pt-BR"), localized(c.Subtitle, "en")
		}
	})
}
