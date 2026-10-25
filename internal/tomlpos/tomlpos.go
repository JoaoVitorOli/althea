// Package tomlpos finds where keys are in a TOML document, so problems found after decoding
// can point at the line to fix, and edits can change a value in place, keeping the comments
// and layout around it.
package tomlpos

import (
	"bytes"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Value is where a key's value is in a document.
type Value struct {
	Line       int // 1-based line of the key
	Start, End int // byte range of the value as written, quotes included
}

// Doc is the layout of a document.
type Doc struct {
	Values map[string]Value // by dotted key
	// TableEnds are the byte offsets just past the line of each table's last key, or of its
	// header when it has none, where a new key of the table goes. The root table is "".
	TableEnds map[string]int
}

// Parse reads the layout of a TOML document, as far as it parses.
func Parse(data []byte) (Doc, error) {
	d := Doc{Values: map[string]Value{}, TableEnds: map[string]int{"": 0}}
	var p unstable.Parser
	p.Reset(data)
	var table []string
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = keyOf(e)
			d.TableEnds[strings.Join(table, ".")] = lineEnd(data, int(p.Range(lastKeyPart(e)).Offset))
		case unstable.KeyValue:
			key := strings.Join(append(append([]string(nil), table...), keyOf(e)...), ".")
			first := e.Key()
			if !first.Next() {
				continue
			}
			v := e.Value()
			r := v.Raw
			if r.Length == 0 && len(v.Data) > 0 {
				r = p.Range(v.Data)
			}
			start, end := int(r.Offset), int(r.Offset+r.Length)
			start, end = quoted(data, start, end)
			d.Values[key] = Value{Line: lineOf(data, p.Range(first.Node().Data).Offset), Start: start, End: end}
			d.TableEnds[strings.Join(table, ".")] = lineEnd(data, end)
		default:
		}
	}
	return d, p.Error()
}

// Lines maps each dotted key that a TOML document sets to the line it is on. Unparsable
// input gives what was read before the mistake.
func Lines(data []byte) map[string]int {
	d, _ := Parse(data)
	lines := make(map[string]int, len(d.Values))
	for k, v := range d.Values {
		lines[k] = v.Line
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

func lastKeyPart(n *unstable.Node) []byte {
	var last []byte
	for it := n.Key(); it.Next(); {
		last = it.Node().Data
	}
	return last
}

// quoted widens the range of a string's contents to its quotes.
func quoted(data []byte, start, end int) (int, int) {
	for _, q := range []string{`"""`, `'''`, `"`, `'`} {
		if start >= len(q) && end+len(q) <= len(data) &&
			string(data[start-len(q):start]) == q && string(data[end:end+len(q)]) == q {
			return start - len(q), end + len(q)
		}
	}
	return start, end
}

func lineOf(data []byte, offset uint32) int {
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}

// lineEnd returns the offset just past the newline ending the line at offset.
func lineEnd(data []byte, offset int) int {
	if i := bytes.IndexByte(data[offset:], '\n'); i >= 0 {
		return offset + i + 1
	}
	return len(data)
}
