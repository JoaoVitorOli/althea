package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/sdk"
)

// formPage shows a Form. Its problems show once the user first tries to submit, and follow the
// values from then on.
type formPage struct {
	pageBase
	f *sdk.Form

	text    map[string]*string // text fields' values, by field ID
	checks  map[string]*bool
	choices map[string]*string // dropdowns' chosen titles
	first   ui.Handle          // the first field, which gets the keyboard

	tried      bool // the user tried to submit
	submitting bool
}

func newFormPage(r *Root, ext string, f *sdk.Form) *formPage {
	p := &formPage{
		pageBase: pageBase{r: r, ext: ext, name: f.Title}, f: f,
		text: map[string]*string{}, checks: map[string]*bool{}, choices: map[string]*string{},
	}
	for _, fd := range f.Fields {
		switch fd.Kind {
		case sdk.Checkbox:
			on := fd.Default == "true"
			p.checks[fd.ID] = &on
		case sdk.Dropdown:
			title := ""
			if len(fd.Options) > 0 {
				title = fd.Options[0].Title
			}
			for _, o := range fd.Options {
				if o.Value == fd.Default {
					title = o.Title
					break
				}
			}
			p.choices[fd.ID] = &title
		default:
			v := fd.Default
			p.text[fd.ID] = &v
		}
	}
	return p
}

// values are the form's values, by field ID.
func (p *formPage) values() sdk.Values {
	v := sdk.Values{}
	for _, fd := range p.f.Fields {
		v[fd.ID] = p.value(fd)
	}
	return v
}

func (p *formPage) value(fd sdk.Field) string {
	switch fd.Kind {
	case sdk.Checkbox:
		return fmt.Sprint(*p.checks[fd.ID])
	case sdk.Dropdown:
		for _, o := range fd.Options {
			if o.Title == *p.choices[fd.ID] {
				return o.Value
			}
		}
		return ""
	default:
		return *p.text[fd.ID]
	}
}

// problem is what is wrong with a field's value, or "".
func (p *formPage) problem(fd sdk.Field) (msg string) {
	v := p.value(fd)
	if fd.Required && strings.TrimSpace(v) == "" && fd.Kind != sdk.Checkbox {
		return p.r.T("view.required")
	}
	if fd.Validate == nil {
		return ""
	}
	defer func() {
		if recover() != nil {
			msg = "" // a broken validator does not lock the form
		}
	}()
	return fd.Validate(v)
}

// submit hands the values to the extension once every field is fine.
func (p *formPage) submit() {
	p.tried = true
	if p.submitting {
		return
	}
	for _, fd := range p.f.Fields {
		if p.problem(fd) != "" {
			return
		}
	}
	if p.f.Submit == nil {
		p.r.remove(p)
		return
	}
	p.submitting = true
	values := p.values()
	p.work(func(ctx sdk.Context) error {
		err := p.f.Submit(ctx, values)
		p.update(func() {
			p.submitting = false
			if err == nil {
				p.r.remove(p) // a view Submit pushed stays, in the form's place
			}
		})
		return err
	})
}

func (p *formPage) header(c *ui.Context) {
	ui.Text(c, p.f.Title).FontSize(p.r.theme.Font.Size + 2).FontWeight(600).TextColor(c.Theme().Text).Grow(1).SingleLine()
	if p.submitting {
		ui.Spinner(c).Size(16, 16)
	}
}

func (p *formPage) body(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(18, 28).Children(func() {
		ui.Form(c, func() {
			for i, fd := range p.f.Fields {
				var problem string
				if p.tried {
					problem = p.problem(fd)
				}
				ui.Field(c, fd.Title, func() { p.control(c, fd, i == 0) }).Description(fd.Info).Error(problem)
			}
		})
	})
}

// control draws the control of a field; the first takes the keyboard as the form opens.
func (p *formPage) control(c *ui.Context, fd sdk.Field, first bool) {
	var el ui.Element
	switch fd.Kind {
	case sdk.Checkbox:
		el = ui.Checkbox(c, p.checks[fd.ID], fd.Placeholder)
	case sdk.Dropdown:
		titles := make([]string, len(fd.Options))
		for i, o := range fd.Options {
			titles[i] = o.Title
		}
		if len(titles) > searchableOptions {
			el = ui.Combobox(c, p.choices[fd.ID], titles).Label(fd.Title)
		} else {
			el = ui.Select(c, p.choices[fd.ID], titles).Label(fd.Title)
		}
	case sdk.TextArea:
		el = ui.TextArea(c, p.text[fd.ID]).Placeholder(fd.Placeholder).Height(96)
	default:
		el = ui.TextInput(c, p.text[fd.ID]).Placeholder(fd.Placeholder)
		if fd.Kind == sdk.Password {
			el.Password()
		}
		if el.Submitted() {
			p.submit()
		}
	}
	if first {
		el.Bind(&p.first)
	}
}

func (p *formPage) keys(*ui.Context) {}

func (p *formPage) actions() []sdk.Action {
	title := p.f.SubmitTitle
	if title == "" {
		title = p.r.T("view.submit")
	}
	return []sdk.Action{{Title: title}}
}

// The form's one action is submitting, by Ctrl+Enter, which runs the first action.
func (*formPage) secondary() int { return 0 }
func (*formPage) chosen() string { return "" }
func (*formPage) escape() bool   { return false }
func (p *formPage) focus()       { p.first.Focus() }
