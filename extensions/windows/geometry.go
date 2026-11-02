package windows

// Rect is a rectangle of the screen, in pixels.
type Rect struct{ X, Y, W, H int }

// overlap is the area r shares with o.
func (r Rect) overlap(o Rect) int {
	w := min(r.X+r.W, o.X+o.W) - max(r.X, o.X)
	h := min(r.Y+r.H, o.Y+o.H) - max(r.Y, o.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// part is the cell of a grid over a. Edges come from the same sums on both sides of a line, so
// windows side by side meet without a gap whatever the rounding.
func part(a Rect, col, cols, row, rows int) Rect {
	x0, x1 := a.X+a.W*col/cols, a.X+a.W*(col+1)/cols
	y0, y1 := a.Y+a.H*row/rows, a.Y+a.H*(row+1)/rows
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// third is one of three parts of a, side by side on a landscape screen, stacked on a portrait
// one, where three columns would be too narrow to read.
func third(a Rect, i int) Rect {
	if a.H > a.W {
		return part(a, 0, 1, i, 3)
	}
	return part(a, i, 3, 0, 1)
}

// centered is a rectangle of size w×h in the middle of a, no larger than a.
func centered(a Rect, w, h int) Rect {
	w, h = min(w, a.W), min(h, a.H)
	return Rect{X: a.X + (a.W-w)/2, Y: a.Y + (a.H-h)/2, W: w, H: h}
}

// almostMaximized leaves a margin all around, showing what lies behind.
func almostMaximized(a Rect) Rect { return centered(a, a.W*9/10, a.H*9/10) }

// displayOf is the display holding most of frame, the first when frame is on none.
func displayOf(displays []Display, frame Rect) Display {
	best, area := displays[0], 0
	for _, d := range displays {
		if o := frame.overlap(d.Work); o > area {
			best, area = d, o
		}
	}
	return best
}

// fraction is a rectangle as a share of a display, which fits the display's size at any time.
type fraction struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func fractionOf(r, a Rect) fraction {
	return fraction{
		X: float64(r.X-a.X) / float64(a.W), Y: float64(r.Y-a.Y) / float64(a.H),
		W: float64(r.W) / float64(a.W), H: float64(r.H) / float64(a.H),
	}
}

// in is the rectangle at this share of a.
func (f fraction) in(a Rect) Rect {
	return Rect{
		X: a.X + round(f.X*float64(a.W)), Y: a.Y + round(f.Y*float64(a.H)),
		W: round(f.W * float64(a.W)), H: round(f.H * float64(a.H)),
	}
}

func round(v float64) int {
	if v < 0 {
		return int(v - 0.5)
	}
	return int(v + 0.5)
}
