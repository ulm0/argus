package bluetooth

import "testing"

func TestIsValidMAC(t *testing.T) {
	valid := []string{
		"AA:BB:CC:DD:EE:FF",
		"00:11:22:33:44:55",
		"a0:b1:c2:d3:e4:f5",
	}
	for _, s := range valid {
		if !isValidMAC(s) {
			t.Errorf("isValidMAC(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"",
		"AA:BB:CC:DD:EE",            // too short
		"AA:BB:CC:DD:EE:FF:00",      // too long
		"ZZ:BB:CC:DD:EE:FF",         // non-hex
		"AA-BB-CC-DD-EE-FF",         // wrong separator
		"-A:BB:CC:DD:EE:FF; reboot", // injection attempt
		"AABBCCDDEEFF",              // no separators
	}
	for _, s := range invalid {
		if isValidMAC(s) {
			t.Errorf("isValidMAC(%q) = true, want false", s)
		}
	}
}
