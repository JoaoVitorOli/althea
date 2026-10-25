// Package tomlpos finds where keys are in a TOML document, so problems found after decoding
// can point at the line to fix.
package tomlpos

import (
	"bytes"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Lines maps each dotted key that a TOML document sets to the line it is on. Unparsable
// input gives what was read before the mistake.
func Lines(data []byte) map[string]int {
	lines := map[string]int{}
	var p unstable.Parser
	p.Reset(data)
	var table []string
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = keyOf(e)
		case unstable.KeyValue:
			key := strings.Join(append(append([]string(nil), table...), keyOf(e)...), ".")
			first := e.Key()
			if first.Next() {
				lines[key] = lineOf(data, p.Range(first.Node().Data).Offset)
			}
		default:
		}
	}
	return lines
}

func keyOf(n *unstable.Node) []string {
	var parts []string
	for it := n.Key(); it.Next(); {
		parts = append(parts, string(it.Node().Data))
	}
	return parts
}

func lineOf(data []byte, offset uint32) int {
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
