package scripts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JoaoVitorOli/althea/sdk"
)

// Mode is how a script's output shows.
type Mode string

// The modes of a script.
const (
	Silent Mode = "silent" // nothing: the launcher hides, and only a failure shows
	Toast  Mode = "toast"  // the last line of output, as a toast
	Detail Mode = "detail" // all of the output, in a view of its own
	List   Mode = "list"   // each line of output, as an item to copy
	Inline Mode = "inline" // the first line of output, under the title in the root search
)

// Script is a Script Command: an executable file with its metadata.
type Script struct {
	Path     string
	Title    string
	Subtitle string
	Mode     Mode
	Icon     sdk.Icon
	Keywords []string
	// Arguments are asked in a form before the script runs, and passed in order.
	Arguments []Argument
	// Confirm asks before the script runs.
	Confirm bool
	// Timeout stops a script that runs longer; 30 s by default.
	Timeout time.Duration
	// Refresh is how old an inline script's output may get before it runs again.
	Refresh time.Duration
}

// Argument is a value a script asks for.
type Argument struct {
	Placeholder string `json:"placeholder"`
	Optional    bool   `json:"optional"`
	// Secure hides what is typed, as for a password.
	Secure bool `json:"secure"`
}

const (
	defaultTimeout = 30 * time.Second
	defaultRefresh = time.Minute
	minRefresh     = 10 * time.Second
	// metaLines is how far into a file metadata is looked for.
	metaLines = 100
)

// metaLine matches a metadata comment in any common script language: # @althea.title Hello.
var metaLine = regexp.MustCompile(`^\s*(?:#|//|--|;|::|'|(?i:rem)\s)\s*@althea\.(\w+)\s*(.*?)\s*$`)

// Parse reads the metadata of the script at path from its contents. A script needs a title;
// a mistake in another value is an error naming it.
func Parse(path string, data []byte) (Script, error) {
	s := Script{Path: path, Mode: Toast, Icon: sdk.Icon{Glyph: "script"}, Timeout: defaultTimeout, Refresh: defaultRefresh}
	var errs []error
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), 64*1024)
	args := map[int]Argument{}
	for n := 0; n < metaLines && sc.Scan(); n++ {
		m := metaLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		key, value := strings.ToLower(m[1]), m[2]
		bad := func(format string, a ...any) {
			errs = append(errs, fmt.Errorf("line %d: @althea.%s: %s", n+1, m[1], fmt.Sprintf(format, a...)))
		}
		switch {
		case key == "title":
			s.Title = value
		case key == "subtitle" || key == "description":
			s.Subtitle = value
		case key == "mode":
			switch mode := Mode(strings.ToLower(value)); mode {
			case Silent, Toast, Detail, List, Inline:
				s.Mode = mode
			default:
				bad("%q is not a mode: use silent, toast, detail, list or inline", value)
			}
		case key == "icon":
			s.Icon = icon(path, value)
		case key == "keywords":
			for _, k := range strings.Split(value, ",") {
				if k = strings.TrimSpace(k); k != "" {
					s.Keywords = append(s.Keywords, k)
				}
			}
		case key == "confirm" || key == "needsconfirmation":
			b, err := strconv.ParseBool(value)
			if err != nil {
				bad("must be true or false")
			}
			s.Confirm = b
		case key == "timeout":
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 {
				bad("must be a duration such as 10s")
				continue
			}
			s.Timeout = d
		case key == "refresh" || key == "refreshtime":
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 {
				bad("must be a duration such as 5m")
				continue
			}
			s.Refresh = max(d, minRefresh)
		case strings.HasPrefix(key, "argument"):
			i, err := strconv.Atoi(strings.TrimPrefix(key, "argument"))
			if err != nil || i < 1 || i > 3 {
				bad("scripts take argument1, argument2 and argument3")
				continue
			}
			a, err := argument(value)
			if err != nil {
				bad("%v", err)
				continue
			}
			args[i] = a
		default:
			// Unknown keys are left for later versions, and other tools' metadata.
		}
	}
	for i := 1; i <= 3; i++ {
		a, ok := args[i]
		if !ok {
			break // arguments are numbered from 1 without gaps
		}
		s.Arguments = append(s.Arguments, a)
	}
	if s.Title == "" {
		errs = append(errs, errors.New("no @althea.title"))
	}
	if err := errors.Join(errs...); err != nil {
		return s, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return s, nil
}

// argument reads an argument as JSON, {"placeholder": "Query", "optional": true}, or as its
// placeholder alone.
func argument(value string) (Argument, error) {
	if !strings.HasPrefix(value, "{") {
		return Argument{Placeholder: value}, nil
	}
	var a Argument
	if err := json.Unmarshal([]byte(value), &a); err != nil {
		return a, fmt.Errorf("not valid JSON: %w", err)
	}
	return a, nil
}

// icon reads an icon: an image file beside the script, or a short text such as an emoji.
func icon(path, value string) sdk.Icon {
	switch strings.ToLower(filepath.Ext(value)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		if !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(path), value)
		}
		return sdk.Icon{Image: value, Glyph: "script"}
	}
	if value == "" || utf8.RuneCountInString(value) > 4 {
		return sdk.Icon{Glyph: "script"}
	}
	return sdk.Icon{Text: value, Glyph: "script"}
}
