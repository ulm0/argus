package auth

import (
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := SignSession("secret-key", "admin", now)

	user, ok := VerifySession("secret-key", tok, now)
	if !ok || user != "admin" {
		t.Fatalf("VerifySession = (%q, %v), want (admin, true)", user, ok)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := SignSession("secret-key", "admin", now)

	cases := map[string]string{
		"wrong secret":  "different-key",
		"correct again": "secret-key",
	}
	if _, ok := VerifySession(cases["wrong secret"], tok, now); ok {
		t.Error("accepted token signed with a different secret")
	}
	// Flip a byte in the payload.
	bad := "x" + tok[1:]
	if _, ok := VerifySession("secret-key", bad, now); ok {
		t.Error("accepted tampered token")
	}
	// Sanity: untampered still verifies.
	if _, ok := VerifySession(cases["correct again"], tok, now); !ok {
		t.Error("rejected a valid token")
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	issued := time.Unix(1_700_000_000, 0)
	tok := SignSession("secret-key", "admin", issued)

	later := issued.Add(SessionTTL + time.Second)
	if _, ok := VerifySession("secret-key", tok, later); ok {
		t.Error("accepted an expired token")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, tok := range []string{"", "nodot", "a.b.c", "...."} {
		if _, ok := VerifySession("secret-key", tok, now); ok {
			t.Errorf("accepted malformed token %q", tok)
		}
	}
}
