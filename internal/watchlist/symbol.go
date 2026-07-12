package watchlist

import "strings"

// NormalizeInstrumentSymbol returns the canonical stored representation of an instrument symbol.
func NormalizeInstrumentSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
