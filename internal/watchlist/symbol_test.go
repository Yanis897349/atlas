package watchlist

import "testing"

func TestNormalizeInstrumentSymbol(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   string
	}{
		{name: "already canonical", symbol: "EURUSD", want: "EURUSD"},
		{name: "trim and uppercase", symbol: " spy ", want: "SPY"},
		{name: "preserve punctuation", symbol: " brk.b ", want: "BRK.B"},
		{name: "Unicode uppercase", symbol: " éur ", want: "ÉUR"},
		{name: "blank", symbol: " \t\n", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeInstrumentSymbol(test.symbol); got != test.want {
				t.Errorf("NormalizeInstrumentSymbol(%q) = %q, want %q", test.symbol, got, test.want)
			}
		})
	}
}
