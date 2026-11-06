package scripts

import "testing"

// FuzzParse reads anything as a script's metadata comments.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"#!/bin/sh\n# @althea.title Hello\n# @althea.mode inline\n# @althea.argument1 { \"type\": \"text\" }\necho hi\n",
		"// @althea.title x\n// @althea.argument1 {", "REM @althea.title Batch\r\n", "# @althea.icon \U0001F600",
	} {
		f.Add("hello.sh", []byte(s))
	}
	f.Fuzz(func(_ *testing.T, name string, data []byte) { _, _ = Parse(name, data) })
}
