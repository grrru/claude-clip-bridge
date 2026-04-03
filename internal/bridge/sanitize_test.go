package bridge

import "testing"

func TestSanitizeSocketComponent(t *testing.T) {
	t.Parallel()

	got := SanitizeSocketComponent("host name/with:chars")
	want := "host-name-with-chars"
	if got != want {
		t.Fatalf("SanitizeSocketComponent() = %q, want %q", got, want)
	}
}
