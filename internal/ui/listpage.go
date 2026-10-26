package ui

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/JoaoVitorOli/althea/internal/search"
	"github.com/JoaoVitorOli/althea/sdk"
)

// listSource is what a List or a Grid gives: the same but for the grid's columns.
type listSource struct {
	title, placeholder string
	items              []sdk.Item
	load               func(ctx sdk.Context) ([]sdk.Item, error)
	search             func(ctx sdk.Context, query string) ([]sdk.Item, error)
	empty              sdk.Empty
	showDetail         bool
	grid               bool
	columns            int
}

// listPage shows a List or a Grid: a search field over its items. It filters the items itself,
// unless the extension searches.
type listPage struct {
	pageBase
	src listSource

	all   []sdk.Item    // what the extension gave, for filtering here
	index *search.Index // over all, by position
	shown []shownItem   // what matches the query, in order
	rows  []listRow     // shown, with section headings when unfiltered

	query    string
	input    ui.Handle
	list     ui.ListState
	gridView ui.GridState
	selected int // in shown
	loading  bool

	// cellRows are the tops of the grid's cells the last frame built, by item.
	cellRows map[int]float32

	searching func() // cancels the search in flight
	searches  int    // searches asked, so that a late one is dropped
	md        markdown.Options
}

type shownItem struct {
	item      sdk.Item
	positions []int
}

// listRow is a row of a list: a section heading or an item of shown.
type listRow struct {
	heading string
	item    int // -1 for a heading
}

func newListPage(r *Root, ext string, src listSource) *listPage {
	p := &listPage{pageBase: pageBase{r: r, ext: ext, name: src.title}, src: src}
	p.list.Key = func(i int) any {
		if row := p.rows[i]; row.item >= 0 {
			return p.key(row.item)
		}
		return "heading:" + p.rows[i].heading
	}
	p.gridView.Key = func(i int) any { return p.key(i) }
	p.gridView.Label = func(i int) string { return p.shown[i].item.Title }
	switch {
	case src.search != nil:
		p.searchFor("")
	case src.load != nil:
		p.loading = true
		p.work(func(ctx sdk.Context) error {
			items, err := src.load(ctx)
			p.update(func() {
				p.loading = false
				if err == nil {
					p.setItems(items)
				}
			})
			return err
		})
	default:
		p.setItems(src.items)
	}
	return p
}

// key identifies the i-th shown item, by its ID or else its place.
func (p *listPage) key(i int) any {
	if id := p.shown[i].item.ID; id != "" {
		return id
	}
	return i
}

// setItems takes the items to filter here.
func (p *listPage) setItems(items []sdk.Item) {
	p.all = items
	candidates := make([]search.Candidate, len(items))
	for i, it := range items {
		candidates[i] = search.Candidate{ID: strconv.Itoa(i), Title: it.Title, Keywords: it.Keywords}
	}
	p.index = search.NewIndex(candidates)
	p.filter()
}

// filter shows the items matching the query: all of them, in sections, while it is empty, and
// the matches, best first, once it is not.
func (p *listPage) filter() {
	if p.query == "" || p.index == nil {
		p.show(p.all, true)
		return
	}
	ranked := p.index.Rank(p.query, func(string) float64 { return 0 })
	shown := make([]shownItem, len(ranked))
	for i, rk := range ranked {
		n, _ := strconv.Atoi(rk.ID)
		shown[i] = shownItem{item: p.all[n], positions: rk.Positions}
	}
	p.shown = shown
	p.layOut(false)
}

// show shows items in their order, grouped by section when sections is set.
func (p *listPage) show(items []sdk.Item, sections bool) {
	p.shown = make([]shownItem, 0, len(items))
	if !sections {
		for _, it := range items {
			p.shown = append(p.shown, shownItem{item: it})
		}
		p.layOut(false)
		return
	}
	// Each section's items together, the sections in the order they first appear.
	var order []string
	bySection := map[string][]sdk.Item{}
	for _, it := range items {
		if _, ok := bySection[it.Section]; !ok {
			order = append(order, it.Section)
		}
		bySection[it.Section] = append(bySection[it.Section], it)
	}
	for _, s := range order {
		for _, it := range bySection[s] {
			p.shown = append(p.shown, shownItem{item: it})
		}
	}
	p.layOut(true)
}

// layOut makes the rows of the shown items, with a heading over each named section when
// headings is set, and chooses the first item.
func (p *listPage) layOut(headings bool) {
	p.rows = p.rows[:0]
	section := ""
	for i, s := range p.shown {
		if headings && s.item.Section != "" && (i == 0 || s.item.Section != section) {
			p.rows = append(p.rows, listRow{heading: s.item.Section, item: -1})
		}
		section = s.item.Section
		p.rows = append(p.rows, listRow{item: i})
	}
	p.choose(0)
}

// searchFor asks the extension for the items of query, in place of the search before.
func (p *listPage) searchFor(query string) {
	if p.searching != nil {
		p.searching()
	}
	p.searches++
	n := p.searches
	p.loading = true
	if p.r.Host == nil {
		return
	}
	p.searching = p.r.Host.Go(p.ext, func(ctx sdk.Context) error {
		items, err := p.src.search(ctx, query)
		if ctx.Err() != nil {
			return nil // the user typed on: this search is moot
		}
		p.update(func() {
			if n != p.searches {
				return
			}
			p.loading = false
			if err == nil {
				p.show(items, true)
			}
		})
		return err
	})
}

func (p *listPage) close() {
	p.pageBase.close()
	if p.searching != nil {
		p.searching()
	}
}

func (p *listPage) choose(i int) {
	p.r.armed, p.r.notice = armed{}, ""
	p.selected = max(0, min(i, len(p.shown)-1))
	if len(p.shown) == 0 {
		return
	}
	if p.src.grid {
		p.gridView.ScrollIntoView(p.selected)
		return
	}
	for r, row := range p.rows {
		if row.item == p.selected {
			p.list.ScrollIntoView(r)
			return
		}
	}
}

func (p *listPage) focus() { p.input.Focus() }

func (p *listPage) chosen() string {
	if len(p.shown) == 0 {
		return ""
	}
	return strconv.Itoa(p.selected) + "/" + p.shown[p.selected].item.ID
}

func (p *listPage) actions() []sdk.Action {
	if len(p.shown) == 0 {
		return nil
	}
	return p.shown[p.selected].item.Actions
}

func (p *listPage) secondary() int { return 1 }

func (p *listPage) escape() bool {
	if p.query == "" {
		return false
	}
	p.query = ""
	p.changed()
	return true
}

// changed follows a new query.
func (p *listPage) changed() {
	if p.src.search != nil {
		p.searchFor(p.query)
		return
	}
	p.filter()
}

// columns is how many items a row of the grid holds.
func (p *listPage) columns() int {
	if p.src.columns > 0 {
		return p.src.columns
	}
	return 6
}

func (p *listPage) keys(c *ui.Context) {
	step := 1
	if p.src.grid {
		step = p.laidOutColumns()
	}
	if c.Shortcut(0, ui.KeyDown) {
		p.choose(p.selected + step)
	}
	if c.Shortcut(0, ui.KeyUp) {
		p.choose(p.selected - step)
	}
}

func (p *listPage) header(c *ui.Context) {
	placeholder := p.src.placeholder
	if placeholder == "" {
		placeholder = p.r.T("view.search_placeholder")
	}
	field := ui.TextInputBase(c, &p.query).Bind(&p.input).Grow(1).FontSize(p.r.theme.Font.Size + 4).
		Placeholder(placeholder).Label(p.r.T("launcher.search_label"))
	if field.Changed() {
		p.r.actions.open = false
		p.changed()
	}
	field.HandleInput(func(ev ui.InputEvent) bool {
		if p.r.pageShortcut(p, ev) {
			return true
		}
		// Left and Right move through a grid's items rather than the caret.
		if p.src.grid && ev.Kind == ui.InputKeyDown && ev.Mods == 0 && !p.r.actions.open {
			switch ev.Key {
			case ui.KeyLeft:
				p.choose(p.selected - 1)
				return true
			case ui.KeyRight:
				p.choose(p.selected + 1)
				return true
			}
		}
		return false
	})
	if field.Submitted() && len(p.shown) > 0 {
		action := 0
		if p.r.actions.open {
			action = p.r.actions.selected
		}
		p.r.runPage(p, action)
	}
	if p.loading {
		ui.Spinner(c).Size(16, 16)
	}
}

func (p *listPage) body(c *ui.Context) {
	if len(p.shown) == 0 {
		if p.loading {
			return
		}
		empty := p.src.empty
		if empty.Title == "" {
			empty.Title = p.r.T("view.empty")
			if p.query != "" {
				empty.Title = p.r.T("launcher.no_results", "query", p.query)
			}
		}
		p.r.emptyState(c, empty)
		return
	}
	if p.src.grid {
		p.grid(c)
		return
	}
	if !p.src.showDetail {
		p.listView(c).Grow(1)
		return
	}
	ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
		p.listView(c).Width(300)
		ui.Box(c).Width(1).Background(c.Theme().Border)
		ui.Scroll(c).Grow(1).Padding(16).Children(func() {
			if d := p.shown[p.selected].item.Detail; d != nil {
				p.r.detail(c, d, &p.md)
			}
		})
	})
}

func (p *listPage) listView(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.List(c, &p.list, len(p.rows), func(i int) {
		row := p.rows[i]
		if row.item < 0 {
			ui.Row(c).Height(28).Padding(8, 10, 0, 10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, row.heading).FontSize(p.r.theme.Font.Size - 2).FontWeight(600).TextColor(t.TextMuted)
			})
			return
		}
		s := p.shown[row.item]
		if p.r.itemRow(c, s.item, s.positions, row.item == p.selected, false, s.item.Accessory).Clicked() {
			p.choose(row.item)
			p.r.runPage(p, 0)
		}
	}).Padding(6)
}

func (p *listPage) grid(c *ui.Context) {
	t := c.Theme()
	cols := p.columns()
	// The gap MyGo counts columns with, so that the cells come out as wide as asked.
	gap := t.Space(2)
	const pad = 10
	w, _ := c.Size()
	// A little under the width each column has, leaving room for the scroll bar, so that
	// rounding never costs a column.
	const scrollBar = 16
	cell := (w-2*pad-scrollBar-float32(cols-1)*gap)/float32(cols) - 1
	p.cellRows = map[int]float32{}
	ui.GridView(c, &p.gridView, len(p.shown), cell, cell+22, func(i int) {
		s := p.shown[i]
		// Every cell has the border, clear but for the chosen one's, so that nothing moves.
		box := ui.Column(c).Grow(1).Padding(8).Gap(4).AlignItems(ui.Center).Radius(r10(p.r)).
			BorderWidth(2).BorderColor(ui.Color{})
		switch {
		case i == p.selected:
			box.Background(color(p.r.theme.Colors.RowSelected)).BorderColor(t.Accent)
		case box.Hovered():
			box.Background(color(p.r.theme.Colors.RowSelected).Alpha(0.5))
		}
		if box.Clicked() {
			p.choose(i)
			p.r.runPage(p, 0)
		}
		if b := box.LayoutBounds(); b.H > 0 {
			p.cellRows[i] = b.Y
		}
		box.Children(func() {
			ui.Row(c).Grow(1).Center().Children(func() {
				p.r.icon(c, s.item.Icon, min(cell*0.5, 56), t.TextMuted)
			})
			ui.Text(c, s.item.Title).FontSize(p.r.theme.Font.Size - 3).TextColor(t.TextMuted).SingleLine()
		})
	}).Grow(1).Padding(pad).Gap(gap)
}

// laidOutColumns is how many items a row of the grid held in the last frame, which Up and Down
// move by: the columns asked for may not fit the window. The cells built then tell, by how many
// share a row.
func (p *listPage) laidOutColumns() int {
	perRow := map[float32]int{}
	most := 0
	for _, y := range p.cellRows {
		perRow[y]++
		most = max(most, perRow[y])
	}
	if most == 0 {
		return p.columns()
	}
	return most
}

// r10 is the radius of a grid's cells: the rows', a little rounder.
func r10(r *Root) float32 { return r.theme.Shape.RowRadius + 2 }
