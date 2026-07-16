package bls

import "testing"

func TestFormatPercentageChangeRoundsHalfAwayFromZero(t *testing.T) {
	tests := []struct {
		name    string
		current string
		earlier string
		want    string
	}{
		{name: "positive half", current: "100.05", earlier: "100", want: "0.1%"},
		{name: "negative half", current: "99.95", earlier: "100", want: "-0.1%"},
		{name: "positive", current: "101.25", earlier: "100", want: "1.3%"},
		{name: "negative", current: "98.75", earlier: "100", want: "-1.3%"},
		{name: "zero", current: "100", earlier: "100", want: "0.0%"},
		{name: "negative zero", current: "99.99", earlier: "100", want: "0.0%"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current, err := parseDecimal(test.current)
			if err != nil {
				t.Fatalf("parse current decimal: %v", err)
			}
			earlier, err := parseDecimal(test.earlier)
			if err != nil {
				t.Fatalf("parse earlier decimal: %v", err)
			}
			got, err := formatPercentageChange(
				monthlyValue{value: current},
				monthlyValue{year: "2025", period: "M06", value: earlier},
			)
			if err != nil {
				t.Fatalf("formatPercentageChange() error = %v", err)
			}
			if got != test.want {
				t.Errorf("formatPercentageChange() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseDecimalRejectsNonCanonicalValues(t *testing.T) {
	for _, value := range []string{"", ".1", "1.", " 1", "1 ", "1e2", "--1"} {
		if _, err := parseDecimal(value); err == nil {
			t.Errorf("parseDecimal(%q) succeeded, want error", value)
		}
	}
}

func TestSubtractIntegersFormatsSignedChanges(t *testing.T) {
	tests := []struct {
		left, right string
		want        string
	}{
		{left: "200", right: "150", want: "+50"},
		{left: "150", right: "200", want: "-50"},
		{left: "200", right: "200", want: "0"},
	}
	for _, test := range tests {
		left, err := parseDecimal(test.left)
		if err != nil {
			t.Fatalf("parse left decimal: %v", err)
		}
		right, err := parseDecimal(test.right)
		if err != nil {
			t.Fatalf("parse right decimal: %v", err)
		}
		if got := subtractIntegers(left, right); got != test.want {
			t.Errorf("subtractIntegers(%s, %s) = %q, want %q", test.left, test.right, got, test.want)
		}
	}
}
