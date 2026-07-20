package intelligence

import (
	"strings"
	"testing"
)

func TestObservationNumericDeltaFormatsExactCompatibleValues(t *testing.T) {
	tests := []struct {
		name   string
		oldRaw string
		newRaw string
		want   string
	}{
		{name: "positive percent", oldRaw: "3.00%", newRaw: "3.200%", want: "+0.2%"},
		{name: "negative integer", oldRaw: "+100", newRaw: "+50", want: "-50"},
		{name: "positive fraction", oldRaw: "1.25", newRaw: "1.500", want: "+0.25"},
		{name: "explicit signs", oldRaw: "-1.5", newRaw: "+0.5", want: "+2"},
		{name: "negative fraction", oldRaw: "0.125", newRaw: "-0.125", want: "-0.25"},
		{name: "numeric zero", oldRaw: "+1.0", newRaw: "1.00", want: "0"},
		{name: "percent zero", oldRaw: "0.00%", newRaw: "-0.0%", want: "0%"},
		{name: "maximum integer length", oldRaw: strings.Repeat("9", maxObservationNumericValueLength), newRaw: strings.Repeat("9", maxObservationNumericValueLength), want: "0"},
		{name: "maximum percent length", oldRaw: "0." + strings.Repeat("1", maxObservationNumericValueLength-3) + "%", newRaw: "0." + strings.Repeat("1", maxObservationNumericValueLength-3) + "%", want: "0%"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := observationNumericDelta(test.oldRaw, test.newRaw)
			if !valid || got == nil || *got != test.want {
				t.Fatalf("observationNumericDelta(%q, %q) = (%v, %t), want %q", test.oldRaw, test.newRaw, got, valid, test.want)
			}
		})
	}
}

func TestObservationNumericDeltaRejectsUnsupportedOrIncompatibleValues(t *testing.T) {
	tests := []struct {
		name   string
		oldRaw string
		newRaw string
	}{
		{name: "incompatible units", oldRaw: "3.0%", newRaw: "3.2"},
		{name: "grouped", oldRaw: "147,000", newRaw: "148,000"},
		{name: "exponent", oldRaw: "1e2", newRaw: "2e2"},
		{name: "leading whitespace", oldRaw: " 1", newRaw: "2"},
		{name: "trailing whitespace", oldRaw: "1", newRaw: "2 "},
		{name: "leading decimal point", oldRaw: ".1", newRaw: ".2"},
		{name: "trailing decimal point", oldRaw: "1.", newRaw: "2."},
		{name: "other unit", oldRaw: "1K", newRaw: "2K"},
		{name: "blank", oldRaw: "", newRaw: "1"},
		{name: "sign only", oldRaw: "+", newRaw: "1"},
		{name: "oversized integer", oldRaw: strings.Repeat("9", maxObservationNumericValueLength+1), newRaw: "1"},
		{name: "oversized scale", oldRaw: "0." + strings.Repeat("1", maxObservationNumericValueLength), newRaw: "0.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := observationNumericDelta(test.oldRaw, test.newRaw)
			if valid || got != nil {
				t.Fatalf("observationNumericDelta(%q, %q) = (%v, %t), want unsupported", test.oldRaw, test.newRaw, got, valid)
			}
		})
	}
}

func TestObservationNumericSurpriseCalculatesCompatibleValues(t *testing.T) {
	tests := []struct {
		name          string
		consensus     *string
		actual        *string
		want          *string
		wantDirection SurpriseDirection
	}{
		{
			name:          "positive percent",
			consensus:     observationNumericTestValue("3.10%"),
			actual:        observationNumericTestValue("3.3%"),
			want:          observationNumericTestValue("+0.2%"),
			wantDirection: SurpriseDirectionAboveConsensus,
		},
		{
			name:          "negative percent",
			consensus:     observationNumericTestValue("3.3%"),
			actual:        observationNumericTestValue("3.10%"),
			want:          observationNumericTestValue("-0.2%"),
			wantDirection: SurpriseDirectionBelowConsensus,
		},
		{
			name:          "positive unitless",
			consensus:     observationNumericTestValue("100"),
			actual:        observationNumericTestValue("125.50"),
			want:          observationNumericTestValue("+25.5"),
			wantDirection: SurpriseDirectionAboveConsensus,
		},
		{
			name:          "zero",
			consensus:     observationNumericTestValue("+1.0"),
			actual:        observationNumericTestValue("1.00"),
			want:          observationNumericTestValue("0"),
			wantDirection: SurpriseDirectionInLine,
		},
		{
			name:          "percent signed zero",
			consensus:     observationNumericTestValue("+0.00%"),
			actual:        observationNumericTestValue("-0.0%"),
			want:          observationNumericTestValue("0%"),
			wantDirection: SurpriseDirectionInLine,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, gotDirection := observationNumericSurprise(test.consensus, test.actual)
			if got == nil || *got != *test.want || gotDirection == nil || *gotDirection != test.wantDirection {
				t.Fatalf(
					"observationNumericSurprise(%v, %v) = (%v, %v), want (%q, %q)",
					test.consensus,
					test.actual,
					got,
					gotDirection,
					*test.want,
					test.wantDirection,
				)
			}
		})
	}
}

func TestObservationNumericSurpriseIsUnavailableForMissingUnsupportedOrIncompatibleValues(t *testing.T) {
	tests := []struct {
		name      string
		consensus *string
		actual    *string
	}{
		{name: "missing consensus", actual: observationNumericTestValue("3.2%")},
		{name: "missing actual", consensus: observationNumericTestValue("3.0%")},
		{
			name:      "unsupported grouped values",
			consensus: observationNumericTestValue("147,000"),
			actual:    observationNumericTestValue("148,000"),
		},
		{
			name:      "oversized value",
			consensus: observationNumericTestValue(strings.Repeat("9", maxObservationNumericValueLength+1)),
			actual:    observationNumericTestValue("1"),
		},
		{
			name:      "incompatible units",
			consensus: observationNumericTestValue("3.1%"),
			actual:    observationNumericTestValue("3.3"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, gotDirection := observationNumericSurprise(test.consensus, test.actual)
			if got != nil || gotDirection != nil {
				t.Fatalf(
					"observationNumericSurprise(%v, %v) = (%v, %v), want (nil, nil)",
					test.consensus,
					test.actual,
					got,
					gotDirection,
				)
			}
		})
	}
}

func TestObservationNumericActualChangeCalculatesCompatibleValues(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		actual   string
		want     string
	}{
		{name: "positive percent", previous: "3.10%", actual: "3.3%", want: "+0.2%"},
		{name: "negative percent", previous: "3.3%", actual: "3.10%", want: "-0.2%"},
		{name: "zero percent", previous: "+0.00%", actual: "-0.0%", want: "0%"},
		{name: "positive unitless", previous: "100", actual: "125.50", want: "+25.5"},
		{name: "negative unitless", previous: "125.50", actual: "100", want: "-25.5"},
		{name: "zero unitless", previous: "+1.0", actual: "1.00", want: "0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := observationNumericActualChange(
				observationNumericTestValue(test.previous),
				observationNumericTestValue(test.actual),
			)
			if got == nil || *got != test.want {
				t.Fatalf(
					"observationNumericActualChange(%q, %q) = %v, want %q",
					test.previous,
					test.actual,
					got,
					test.want,
				)
			}
		})
	}
}

func TestObservationNumericActualChangeIsUnavailableForMissingUnsupportedOrIncompatibleValues(t *testing.T) {
	tests := []struct {
		name     string
		previous *string
		actual   *string
	}{
		{name: "missing previous", actual: observationNumericTestValue("3.2%")},
		{name: "missing actual", previous: observationNumericTestValue("3.0%")},
		{
			name:     "unsupported grouped values",
			previous: observationNumericTestValue("147,000"),
			actual:   observationNumericTestValue("148,000"),
		},
		{
			name:     "oversized value",
			previous: observationNumericTestValue(strings.Repeat("9", maxObservationNumericValueLength+1)),
			actual:   observationNumericTestValue("1"),
		},
		{
			name:     "incompatible units",
			previous: observationNumericTestValue("3.1%"),
			actual:   observationNumericTestValue("3.3"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := observationNumericActualChange(test.previous, test.actual); got != nil {
				t.Fatalf(
					"observationNumericActualChange(%v, %v) = %v, want nil",
					test.previous,
					test.actual,
					got,
				)
			}
		})
	}
}

func observationNumericTestValue(value string) *string {
	return &value
}
