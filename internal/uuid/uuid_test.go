package uuid

import "testing"

func TestNormalize(t *testing.T) {
	const want = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	tests := []struct {
		name  string
		value string
		want  string
		valid bool
	}{
		{name: "canonical", value: want, want: want, valid: true},
		{name: "uppercase", value: "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", want: want, valid: true},
		{name: "compact", value: "aaaaaaaabbbbccccddddeeeeeeeeeeee", want: want, valid: true},
		{name: "empty", value: ""},
		{name: "wrong length", value: "not-a-uuid"},
		{name: "invalid hexadecimal", value: "gaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
		{name: "non-hyphen separators", value: "aaaaaaaaXbbbbXccccXddddXeeeeeeeeeeee"},
		{name: "misplaced hyphens", value: "aaaaaaa-abbbb-cccc-dddd-eeeeeeeeeeee"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := Normalize(test.value)
			if got != test.want || valid != test.valid {
				t.Errorf("Normalize(%q) = (%q, %t), want (%q, %t)", test.value, got, valid, test.want, test.valid)
			}
		})
	}
}
