package bls

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type monthlyValue struct {
	year    string
	period  string
	ordinal int
	value   decimal
}

func normalizeSeries(series apiSeries) (snapshot, error) {
	seriesID := strings.TrimSpace(series.SeriesID)
	if seriesID == "" {
		return snapshot{}, errors.New("series ID is required")
	}

	values := make([]monthlyValue, 0, len(series.Data))
	seen := make(map[string]string, len(series.Data))
	for index, data := range series.Data {
		value, err := normalizeData(data, Series(seriesID))
		if err != nil {
			return snapshot{}, fmt.Errorf("data point %d: %w", index+1, err)
		}
		identity := data.Year + "-" + data.Period
		if value, exists := seen[identity]; exists {
			if value != data.Value {
				return snapshot{}, fmt.Errorf("period %q has conflicting values", identity)
			}
			continue
		}
		seen[identity] = data.Value
		values = append(values, value)
	}
	if len(values) == 0 {
		return snapshot{}, errors.New("at least one monthly data point is required")
	}
	sort.Slice(values, func(left, right int) bool {
		return values[left].ordinal > values[right].ordinal
	})

	required := 3
	if Series(seriesID) == SeriesCPIAllItemsNSA {
		required = 14
	}
	if len(values) < required {
		return snapshot{}, fmt.Errorf(
			"requires at least %d monthly data points, got %d",
			required,
			len(values),
		)
	}
	for index := 1; index < required; index++ {
		if values[index-1].ordinal-values[index].ordinal != 1 {
			return snapshot{}, fmt.Errorf(
				"monthly history from %s-%s must be consecutive before %s-%s",
				values[0].year,
				values[0].period,
				values[index-1].year,
				values[index-1].period,
			)
		}
	}

	previous, actual, err := calculateChanges(Series(seriesID), values)
	if err != nil {
		return snapshot{}, err
	}

	return snapshot{
		seriesID: seriesID,
		year:     values[0].year,
		period:   values[0].period,
		actual:   actual,
		previous: previous,
	}, nil
}

func normalizeData(data apiData, series Series) (monthlyValue, error) {
	if len(data.Year) != 4 {
		return monthlyValue{}, errors.New("year must contain four digits")
	}
	year, err := strconv.Atoi(data.Year)
	if err != nil || year < 1000 {
		return monthlyValue{}, errors.New("year must contain four digits")
	}
	if len(data.Period) != 3 || data.Period[0] != 'M' {
		return monthlyValue{}, errors.New("period must be between M01 and M12")
	}
	month, err := strconv.Atoi(data.Period[1:])
	if err != nil || month < 1 || month > 12 {
		return monthlyValue{}, errors.New("period must be between M01 and M12")
	}
	if strings.TrimSpace(data.Value) == "" {
		return monthlyValue{}, errors.New("value must not be blank")
	}
	value, err := parseDecimal(data.Value)
	if err != nil {
		return monthlyValue{}, fmt.Errorf("period %q value must be a decimal", data.Year+"-"+data.Period)
	}
	if series == SeriesTotalNonfarmPayrollSA && !value.isInteger() {
		return monthlyValue{}, fmt.Errorf("period %q payroll value must be an integer", data.Year+"-"+data.Period)
	}
	return monthlyValue{
		year:    data.Year,
		period:  data.Period,
		ordinal: year*12 + month - 1,
		value:   value,
	}, nil
}
