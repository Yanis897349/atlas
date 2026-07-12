package vector

import (
	"math"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		vector   []float32
		contains string
	}{
		{name: "valid", vector: []float32{0.1, -0.2}},
		{name: "missing", contains: "vector is required"},
		{name: "zero norm", vector: []float32{0, 0}, contains: "finite non-zero cosine norm"},
		{name: "underflowing norm", vector: []float32{1e-30}, contains: "finite non-zero cosine norm"},
		{name: "overflowing norm", vector: []float32{1e30}, contains: "finite non-zero cosine norm"},
		{name: "NaN", vector: []float32{1, float32(math.NaN())}, contains: "value 1 must be finite"},
		{name: "positive infinity", vector: []float32{float32(math.Inf(1))}, contains: "value 0 must be finite"},
		{name: "negative infinity", vector: []float32{float32(math.Inf(-1))}, contains: "value 0 must be finite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.vector)
			if test.contains == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if test.contains != "" && (err == nil || !strings.Contains(err.Error(), test.contains)) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.contains)
			}
		})
	}
}
