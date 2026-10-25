package calc_test

import (
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/calculator/calc"
)

func TestEval(t *testing.T) {
	tests := []struct {
		in, lang, want string
	}{
		// Arithmetic, precedence, powers to the right.
		{"2+2", "en", "4"},
		{"2 + 3 * 4", "en", "14"},
		{"(2 + 3) * 4", "en", "20"},
		{"2^3^2", "en", "512"},
		{"2**10", "en", "1,024"},
		{"-3^2", "en", "-9"},
		{"10 / 4", "en", "2.5"},
		{"7 mod 3", "en", "1"},
		{"5!", "en", "120"},
		{"0.1 + 0.2", "en", "0.3"},
		{"1/3", "en", "0.3333333333"},
		{"6 × 7 ÷ 2", "en", "21"},
		{"1e3 * 2", "en", "2,000"},
		// Percentages, as people say them.
		{"150 + 10%", "en", "165"},
		{"150 - 10%", "en", "135"},
		{"20% of 150", "en", "30"},
		{"20% de 150", "pt-BR", "30"},
		{"50%", "en", "0.5"},
		// Functions and constants.
		{"sqrt(16) + abs(-2)", "en", "6"},
		{"round(2.567)", "en", "3"},
		{"log(1000)", "en", "3"},
		{"ln(e)", "en", "1"},
		{"2pi", "en", "6.283185307"},
		{"sin(pi/2)", "en", "1"},
		// Decimal commas, and the output in the user's language.
		{"1,5 + 1,5", "pt-BR", "3"},
		{"1234.5 * 1", "pt-BR", "1.234,5"},
		{"1234,5 * 2", "pt-BR", "2.469"},
		{"1234.5 * 2", "en", "2,469"},
		// Bases.
		{"0xff + 1", "en", "256"},
		{"0b1010", "en", "10"},
		{"255 in hex", "en", "0xFF"},
		{"10 to bin", "en", "0b1010"},
		{"0xff em decimal", "pt-BR", "255"},
		// Units.
		{"10 km in mi", "en", "6.213711922 mi"},
		{"1 km + 500 m", "en", "1.5 km"},
		{"100 f to c", "en", "37.77777778 °C"},
		{"0 °C in °F", "en", "32 °F"},
		{"2 GB in MB", "en", "2,048 MB"},
		{"90 min in h", "en", "1.5 h"},
		{"5 kg em lb", "pt-BR", "11,02311311 lb"},
		{"3 * 2 m", "en", "6 m"},
	}
	for _, tc := range tests {
		got, err := calc.Eval(tc.in, tc.lang)
		if err != nil || got.Text != tc.want {
			t.Errorf("Eval(%q, %s) = %q, %v; want %q", tc.in, tc.lang, got.Text, err, tc.want)
		}
	}
}

func TestEvalRejects(t *testing.T) {
	for _, in := range []string{
		"", "hello", "2 +", "(2 + 3", "1 / 0", "sqrt(-1)", "1 km + 2 kg", "5 kg in km", "2 3",
		"chrome", "notes 2", "1 in parsecs",
	} {
		if got, err := calc.Eval(in, "en"); err == nil {
			t.Errorf("Eval(%q) = %q, want an error", in, got.Text)
		}
	}
}

func TestLooksLikeMath(t *testing.T) {
	for in, want := range map[string]bool{
		"2+2": true, "10 km in mi": true, "sqrt(2)": true, "0xff": true, "20% of 150": true,
		"7": false, "chrome": false, "windows 11": false, "": false, "c++": false,
	} {
		if got := calc.LooksLikeMath(in); got != want {
			t.Errorf("LooksLikeMath(%q) = %v, want %v", in, got, want)
		}
	}
}
