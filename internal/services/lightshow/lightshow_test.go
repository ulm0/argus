package lightshow

import (
	"encoding/binary"
	"testing"
)

// buildFseq creates a minimal fseq header with the fields the validator reads.
func buildFseq(major, compression, stepMS byte, frames uint32) []byte {
	b := make([]byte, 32)
	copy(b[0:4], "PSEQ")
	b[7] = major
	binary.LittleEndian.PutUint32(b[14:18], frames)
	b[18] = stepMS
	b[20] = compression
	return b
}

func TestValidateFseq(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		ok   bool
	}{
		{"valid v2", buildFseq(2, 0, 50, 1200), true},
		{"v1", buildFseq(1, 0, 50, 1200), false},
		{"compressed", buildFseq(2, 1, 50, 1200), false},
		{"frame interval too short", buildFseq(2, 0, 10, 1200), false},
		{"frame interval too long", buildFseq(2, 0, 120, 1200), false},
		{"longer than 4h", buildFseq(2, 0, 50, 300000), false},
		{"not an fseq", []byte("not a light show at all!!"), false},
		{"truncated", []byte("PSEQ"), false},
	}
	for _, c := range cases {
		err := validateFseq(c.data)
		if (err == nil) != c.ok {
			t.Errorf("validateFseq(%s) err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// buildShowWAV creates a WAV header at the given sample rate, optionally behind
// a metadata chunk so the chunk walk is exercised.
func buildShowWAV(rate uint32, junk int) []byte {
	b := append([]byte("RIFF"), 0, 0, 0, 0)
	b = append(b, "WAVE"...)
	if junk > 0 {
		b = append(b, "JUNK"...)
		b = binary.LittleEndian.AppendUint32(b, uint32(junk))
		b = append(b, make([]byte, junk)...)
	}
	b = append(b, "fmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, 2)
	b = binary.LittleEndian.AppendUint32(b, rate)
	return append(b, make([]byte, 8)...)
}

func TestValidateShowWAV(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		ok   bool
	}{
		{"44.1 kHz", buildShowWAV(44100, 0), true},
		{"44.1 kHz behind metadata chunk", buildShowWAV(44100, 32), true},
		{"48 kHz", buildShowWAV(48000, 0), false},
		{"48 kHz behind metadata chunk", buildShowWAV(48000, 32), false},
		{"not a WAV", []byte("PSEQ nope"), false},
	}
	for _, c := range cases {
		err := validateShowWAV(c.data)
		if (err == nil) != c.ok {
			t.Errorf("validateShowWAV(%s) err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// MP3 is deliberately not probed, and unknown extensions never reach here.
func TestValidateShowHeaderSkipsMP3(t *testing.T) {
	if err := validateShowHeader(".mp3", []byte("ID3 whatever")); err != nil {
		t.Errorf("validateShowHeader(.mp3) = %v, want nil", err)
	}
}
