package webhook

import (
	"testing"
	"time"
)

func TestWebhookSignatureVector(t *testing.T) {
	body := []byte(`{"eventType":"ticket_created","x":"\u003ca\u0026b\u003e"}`)
	if got, want := Sign([]byte("secret"), body), "sha256=4f9dfd4b209c1c2ef393d754a09a60be16dd2df7a805a41ec5ebcc3115e3ae3a"; got != want {
		t.Fatalf("Sign() = %q, want %q", got, want)
	}
	sig := Sign([]byte("secret"), body)
	if !Verify([]byte("secret"), body, sig) {
		t.Fatal("Verify rejected valid signature")
	}
	if Verify([]byte("secret"), []byte(`{"changed":true}`), sig) {
		t.Fatal("Verify accepted tampered body")
	}
	if Verify([]byte("secret"), body, "sha256=not-hex") {
		t.Fatal("Verify accepted malformed signature")
	}
}

func TestWebhookBackoff(t *testing.T) {
	if got := Backoff(10*time.Second, 1); got != 10*time.Second {
		t.Fatalf("Backoff(1) = %s", got)
	}
	if got := Backoff(10*time.Second, 10); got != time.Hour {
		t.Fatalf("Backoff(10) = %s, want 1h", got)
	}
	if got := Backoff(10*time.Second, 0); got != 0 {
		t.Fatalf("Backoff(0) = %s, want 0", got)
	}
}
