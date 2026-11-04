package build

import "testing"

func TestVersion(t *testing.T) {
	if got, want := Version(), "0.1.1"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}
