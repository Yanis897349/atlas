package bls

import (
	"errors"
	"fmt"
	"math/big"
)

type decimal struct {
	value *big.Rat
}

func parseDecimal(value string) (decimal, error) {
	if value == "" {
		return decimal{}, errors.New("decimal is required")
	}

	start := 0
	if value[0] == '+' || value[0] == '-' {
		start = 1
	}
	if start == len(value) {
		return decimal{}, errors.New("decimal requires digits")
	}

	dot := -1
	for index := start; index < len(value); index++ {
		switch {
		case value[index] == '.' && dot == -1:
			dot = index
		case value[index] < '0' || value[index] > '9':
			return decimal{}, errors.New("decimal contains invalid characters")
		}
	}
	if dot == start || dot == len(value)-1 {
		return decimal{}, errors.New("decimal point requires digits on both sides")
	}

	digits := value
	scale := 0
	if dot >= 0 {
		digits = value[:dot] + value[dot+1:]
		scale = len(value) - dot - 1
	}
	unscaled, valid := new(big.Int).SetString(digits, 10)
	if !valid {
		return decimal{}, errors.New("parse decimal digits")
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	return decimal{value: new(big.Rat).SetFrac(unscaled, denominator)}, nil
}

func (value decimal) isInteger() bool {
	return value.value.IsInt()
}

func calculateChanges(series Series, values []monthlyValue) (string, string, error) {
	switch series {
	case SeriesCPIAllItemsNSA:
		previous, err := formatPercentageChange(values[1], values[13])
		if err != nil {
			return "", "", err
		}
		actual, err := formatPercentageChange(values[0], values[12])
		if err != nil {
			return "", "", err
		}
		return previous, actual, nil
	case SeriesTotalNonfarmPayrollSA:
		previous := subtractIntegers(values[1].value, values[2].value)
		actual := subtractIntegers(values[0].value, values[1].value)
		return previous, actual, nil
	default:
		return "", "", fmt.Errorf("unsupported BLS series %q", series)
	}
}

func formatPercentageChange(current, earlier monthlyValue) (string, error) {
	if earlier.value.value.Sign() <= 0 {
		return "", fmt.Errorf(
			"period %q CPI comparison value must be positive",
			earlier.year+"-"+earlier.period,
		)
	}
	change := new(big.Rat).Sub(current.value.value, earlier.value.value)
	change.Quo(change, earlier.value.value)
	change.Mul(change, big.NewRat(100, 1))
	return formatRoundedTenths(change) + "%", nil
}

func formatRoundedTenths(value *big.Rat) string {
	scaled := new(big.Rat).Mul(value, big.NewRat(10, 1))
	numerator := new(big.Int).Abs(new(big.Int).Set(scaled.Num()))
	denominator := new(big.Int).Set(scaled.Denom())
	wholeTenths, remainder := new(big.Int), new(big.Int)
	wholeTenths.QuoRem(numerator, denominator, remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		wholeTenths.Add(wholeTenths, big.NewInt(1))
	}
	if wholeTenths.Sign() == 0 {
		return "0.0"
	}

	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(wholeTenths, big.NewInt(10), fraction)
	prefix := ""
	if scaled.Sign() < 0 {
		prefix = "-"
	}
	return fmt.Sprintf("%s%s.%s", prefix, whole.String(), fraction.String())
}

func subtractIntegers(left, right decimal) string {
	difference := new(big.Rat).Sub(left.value, right.value)
	if difference.Sign() > 0 {
		return "+" + difference.Num().String()
	}
	return difference.Num().String()
}
