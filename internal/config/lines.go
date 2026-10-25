package config

import (
	"bytes"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// keyLines maps each dotted key that a TOML document sets to the line it is on, so problems
// found after decoding can point at the line to fix. Unparsable input gives what was read.
func keyLines(data []byte) map[string]int {
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
