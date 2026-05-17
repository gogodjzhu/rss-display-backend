package items_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	ext "github.com/mmcdole/gofeed/extensions"

	"github.com/esp32-rss-display/backend/server/domain/items"
	"github.com/mmcdole/gofeed"
)

func TestImageExtractorNilItem(t *testing.T) {
	e := items.NewImageExtractor()
	if u := e.Extract(nil); u != "" {
		t.Fatalf("expected empty, got %q", u)
	}
}

func TestImageExtractorMediaThumbnail(t *testing.T) {
	e := items.NewImageExtractor()
	item := &gofeed.Item{
		Extensions: ext.Extensions{
			"media": {
				"thumbnail": {
					{Attrs: map[string]string{"url": "https://example.com/thumb.jpg", "width": "320", "height": "240"}},
				},
			},
		},
	}
	u := e.Extract(item)
	if u != "https://example.com/thumb.jpg" {
		t.Fatalf("expected thumbnail url, got %q", u)
	}
}

func TestImageExtractorEnclosure(t *testing.T) {
	e := items.NewImageExtractor()
	item := &gofeed.Item{
		Enclosures: []*gofeed.Enclosure{
			{URL: "https://example.com/photo.jpg", Type: "image/jpeg"},
		},
	}
	u := e.Extract(item)
	if u != "https://example.com/photo.jpg" {
		t.Fatalf("expected enclosure url, got %q", u)
	}
}

func TestImageExtractorHTMLContent(t *testing.T) {
	e := items.NewImageExtractor()
	item := &gofeed.Item{
		Description: `<p>Hello <img src="https://example.com/img.png" /></p>`,
	}
	u := e.Extract(item)
	if u != "https://example.com/img.png" {
		t.Fatalf("expected html img url, got %q", u)
	}
}

func TestImageExtractorRejectsGIF(t *testing.T) {
	e := items.NewImageExtractor()
	item := &gofeed.Item{
		Image: &gofeed.Image{URL: "https://example.com/anim.gif"},
	}
	// gif should be rejected; falls through to other stages which also return ""
	// so the final result should be ""
	u := e.Extract(item)
	if u != "" {
		t.Fatalf("expected gif to be rejected, got %q", u)
	}
}

func TestImageExtractorWebPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><head><meta property="og:image" content="https://cdn.example.com/og.jpg"/></head></html>`))
	}))
	defer srv.Close()

	e := items.NewImageExtractor()
	item := &gofeed.Item{Link: srv.URL}
	u := e.Extract(item)
	if u != "https://cdn.example.com/og.jpg" {
		t.Fatalf("expected og:image from webpage, got %q", u)
	}
}
