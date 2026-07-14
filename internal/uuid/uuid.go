// Package uuid validates and normalizes UUID text independently of storage adapters.
package uuid

import "encoding/hex"

// Normalize returns the canonical lowercase representation of a valid UUID.
// Canonical hyphenated and compact hexadecimal forms are accepted.
func Normalize(value string) (string, bool) {
	var compact [32]byte
	switch len(value) {
	case len(compact):
		copy(compact[:], value)
	case 36:
		if value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
			return "", false
		}
		copy(compact[0:8], value[0:8])
		copy(compact[8:12], value[9:13])
		copy(compact[12:16], value[14:18])
		copy(compact[16:20], value[19:23])
		copy(compact[20:32], value[24:36])
	default:
		return "", false
	}

	var decoded [16]byte
	if _, err := hex.Decode(decoded[:], compact[:]); err != nil {
		return "", false
	}

	var normalized [36]byte
	hex.Encode(normalized[0:8], decoded[0:4])
	normalized[8] = '-'
	hex.Encode(normalized[9:13], decoded[4:6])
	normalized[13] = '-'
	hex.Encode(normalized[14:18], decoded[6:8])
	normalized[18] = '-'
	hex.Encode(normalized[19:23], decoded[8:10])
	normalized[23] = '-'
	hex.Encode(normalized[24:36], decoded[10:16])
	return string(normalized[:]), true
}
