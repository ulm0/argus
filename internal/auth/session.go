// Package auth provides stateless, HMAC-signed session tokens for the web UI.
// Tokens are self-contained (username + expiry, signed with the config
// SecretKey), so no server-side session store is needed on the constrained Pi.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// SessionTTL is how long an issued session stays valid.
const SessionTTL = 7 * 24 * time.Hour

// SignSession returns a signed token encoding the username and an expiry
// SessionTTL into the future.
func SignSession(secret, username string, now time.Time) string {
	payload := username + "|" + strconv.FormatInt(now.Add(SessionTTL).Unix(), 10)
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return enc + "." + sign(secret, enc)
}

// VerifySession validates a token's signature and expiry and returns the
// username it was issued for. ok is false for any malformed, tampered, or
// expired token.
func VerifySession(secret, token string, now time.Time) (username string, ok bool) {
	enc, sig, found := strings.Cut(token, ".")
	if !found {
		return "", false
	}
	// Constant-time signature check.
	if !hmac.Equal([]byte(sig), []byte(sign(secret, enc))) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", false
	}
	// Split on the LAST separator: the expiry never contains '|', but the
	// username legitimately can, so cutting on the first '|' would misparse it.
	payload := string(raw)
	i := strings.LastIndex(payload, "|")
	if i < 0 {
		return "", false
	}
	name, expStr := payload[:i], payload[i+1:]
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || now.Unix() >= exp {
		return "", false
	}
	return name, true
}

func sign(secret, msg string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
