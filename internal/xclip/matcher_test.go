package xclip

import "testing"

func TestParseArgsClipboardImageRead(t *testing.T) {
	t.Parallel()

	match := ParseArgs([]string{"-selection", "clipboard", "-t", "image/png", "-o"})
	if !match.IsClipboardRead() {
		t.Fatal("expected clipboard image read to match")
	}
	if !match.IsPNGRequest() {
		t.Fatal("expected png request")
	}
}

func TestParseArgsTargetsProbe(t *testing.T) {
	t.Parallel()

	match := ParseArgs([]string{"-sel", "clipboard", "-t", "TARGETS", "-o"})
	if !match.IsClipboardRead() || !match.IsTargetsProbe() {
		t.Fatal("expected TARGETS probe to match")
	}
}

func TestParseArgsNegativeCases(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"-selection", "primary", "-t", "image/png", "-o"},
		{"-selection", "clipboard", "-t", "text/plain", "-o"},
		{"-selection", "clipboard", "-t", "image/png"},
	}

	for _, args := range cases {
		if ParseArgs(args).IsClipboardRead() {
			t.Fatalf("ParseArgs(%v).IsClipboardRead() = true, want false", args)
		}
	}
}
