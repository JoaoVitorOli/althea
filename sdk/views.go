package sdk

// View is what a command shows in the launcher in place of the root search: a *List, a *Grid,
// a *Detail, a *Form, a *Chat or a *Converter. An action shows one with Context.Push; Escape goes back to the view
// before it. The host draws views with the theme, the motion and the keys of every other view,
// so an extension says what to show, not how. See docs/adr/0004-declarative-views.md.
type View interface{ isView() }

// List is a searchable list of items, as the root search is.
//
// Its rows come from Items, from Load or from Search, whichever is set first: the host filters
// Items and what Load returns as the user types, while Search filters by itself.
type List struct {
	// Title names the view in the footer, as the command that showed it.
	Title string
	// Placeholder is the search field's text while it is empty.
	Placeholder string
	Items       []Item
	// Load returns the items once, as the view opens; the view shows that it is loading
	// meanwhile.
	Load func(ctx Context) ([]Item, error)
	// Search returns the items for what is typed: called as the view opens with "", then as
	// the query changes. The host cancels a call whose query the user has since changed.
	Search func(ctx Context, query string) ([]Item, error)
	// Empty is shown when there are no items.
	Empty Empty
	// ShowDetail shows the chosen item's Detail beside the list.
	ShowDetail bool
}

// Grid is a searchable grid of items with large icons, as emoji or images. Its items come as
// a List's do.
type Grid struct {
	Title       string
	Placeholder string
	Items       []Item
	Load        func(ctx Context) ([]Item, error)
	Search      func(ctx Context, query string) ([]Item, error)
	Empty       Empty
	// Columns is how many items a row holds; 6 when zero.
	Columns int
}

// Empty is what a List or Grid shows without items.
type Empty struct {
	Title       string
	Description string
	Icon        Icon
}

// Detail shows a document: Markdown, with labeled values beside it.
type Detail struct {
	Title string
	// Image is shown above the Markdown, as large as fits: a PNG held in memory.
	Image    []byte
	Markdown string
	// Stream, when set, writes the Markdown as the view opens, as an answer that comes little by
	// little: each call of show replaces what shows. Escape or closing the view cancels ctx, and
	// the error is shown as a toast. It runs off the main thread, and show may be called from any
	// goroutine until Stream returns.
	Stream   func(ctx Context, show func(markdown string)) error
	Metadata []Metadata
	// Actions are what the user can do from the view, as an Item's: the first on Enter.
	Actions []Action
}

// Metadata is a labeled value of a Detail, as "Size: 2 MB".
type Metadata struct {
	Label string
	Text  string
}

// Form asks the user for values. Ctrl+Enter submits it once its required fields are filled
// and their values are valid.
type Form struct {
	Title  string
	Fields []Field
	// SubmitTitle names the submit action; "Submit" when empty.
	SubmitTitle string
	// Submit gets the values by field ID. On success the form closes, giving its place to the
	// view Submit pushed, if any; an error is shown and keeps the form open.
	Submit func(ctx Context, values Values) error
}

// FieldKind is the control a Field shows.
type FieldKind int

// The kinds of fields.
const (
	TextField FieldKind = iota // one line of text
	TextArea                   // several lines of text
	Password                   // one line of text, hidden
	Checkbox                   // on or off, as "true" or "false"
	Dropdown                   // one of Options, as its Value
)

// Field is a value a Form asks for.
type Field struct {
	ID    string // unique within the form: the key of its value
	Title string
	Kind  FieldKind
	// Placeholder is a text field's text while it is empty.
	Placeholder string
	// Info is a line of help below the control.
	Info string
	// Default is the value the field starts with: "true" checks a Checkbox, an option's Value
	// chooses it in a Dropdown, which otherwise starts on its first option.
	Default  string
	Options  []Option
	Required bool
	// Validate returns what is wrong with a value, or "" when it is fine.
	Validate func(value string) string
}

// Chat is a conversation: what was said, over a field to say more, with answers that come little
// by little.
type Chat struct {
	Title       string
	Placeholder string
	// Messages are what was said before.
	Messages []ChatMessage
	// First, when set, is said as the view opens, as if the user typed it.
	First string
	// Send answers what the user said: each call of show replaces the answer so far. Escape or
	// closing the view cancels ctx, and what came stays. The error is shown as a toast. It runs off
	// the main thread, and show may be called from any goroutine until Send returns.
	Send func(ctx Context, text string, show func(markdown string)) error
	// SendTitle names what Enter does in the footer; "Send" when empty.
	SendTitle string
	// Actions are what else the user can do from the view: the first on Ctrl+Enter.
	Actions []Action
}

// ChatMessage is something said in a Chat: by the user when Mine is set, and otherwise an answer,
// in Markdown.
type ChatMessage struct {
	Mine bool
	Text string
}

// Converter turns a text into another, as translating does: two choices with a swap between them
// on top, the text below them and what it became under it. The user searches a choice's options.
// Enter runs it, never typing, since Convert may send the text somewhere. The host offers to copy and
// paste the result.
type Converter struct {
	// Title names the view in the footer.
	Title string
	// Placeholder is the text's while it is empty, and ResultPlaceholder the result's.
	Placeholder       string
	ResultPlaceholder string
	// ConvertTitle names what Enter does in the footer; "Submit" when empty.
	ConvertTitle string
	// Text is the text it starts with, and RunAtOnce runs it on that as the view opens.
	Text      string
	RunAtOnce bool
	// From and To are the choices. Swapping is offered between values both have.
	From, To Picker
	// Convert runs on what was asked. Each call of show replaces what shows. Escape or closing the
	// view cancels ctx. Its error shows in place of the result. It runs off the main thread, and show
	// may be called from any goroutine until Convert returns.
	Convert func(ctx Context, c Conversion, show func(Converted)) error
}

// Picker is a choice of a Converter among options to search through.
type Picker struct {
	// Title names the choice.
	Title string
	// Placeholder is the search field's text while it is empty.
	Placeholder string
	// Value is the option it starts on.
	Value   string
	Options []Option
}

// Conversion is what a Converter is asked: a text and the choices' values.
type Conversion struct {
	Text, From, To string
}

// Converted is what a Converter shows.
type Converted struct {
	// Text is the result, plain text. Note says something else in its place, as how far a download
	// is; both are replaced by each call.
	Text string
	Note string
	// From, when set, says what From turned out to be, when an option leaves it to Convert to find
	// out, as detecting a language. To, when set, changes To, as when the text is in it already. Both
	// stay as they are otherwise.
	From, To string
}

// Option is a choice of a Dropdown or a Picker.
type Option struct {
	Value string
	Title string
	// Badge is a very short text beside the title in a Picker, as a language's code. It can be
	// searched for.
	Badge string
}

// Values are a submitted form's values, by field ID.
type Values map[string]string

// Bool reports whether a Checkbox field is checked.
func (v Values) Bool(id string) bool { return v[id] == "true" }

func (*List) isView()   {}
func (*Grid) isView()   {}
func (*Detail) isView() {}
func (*Form) isView()   {}
func (*Chat) isView()   {}

func (*Converter) isView() {}
