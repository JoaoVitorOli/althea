package calc

import (
	"errors"
	"strings"
)

// Rates are exchange rates: how much of each currency, by ISO 4217 code, one US dollar buys.
type Rates map[string]float64

// ErrNeedRates is the error of an expression with a currency, evaluated without Rates.
var ErrNeedRates = errors.New("calc: exchange rates needed")

// Rate is the exchange rate behind a conversion: one From buys Of To.
type Rate struct {
	From, To string
	Of       float64
}

// currencyNames are the names people type for currencies, in English and Portuguese, lowercase,
// beyond their codes. "pound" alone weighs, as in "2 pounds in kg".
var currencyNames = func() map[string]string {
	m := map[string]string{}
	add := func(code string, names ...string) {
		for _, n := range names {
			m[n] = code
		}
	}
	add("USD", "$", "us$", "dollar", "dollars", "dólar", "dólares", "dolar", "dolares",
		"us dollar", "us dollars", "american dollar", "american dollars", "dólar americano",
		"dólares americanos", "dolar americano", "dolares americanos")
	add("BRL", "r$", "real", "reais", "brazilian real", "brazilian reais", "brazilian reals",
		"real brasileiro", "reais brasileiros")
	add("EUR", "€", "euro", "euros")
	add("GBP", "£", "british pound", "british pounds", "pound sterling", "pounds sterling",
		"libra esterlina", "libras esterlinas", "sterling")
	add("JPY", "¥", "yen", "iene", "ienes", "japanese yen", "iene japonês")
	add("CNY", "yuan", "renminbi", "iuan", "iuane", "chinese yuan", "iuan chinês")
	add("CAD", "c$", "canadian dollar", "canadian dollars", "dólar canadense", "dólares canadenses",
		"dolar canadense")
	add("AUD", "a$", "australian dollar", "australian dollars", "dólar australiano",
		"dólares australianos", "dolar australiano") //nolint:misspell // Portuguese
	add("CHF", "swiss franc", "swiss francs", "franco suíço", "francos suíços", "franco suico")
	add("ARS", "argentine peso", "argentine pesos", "peso argentino", "pesos argentinos")
	add("MXN", "mexican peso", "mexican pesos", "peso mexicano", "pesos mexicanos")
	add("CLP", "chilean peso", "chilean pesos", "peso chileno", "pesos chilenos")
	add("COP", "colombian peso", "colombian pesos", "peso colombiano", "pesos colombianos")
	add("UYU", "uruguayan peso", "uruguayan pesos", "peso uruguaio", "pesos uruguaios")
	add("PYG", "guarani", "guaranis", "guaraní", "guaraníes")
	add("INR", "₹", "rupee", "rupees", "rúpia", "rúpias", "indian rupee", "indian rupees", "rúpia indiana")
	add("KRW", "₩", "won", "korean won", "won sul-coreano")
	add("RUB", "₽", "ruble", "rubles", "rouble", "roubles", "rublo", "rublos")
	add("TRY", "₺", "lira", "liras", "turkish lira", "lira turca")
	add("ILS", "₪", "shekel", "shekels", "siclo", "siclos")
	add("NGN", "₦", "naira", "nairas")
	add("PHP", "₱", "philippine peso", "philippine pesos", "peso filipino")
	add("VND", "₫", "dong", "dongs")
	add("UAH", "₴", "hryvnia", "hryvnias")
	add("PLN", "zloty", "zlotys", "złoty", "zlótis")
	add("SEK", "swedish krona", "swedish kronor", "coroa sueca", "coroas suecas")
	add("NOK", "norwegian krone", "norwegian kroner", "coroa norueguesa", "coroas norueguesas")
	add("DKK", "danish krone", "danish kroner", "coroa dinamarquesa", "coroas dinamarquesas")
	add("ZAR", "rand", "rands")
	add("AOA", "kwanza", "kwanzas")
	add("MZN", "metical", "meticais")
	return m
}()

// isoCodes are the ISO 4217 codes of the currencies in use, for telling one before rates are at
// hand. With rates, the rates' own codes count too.
var isoCodes = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range strings.Fields(`AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF
		BMD BND BOB BRL BSD BTN BWP BYN BZD CAD CDF CHF CLP CNY COP CRC CUP CVE CZK DJF DKK DOP DZD
		EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR
		ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD
		MMK MNT MOP MRU MUR MVR MWK MXN MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN
		PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SYP SZL THB TJS TMT
		TND TOP TRY TTD TWD TZS UAH UGX USD UYU UZS VES VND VUV WST XAF XCD XOF XPF YER ZAR ZMW ZWL`) {
		m[c] = true
	}
	return m
}()

// maxNameWords is the most words a currency's name has, as "dólares americanos".
const maxNameWords = 2

// currencyAt reads a currency at the parser's position: a code, a symbol or a name of up to
// maxNameWords words, the longest that fits. It moves past it.
func (p *parser) currencyAt() (*unit, bool) {
	for n := maxNameWords; n >= 1; n-- {
		if p.pos+n > len(p.toks) {
			continue
		}
		words := make([]string, 0, n)
		for _, t := range p.toks[p.pos : p.pos+n] {
			if t.kind != ident {
				break
			}
			words = append(words, t.text)
		}
		if len(words) < n {
			continue
		}
		code, ok := currencyNames[strings.Join(words, " ")]
		if !ok && n == 1 {
			code, ok = p.isCode(words[0])
		}
		if !ok {
			continue
		}
		p.pos += n
		return p.currency(code), true
	}
	return nil, false
}

func (p *parser) isCode(word string) (string, bool) {
	code := strings.ToUpper(word)
	if len(code) != 3 {
		return "", false
	}
	if _, ok := p.opts.Rates[code]; ok {
		return code, true
	}
	return code, isoCodes[code]
}

// currency is the unit of a currency, in US dollars. Without rates, or without this currency's,
// the expression cannot be evaluated: the parser goes on, to tell whether it was otherwise one.
func (p *parser) currency(code string) *unit {
	p.currencies = append(p.currencies, code)
	rate, ok := p.opts.Rates[code]
	if !ok || rate <= 0 {
		if p.opts.Rates == nil {
			p.needRates = true
		} else {
			p.missing = code
		}
		rate = 1
	}
	return &unit{symbol: code, dim: currency, factor: 1 / rate}
}

// minorDigits are the digits after the decimal point of currencies that have no cents, by code;
// the others have two.
var minorDigits = map[string]int{
	"JPY": 0, "KRW": 0, "CLP": 0, "ISK": 0, "VND": 0, "PYG": 0, "UGX": 0, "XAF": 0, "XOF": 0,
	"XPF": 0, "KMF": 0, "RWF": 0, "VUV": 0, "GNF": 0, "BIF": 0, "DJF": 0,
	"BHD": 3, "KWD": 3, "OMR": 3, "JOD": 3, "TND": 3, "IQD": 3, "LYD": 3,
}

// symbols are what an amount of a currency is written with, before it, for a language; the
// others are written after, by code: "12.50 CHF".
func symbol(code, lang string) (string, bool) {
	switch code {
	case "BRL":
		return "R$", true
	case "USD":
		if lang == "pt-BR" {
			return "US$", true
		}
		return "$", true
	case "EUR":
		return "€", true
	case "GBP":
		return "£", true
	case "JPY":
		return "¥", true
	case "INR":
		return "₹", true
	case "KRW":
		return "₩", true
	}
	return "", false
}

// toDefaultCurrency converts an amount the expression left in its own currency.
func (p *parser) toDefaultCurrency(v *value) {
	to := strings.ToUpper(p.opts.Currency)
	if _, ok := p.opts.Rates[to]; !ok {
		to = "USD"
	}
	if to == v.unit.symbol {
		to = "USD"
		if v.unit.symbol == "USD" {
			to = "EUR"
		}
	}
	if _, ok := p.opts.Rates[to]; !ok {
		return
	}
	u := p.currency(to)
	v.n, v.unit = u.fromBase(v.unit.toBase(v.n)), u
}

// rate is the exchange rate of a result converted from one other currency.
func (p *parser) rate(v value) *Rate {
	if v.unit == nil || v.unit.dim != currency {
		return nil
	}
	from := ""
	for _, c := range p.currencies {
		switch {
		case c == v.unit.symbol:
		case from == "" || from == c:
			from = c
		default:
			return nil // from more than one
		}
	}
	if from == "" {
		return nil
	}
	return &Rate{From: from, To: v.unit.symbol, Of: p.opts.Rates[v.unit.symbol] / p.opts.Rates[from]}
}
