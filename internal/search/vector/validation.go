// Package vector owns embedding-vector invariants shared across search boundaries.
package vector

import (
	"errors"
	"fmt"
	"math"
)

// Validate verifies that value has finite components and a pgvector-compatible cosine norm.
func Validate(value []float32) error {
	if len(value) == 0 {
		return errors.New("vector is required")
	}
	var squaredNorm float32
	for index, component := range value {
		if math.IsNaN(float64(component)) || math.IsInf(float64(component), 0) {
			return fmt.Errorf("vector value %d must be finite", index)
		}
		squaredNorm += component * component
	}
	if squaredNorm <= 0 || math.IsInf(float64(squaredNorm), 0) {
		return errors.New("vector must have finite non-zero cosine norm")
	}
	return nil
}
