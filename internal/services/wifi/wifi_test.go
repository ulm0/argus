package wifi

import "testing"

func TestIsValidUUID(t *testing.T) {
	valid := []string{
		"12345678-90ab-cdef-1234-567890abcdef",
		"DEADBEEF-0000-0000-0000-000000000000",
	}
	for _, s := range valid {
		if !isValidUUID(s) {
			t.Errorf("isValidUUID(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"",
		"short",
		"-12345678",                            // leading dash (flag injection)
		"12345678; rm -rf /",                   // shell metachars
		"12345678 90ab cdef",                   // spaces
		"g2345678-90ab-cdef-1234-567890abcdef", // non-hex
	}
	for _, s := range invalid {
		if isValidUUID(s) {
			t.Errorf("isValidUUID(%q) = true, want false", s)
		}
	}
}

func TestUnescapeNmcli(t *testing.T) {
	cases := map[string]string{
		`My\:Net`:     "My:Net",
		`plain`:       "plain",
		`back\\slash`: `back\slash`,
	}
	for in, want := range cases {
		if got := unescapeNmcli(in); got != want {
			t.Errorf("unescapeNmcli(%q) = %q, want %q", in, got, want)
		}
	}
}
