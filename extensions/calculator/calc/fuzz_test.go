package calc

import "testing"

// FuzzEval feeds the calculator anything typed in the root search: it answers or refuses, and
// never panics.
func FuzzEval(f *testing.F) {
	for _, s := range []string{
		"150 + 10%", "20% of 150", "255 in hex", "10 km in mi", "100 f to c", "1,5 * 2", "2^3^2",
		"10 usd in brl", "R$ 30 em dólares", "(((1)))", "-(-(-1))", "1e308 * 10", "0x1F + 0b101",
		"sqrt(16)", "5 mod 3", "3(4)", "1/0", "%%%", "^^", "10 in in in",
	} {
		f.Add(s, "en")
		f.Add(s, "pt-BR")
	}
	f.Fuzz(func(_ *testing.T, input, lang string) {
		_, _ = EvalWith(input, Options{Lang: lang, Rates: Rates{"USD": 1, "BRL": 5, "EUR": 0.9}, Currency: "BRL"})
	})
}
