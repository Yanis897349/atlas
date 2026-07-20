package intelligence

import (
	"math/big"
	"strings"
)

// maxObservationNumericValueLength bounds exact-decimal work for provider-supplied values.
const maxObservationNumericValueLength = 128

type observationNumericUnit string

// SurpriseDirection classifies an exact actual-minus-consensus difference.
type SurpriseDirection string

const (
	observationNumericUnitNone    observationNumericUnit = ""
	observationNumericUnitPercent observationNumericUnit = "%"

	SurpriseDirectionAboveConsensus SurpriseDirection = "above_consensus"
	SurpriseDirectionBelowConsensus SurpriseDirection = "below_consensus"
	SurpriseDirectionInLine         SurpriseDirection = "in_line"
)

type observationNumericValue struct {
	value *big.Rat
	scale int
	unit  observationNumericUnit
}

func parseObservationNumericValue(raw string) (observationNumericValue, bool) {
	if len(raw) > maxObservationNumericValueLength {
		return observationNumericValue{}, false
	}

	unit := observationNumericUnitNone
	value := raw
	if strings.HasSuffix(value, string(observationNumericUnitPercent)) {
		unit = observationNumericUnitPercent
		value = strings.TrimSuffix(value, string(unit))
	}
	if value == "" {
		return observationNumericValue{}, false
	}

	start := 0
	if value[0] == '+' || value[0] == '-' {
		start = 1
	}
	if start == len(value) {
		return observationNumericValue{}, false
	}

	dot := -1
	for index := start; index < len(value); index++ {
		switch {
		case value[index] == '.' && dot == -1:
			dot = index
		case value[index] < '0' || value[index] > '9':
			return observationNumericValue{}, false
		}
	}
	if dot == start || dot == len(value)-1 {
		return observationNumericValue{}, false
	}

	digits := value
	scale := 0
	if dot >= 0 {
		digits = value[:dot] + value[dot+1:]
		scale = len(value) - dot - 1
	}
	unscaled, valid := new(big.Int).SetString(digits, 10)
	if !valid {
		return observationNumericValue{}, false
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	return observationNumericValue{
		value: new(big.Rat).SetFrac(unscaled, denominator),
		scale: scale,
		unit:  unit,
	}, true
}

func observationNumericDelta(oldRaw, newRaw string) (*string, bool) {
	delta, _, valid := observationNumericDifference(oldRaw, newRaw)
	return delta, valid
}

func observationNumericDifference(oldRaw, newRaw string) (*string, int, bool) {
	oldValue, valid := parseObservationNumericValue(oldRaw)
	if !valid {
		return nil, 0, false
	}
	newValue, valid := parseObservationNumericValue(newRaw)
	if !valid || oldValue.unit != newValue.unit {
		return nil, 0, false
	}

	difference := new(big.Rat).Sub(newValue.value, oldValue.value)
	formatted := difference.FloatString(max(oldValue.scale, newValue.scale))
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	if difference.Sign() > 0 {
		formatted = "+" + formatted
	}
	formatted += string(oldValue.unit)
	return &formatted, difference.Sign(), true
}

func observationNumericSurprise(consensus, actual *string) (*string, *SurpriseDirection) {
	if consensus == nil || actual == nil {
		return nil, nil
	}

	surprise, sign, valid := observationNumericDifference(*consensus, *actual)
	if !valid {
		return nil, nil
	}

	direction := SurpriseDirectionInLine
	if sign > 0 {
		direction = SurpriseDirectionAboveConsensus
	} else if sign < 0 {
		direction = SurpriseDirectionBelowConsensus
	}
	return surprise, &direction
}

func observationNumericActualChange(previous, actual *string) *string {
	if previous == nil || actual == nil {
		return nil
	}

	change, valid := observationNumericDelta(*previous, *actual)
	if !valid {
		return nil
	}
	return change
}
