package rssworker

import "testing"

func TestNormalizeItemURLRemovesFragment(t *testing.T) {
	t.Parallel()

	raw := "https://www.v2ex.com/t/1210291#reply8"
	got := normalizeItemURL(raw)
	want := "https://www.v2ex.com/t/1210291"

	if got != want {
		t.Fatalf("normalizeItemURL(%q) = %q, want %q", raw, got, want)
	}
}

func TestNormalizeItemURLPreservesQuery(t *testing.T) {
	t.Parallel()

	raw := "https://example.com/post?id=1&utm_source=rss#section"
	got := normalizeItemURL(raw)
	want := "https://example.com/post?id=1&utm_source=rss"

	if got != want {
		t.Fatalf("normalizeItemURL(%q) = %q, want %q", raw, got, want)
	}
}

func TestNormalizeItemURLKeepsInvalidURL(t *testing.T) {
	t.Parallel()

	raw := "://bad url"
	if got := normalizeItemURL(raw); got != raw {
		t.Fatalf("normalizeItemURL(%q) = %q, want original value", raw, got)
	}
}
