package version

import "testing"

func TestCurrentReturnsStampedVersion(t *testing.T) {
	original := stampedVersion
	t.Cleanup(func() { stampedVersion = original })

	stampedVersion = "v1.2.3"

	if got := Current(); got != "v1.2.3" {
		t.Fatalf("Current() = %q, want %q", got, "v1.2.3")
	}
}

func TestCurrentNeverReturnsEmptyVersion(t *testing.T) {
	original := stampedVersion
	t.Cleanup(func() { stampedVersion = original })

	stampedVersion = ""

	if got := Current(); got == "" {
		t.Fatal("Current() returned an empty version")
	}
}
