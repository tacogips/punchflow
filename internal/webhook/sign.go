package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Sign returns the kickflow webhook signature for the exact request body bytes.
func Sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify compares a signature in constant time.
func Verify(secret, body []byte, header string) bool {
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), provided)
}
