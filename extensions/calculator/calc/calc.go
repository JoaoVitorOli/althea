// Package calc evaluates what people type into a launcher as math: arithmetic, percentages as
// they say them ("150 + 10%", "20% of 150"), functions, bases ("255 in hex") and units
// ("10 km in mi") and currencies ("50 dollars in euros", by Rates the caller fetched). It reads
// decimal commas and writes numbers as the user's language does.
package calc

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Result is an evaluated expression.
type Result struct {
	Value float64
	Unit  string // the unit's symbol, or a currency's code, if any
	Text  string // the value written for the user's language, with its unit or base
	Plain string // the value without grouping, for the clipboard
	// Rate is the exchange rate of a conversion from one currency, nil otherwise.
	Rate *Rate
}

// Options are how EvalWith reads and writes.
type Options struct {
	Lang string // "en" or "pt-BR"
	// Rates convert currencies; while nil, an expression with one fails with ErrNeedRates.
	Rates Rates
	// Currency is what an amount is converted to when the expression names no other, as BRL of
	// "50 dollars"; an amount already in it goes to US dollars, or to euros from US dollars.
	Currency string
}

// Eval evaluates an expression for a language ("en", "pt-BR"), without currencies.
func Eval(input, lang string) (Result, error) { return EvalWith(input, Options{Lang: lang}) }

// maxTokens is the most numbers, words and operators an expression may have.
const maxTokens = 256

// EvalWith evaluates an expression.
func EvalWith(input string, o Options) (Result, error) {
	toks, err := lex(input)
	if err != nil {
		return Result{}, err
	}
	// The parser recurses for each parenthesis and power: an expression this long is no
	// calculation, and one nested deeper could exhaust the stack.
	if len(toks) > maxTokens {
		return Result{}, errors.New("too long to calculate")
	}
	p := &parser{toks: toks, opts: o}
	v, err := p.expr()
	if err != nil {
		return Result{}, err
	}
	base, converted, err := p.conversion(&v)
	if err != nil {
		return Result{}, err
	}
	if p.pos < len(p.toks) {
		return Result{}, fmt.Errorf("unexpected %q", p.toks[p.pos].text)
	}
	if p.needRates {
		return Result{}, ErrNeedRates
	}
	if p.missing != "" {
		return Result{}, fmt.Errorf("no rate for %s", p.missing)
	}
	if v.unit != nil && v.unit.dim == currency && !converted {
		p.toDefaultCurrency(&v)
	}
	if math.IsNaN(v.n) || math.IsInf(v.n, 0) {
		return Result{}, errors.New("no number")
	}
	r := format(v, base, o.Lang)
	r.Rate = p.rate(v)
	return r, nil
}

// LooksLikeMath reports whether input is math worth answering: an expression that evaluates,
// beyond a lone number.
func LooksLikeMath(input string) bool {
	toks, err := lex(input)
	if err != nil || len(toks) == 0 {
		return false
	}
	if len(toks) == 1 && toks[0].kind == number && !toks[0].based {
		return false
	}
	_, err = Eval(input, "en")
	return err == nil || errors.Is(err, ErrNeedRates)
}

// value is a number, with a unit, or a percentage waiting for what it is a percentage of.
type value struct {
	n       float64
	unit    *unit
	percent bool
}

// Lexing.

type kind int

const (
	number kind = iota + 1
	ident
	op
)

type token struct {
	kind  kind
	text  string
	n     float64
	based bool // written in hex, binary or octal
}

func lex(s string) ([]token, error) {
	var toks []token
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '0' && i+1 < len(rs) && strings.ContainsRune("xXbBoO", rs[i+1]):
			j := i + 2
			for j < len(rs) && (unicode.IsDigit(rs[j]) || strings.ContainsRune("abcdefABCDEF", rs[j])) {
				j++
			}
			base := map[rune]int{'x': 16, 'b': 2, 'o': 8}[unicode.ToLower(rs[i+1])]
			n, err := strconv.ParseInt(string(rs[i+2:j]), base, 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", string(rs[i:j]))
			}
			toks = append(toks, token{kind: number, text: string(rs[i:j]), n: float64(n), based: true})
			i = j
		case unicode.IsDigit(r) || (r == '.' && i+1 < len(rs) && unicode.IsDigit(rs[i+1])):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || ((rs[j] == '.' || rs[j] == ',') && j+1 < len(rs) && unicode.IsDigit(rs[j+1]))) {
				j++
			}
			// An exponent: 1e3, 2.5E-4 (but 2e alone is 2 times e).
			if j+1 < len(rs) && (rs[j] == 'e' || rs[j] == 'E') {
				k := j + 1
				if k < len(rs) && (rs[k] == '+' || rs[k] == '-') {
					k++
				}
				if k < len(rs) && unicode.IsDigit(rs[k]) {
					for k < len(rs) && unicode.IsDigit(rs[k]) {
						k++
					}
					j = k
				}
			}
			text := strings.ReplaceAll(string(rs[i:j]), ",", ".")
			n, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", string(rs[i:j]))
			}
			toks = append(toks, token{kind: number, text: string(rs[i:j]), n: n})
			i = j
		case unicode.IsLetter(r) || r == '°' || r == 'µ' || r == 'π':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '°' || rs[j] == 'µ') {
				j++
			}
			if j < len(rs) && rs[j] == '$' { // R$, US$
				j++
			}
			toks = append(toks, token{kind: ident, text: strings.ToLower(string(rs[i:j]))})
			i = j
		case strings.ContainsRune("$€£¥₹₩₽₺₪₦₱₫₴", r):
			toks = append(toks, token{kind: ident, text: string(r)})
			i++
		case r == '*' && i+1 < len(rs) && rs[i+1] == '*':
			toks = append(toks, token{kind: op, text: "^"})
			i += 2
		case strings.ContainsRune("+-*/^%!()×÷−", r):
			t := string(r)
			switch r {
			case '×':
				t = "*"
			case '÷':
				t = "/"
			case '−':
				t = "-"
			}
			toks = append(toks, token{kind: op, text: t})
			i++
		default:
			return nil, fmt.Errorf("unexpected %q", string(r))
		}
	}
	return toks, nil
}

// Parsing and evaluating, at once.

type parser struct {
	toks []token
	pos  int
	opts Options
	// currencies are the codes of the currencies read, needRates whether one was read without
	// rates, and missing one the rates do not have.
	currencies []string
	needRates  bool
	missing    string
}

func (p *parser) peek() (token, bool) {
	if p.pos >= len(p.toks) {
		return token{}, false
	}
	return p.toks[p.pos], true
}

func (p *parser) isOp(s string) bool {
	t, ok := p.peek()
	return ok && t.kind == op && t.text == s
}

func (p *parser) isWord(words ...string) bool {
	t, ok := p.peek()
	if !ok || t.kind != ident {
		return false
	}
	for _, w := range words {
		if t.text == w {
			return true
		}
	}
	return false
}

// expr := term (("+" | "-") term)*. A percentage added to a number is that much of it.
func (p *parser) expr() (value, error) {
	left, err := p.term()
	if err != nil {
		return value{}, err
	}
	for p.isOp("+") || p.isOp("-") {
		sign := 1.0
		if p.toks[p.pos].text == "-" {
			sign = -1
		}
		p.pos++
		right, err := p.term()
		if err != nil {
			return value{}, err
		}
		switch {
		case right.percent && !left.percent:
			left.n += sign * left.n * right.n
		default:
			sum, err := add(left, right, sign)
			if err != nil {
				return value{}, err
			}
			left = sum
		}
	}
	return left, nil
}

func add(a, b value, sign float64) (value, error) {
	switch {
	case a.unit == nil && b.unit == nil:
		return value{n: a.n + sign*b.n, percent: a.percent && b.percent}, nil
	case a.unit != nil && b.unit != nil && a.unit.dim == b.unit.dim:
		return value{n: a.unit.fromBase(a.unit.toBase(a.n) + sign*b.unit.toBase(b.n)), unit: a.unit}, nil
	}
	return value{}, errors.New("cannot add these units")
}

// term := power (("*" | "/" | "mod" | implicit) power)*
func (p *parser) term() (value, error) {
	left, err := p.power()
	if err != nil {
		return value{}, err
	}
	for {
		var operator string
		switch {
		case p.isOp("*"), p.isOp("/"):
			operator = p.toks[p.pos].text
			p.pos++
		case p.isWord("mod"):
			operator = "mod"
			p.pos++
		case p.implicit():
			operator = "*"
		default:
			return left, nil
		}
		right, err := p.power()
		if err != nil {
			return value{}, err
		}
		if left, err = multiply(left, right, operator); err != nil {
			return value{}, err
		}
	}
}

// implicit reports a multiplication without its sign, as "2pi" or "3(4+1)".
func (p *parser) implicit() bool {
	t, ok := p.peek()
	if !ok {
		return false
	}
	if t.kind == op {
		return t.text == "("
	}
	_, isFunc := functions[t.text]
	_, isConst := constants[t.text]
	return t.kind == ident && (isFunc || isConst)
}

func multiply(a, b value, operator string) (value, error) {
	if a.unit != nil && b.unit != nil {
		return value{}, errors.New("cannot multiply units")
	}
	u := a.unit
	if u == nil {
		u = b.unit
	}
	switch operator {
	case "*":
		return value{n: a.n * b.n, unit: u}, nil
	case "/":
		if b.unit != nil {
			return value{}, errors.New("cannot divide by a unit")
		}
		if b.n == 0 {
			return value{}, errors.New("division by zero")
		}
		return value{n: a.n / b.n, unit: u}, nil
	default:
		if b.n == 0 {
			return value{}, errors.New("division by zero")
		}
		return value{n: math.Mod(a.n, b.n), unit: u}, nil
	}
}

// power := unary ("^" power)?
func (p *parser) power() (value, error) {
	base, err := p.unary()
	if err != nil {
		return value{}, err
	}
	if !p.isOp("^") {
		return base, nil
	}
	p.pos++
	exp, err := p.power()
	if err != nil {
		return value{}, err
	}
	if base.unit != nil || exp.unit != nil {
		return value{}, errors.New("cannot raise units")
	}
	return value{n: math.Pow(base.n, exp.n)}, nil
}

// unary := "-" unary | postfix. The sign binds looser than "^": -3^2 is -9.
func (p *parser) unary() (value, error) {
	if p.isOp("-") || p.isOp("+") {
		neg := p.toks[p.pos].text == "-"
		p.pos++
		v, err := p.power()
		if neg {
			v.n = -v.n
		}
		return v, err
	}
	return p.postfix()
}

// postfix := primary ("!" | "%" ["of" unary])*
func (p *parser) postfix() (value, error) {
	v, err := p.primary()
	if err != nil {
		return value{}, err
	}
	for {
		switch {
		case p.isOp("!"):
			p.pos++
			if v.n < 0 || v.n > 170 || v.n != math.Trunc(v.n) {
				return value{}, errors.New("factorial of a whole number up to 170")
			}
			f := 1.0
			for i := 2.0; i <= v.n; i++ {
				f *= i
			}
			v.n = f
		case p.isOp("%"):
			p.pos++
			v = value{n: v.n / 100, percent: true}
			if p.isWord("of", "de") {
				p.pos++
				of, err := p.unary()
				if err != nil {
					return value{}, err
				}
				v = value{n: v.n * of.n, unit: of.unit}
			}
		default:
			return v, nil
		}
	}
}

var functions = map[string]func(float64) float64{
	"sqrt": math.Sqrt, "raiz": math.Sqrt, "cbrt": math.Cbrt, "abs": math.Abs,
	"round": math.Round, "floor": math.Floor, "ceil": math.Ceil,
	"ln": math.Log, "log": math.Log10, "log2": math.Log2, "exp": math.Exp,
	"sin": math.Sin, "cos": math.Cos, "tan": math.Tan, "sen": math.Sin,
	"asin": math.Asin, "acos": math.Acos, "atan": math.Atan,
}

var constants = map[string]float64{"pi": math.Pi, "π": math.Pi, "e": math.E}

// primary := number [unit] | function "(" expr ")" | constant | "(" expr ")"
func (p *parser) primary() (value, error) {
	t, ok := p.peek()
	if !ok {
		return value{}, errors.New("incomplete expression")
	}
	p.pos++
	switch {
	case t.kind == number:
		v := value{n: t.n}
		if u, ok := p.unitNext(); ok {
			v.unit = u
		} else if u, ok := p.currencyAt(); ok {
			v.unit = u
		}
		return v, nil
	case t.kind == op && t.text == "(":
		v, err := p.expr()
		if err != nil {
			return value{}, err
		}
		if !p.isOp(")") {
			return value{}, errors.New("missing )")
		}
		p.pos++
		return v, nil
	case t.kind == ident:
		// A currency before its amount: "$50", "R$ 10,50".
		p.pos--
		if u, ok := p.currencyAt(); ok {
			if n, ok := p.peek(); ok && n.kind == number && !n.based {
				p.pos++
				return value{n: n.n, unit: u}, nil
			}
			return value{}, errors.New("a currency needs an amount")
		}
		p.pos++
		if f, ok := functions[t.text]; ok {
			if !p.isOp("(") {
				return value{}, fmt.Errorf("%s needs (…)", t.text)
			}
			arg, err := p.primary()
			if err != nil {
				return value{}, err
			}
			r := f(arg.n)
			if math.IsNaN(r) {
				return value{}, fmt.Errorf("%s of %g", t.text, arg.n)
			}
			// Sines of whole fractions of pi land a hair off 0 and 1: round them back.
			return value{n: clean(r)}, nil
		}
		if c, ok := constants[t.text]; ok {
			return value{n: c}, nil
		}
	}
	return value{}, fmt.Errorf("unexpected %q", t.text)
}

func (p *parser) unitNext() (*unit, bool) {
	t, ok := p.peek()
	if !ok || t.kind != ident {
		return nil, false
	}
	u, ok := units[t.text]
	if ok {
		p.pos++
	}
	return u, ok
}

// conversion reads "in <unit>", "in hex" or "to <currency>" at the end, converting v, and
// returns the base to write the result in (10 by default) and whether it converted.
func (p *parser) conversion(v *value) (int, bool, error) {
	if !p.isWord("in", "to", "as", "em", "para") {
		return 10, false, nil
	}
	p.pos++
	t, ok := p.peek()
	if !ok || t.kind != ident {
		return 0, false, errors.New("convert to what?")
	}
	if _, isUnit := units[t.text]; !isUnit {
		if u, ok := p.currencyAt(); ok {
			if v.unit == nil || v.unit.dim != currency {
				return 0, false, errors.New("cannot convert between these units")
			}
			v.n, v.unit = u.fromBase(v.unit.toBase(v.n)), u
			return 10, true, nil
		}
	}
	p.pos++
	if base, ok := map[string]int{
		"hex": 16, "hexadecimal": 16, "bin": 2, "binary": 2, "binario": 2, "binário": 2,
		"oct": 8, "octal": 8, "dec": 10, "decimal": 10,
	}[t.text]; ok {
		if v.unit != nil || v.n != math.Trunc(v.n) {
			return 0, false, errors.New("bases are for whole numbers")
		}
		return base, true, nil
	}
	u, ok := units[t.text]
	if !ok {
		return 0, false, fmt.Errorf("unknown unit %q", t.text)
	}
	if v.unit == nil || v.unit.dim != u.dim {
		return 0, false, errors.New("cannot convert between these units")
	}
	v.n, v.unit = u.fromBase(v.unit.toBase(v.n)), u
	return 10, true, nil
}

// clean rounds away the float noise beyond 12 significant digits.
func clean(f float64) float64 {
	if f == 0 || math.IsInf(f, 0) {
		return f
	}
	r, _ := strconv.ParseFloat(strconv.FormatFloat(f, 'g', 12, 64), 64)
	return r
}

// Formatting.

func format(v value, base int, lang string) Result {
	r := Result{Value: v.n}
	if v.unit != nil {
		r.Unit = v.unit.symbol
	}
	if base != 10 {
		prefix := map[int]string{16: "0x", 2: "0b", 8: "0o"}[base]
		r.Text = prefix + strings.ToUpper(strconv.FormatInt(int64(v.n), base))
		r.Plain = r.Text
		return r
	}
	if v.unit != nil && v.unit.dim == currency {
		return formatMoney(r, v.n, v.unit.symbol, lang)
	}
	n := v.n
	if n == 0 {
		n = 0 // no "-0"
	}
	// Ten significant digits, written out unless the number is huge or tiny.
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(n, 'g', 10, 64), 64)
	decimal, group := ".", ","
	if lang == "pt-BR" {
		decimal, group = ",", "."
	}
	var text string
	if a := math.Abs(rounded); a != 0 && (a >= 1e15 || a < 1e-9) {
		text = strings.Replace(strconv.FormatFloat(rounded, 'g', 10, 64), ".", decimal, 1)
		r.Plain = text
	} else {
		plain := strconv.FormatFloat(rounded, 'f', -1, 64)
		intPart, frac, _ := strings.Cut(plain, ".")
		r.Plain = intPart
		if frac != "" {
			r.Plain += decimal + frac
		}
		text = groupDigits(intPart, group)
		if frac != "" {
			text += decimal + frac
		}
	}
	if r.Unit != "" {
		text += " " + r.Unit
	}
	r.Text = text
	return r
}

// formatMoney writes an amount with its currency's cents, more for an amount under one that
// would round to nothing: "R$ 1.234,50", "$0.0067", "12.50 CHF".
func formatMoney(r Result, n float64, code, lang string) Result {
	digits, ok := minorDigits[code]
	if !ok {
		digits = 2
	}
	if a := math.Abs(n); a > 0 && a < 1 {
		for digits < 8 && a*math.Pow(10, float64(digits)) < 10 {
			digits++
		}
	}
	decimal, group := ".", ","
	if lang == "pt-BR" {
		decimal, group = ",", "."
	}
	plain := strconv.FormatFloat(n, 'f', digits, 64)
	if strings.Trim(plain, "-0.") == "" {
		plain = strings.TrimPrefix(plain, "-") // no "-0.00"
	}
	intPart, frac, _ := strings.Cut(plain, ".")
	r.Plain = intPart
	amount := groupDigits(intPart, group)
	if frac != "" {
		r.Plain += decimal + frac
		amount += decimal + frac
	}
	switch sym, ok := symbol(code, lang); {
	case !ok:
		r.Text = amount + " " + code
	case lang == "pt-BR" || len([]rune(sym)) > 1:
		r.Text = sym + " " + amount // as "R$ 10,50", never broken apart
	default:
		r.Text = sym + amount
	}
	return r
}

func groupDigits(s, sep string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
