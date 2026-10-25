package calc

import "strings"

// dimension is what a unit measures; values add only within one.
type dimension int

const (
	length dimension = iota + 1
	mass
	temperature
	data
	duration
)

// unit converts to its dimension's base unit: base = value*factor + offset.
type unit struct {
	symbol string
	dim    dimension
	factor float64
	offset float64
}

func (u *unit) toBase(v float64) float64   { return v*u.factor + u.offset }
func (u *unit) fromBase(v float64) float64 { return (v - u.offset) / u.factor }

var (
	celsius    = &unit{"°C", temperature, 1, 0}
	fahrenheit = &unit{"°F", temperature, 5.0 / 9, -32 * 5.0 / 9}
	kelvin     = &unit{"K", temperature, 1, -273.15}
)

// units are known by every name people type for them, in English and Portuguese. "in" is no
// unit here: it converts ("10 km in mi"); inches are "inch" or "pol".
var units = func() map[string]*unit {
	m := map[string]*unit{}
	add := func(u *unit, names ...string) {
		for _, n := range append(names, strings.ToLower(u.symbol)) {
			m[n] = u
		}
	}
	add(&unit{"m", length, 1, 0}, "meter", "meters", "metre", "metres", "metro", "metros") //nolint:misspell // British spelling, on purpose
	add(&unit{"km", length, 1000, 0}, "kilometer", "kilometers", "quilometro", "quilometros", "quilômetro", "quilômetros")
	add(&unit{"cm", length, 0.01, 0}, "centimeter", "centimeters", "centimetro", "centimetros")
	add(&unit{"mm", length, 0.001, 0}, "millimeter", "millimeters", "milimetro", "milimetros")
	add(&unit{"mi", length, 1609.344, 0}, "mile", "miles", "milha", "milhas")
	add(&unit{"yd", length, 0.9144, 0}, "yard", "yards", "jarda", "jardas")
	add(&unit{"ft", length, 0.3048, 0}, "foot", "feet", "pe", "pes", "pé", "pés")
	add(&unit{"inch", length, 0.0254, 0}, "inches", "pol", "polegada", "polegadas")

	add(&unit{"kg", mass, 1, 0}, "kilogram", "kilograms", "quilo", "quilos")
	add(&unit{"g", mass, 0.001, 0}, "gram", "grams", "grama", "gramas")
	add(&unit{"mg", mass, 1e-6, 0}, "milligram", "milligrams")
	add(&unit{"t", mass, 1000, 0}, "tonne", "tonnes", "tonelada", "toneladas")
	add(&unit{"lb", mass, 0.45359237, 0}, "lbs", "pound", "pounds", "libra", "libras")
	add(&unit{"oz", mass, 0.028349523125, 0}, "ounce", "ounces", "onça", "onças")

	add(celsius, "c", "celsius")
	add(fahrenheit, "f", "fahrenheit")
	add(kelvin, "k", "kelvin")

	add(&unit{"B", data, 1, 0}, "byte", "bytes")
	add(&unit{"KB", data, 1 << 10, 0}, "kilobyte", "kilobytes")
	add(&unit{"MB", data, 1 << 20, 0}, "megabyte", "megabytes")
	add(&unit{"GB", data, 1 << 30, 0}, "gigabyte", "gigabytes")
	add(&unit{"TB", data, 1 << 40, 0}, "terabyte", "terabytes")

	add(&unit{"ms", duration, 0.001, 0}, "millisecond", "milliseconds")
	add(&unit{"s", duration, 1, 0}, "sec", "secs", "second", "seconds", "segundo", "segundos")
	add(&unit{"min", duration, 60, 0}, "mins", "minute", "minutes", "minuto", "minutos")
	add(&unit{"h", duration, 3600, 0}, "hr", "hrs", "hour", "hours", "hora", "horas")
	add(&unit{"d", duration, 86400, 0}, "day", "days", "dia", "dias")
	add(&unit{"wk", duration, 7 * 86400, 0}, "week", "weeks", "semana", "semanas")
	return m
}()
