package items

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

var imageLog = logger.Get("image")

// ImageExtractor extracts an image URL from a gofeed.Item using a 7-stage
// fallback strategy. It is stateless and safe for concurrent use.
type ImageExtractor struct {
	httpClient *http.Client
}

// NewImageExtractor creates an ImageExtractor with a sensible default HTTP client.
func NewImageExtractor() *ImageExtractor {
	return &ImageExtractor{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Extract attempts to find an image URL for item, trying stages 1-7 in order.
// Returns "" when no suitable image is found.
func (e *ImageExtractor) Extract(item *gofeed.Item) string {
	if item == nil {
		return ""
	}

	if u := e.extractFromMediaThumbnail(item); u != "" {
		imageLog.Printf("media:thumbnail: %s", u)
		return u
	}
	if u := e.extractFromMediaContent(item); u != "" {
		imageLog.Printf("media:content: %s", u)
		return u
	}
	if u := e.extractFromItemImage(item); u != "" {
		imageLog.Printf("item.Image: %s", u)
		return u
	}
	if u := e.extractFromEnclosures(item); u != "" {
		imageLog.Printf("enclosure: %s", u)
		return u
	}

	content := item.Content
	if content == "" {
		content = item.Description
	}
	if u := e.extractImgFromHTML(content); u != "" {
		imageLog.Printf("html img: %s", u)
		return u
	}

	if u := e.extractFromITunes(item); u != "" {
		imageLog.Printf("itunes:image: %s", u)
		return u
	}

	if item.Link != "" {
		if u := e.extractFromWebPage(item.Link); u != "" {
			imageLog.Printf("webpage: %s", u)
			return u
		}
	}

	imageLog.Printf("no image found for item: %s", item.Link)
	return ""
}

func (e *ImageExtractor) extractFromMediaThumbnail(item *gofeed.Item) string {
	if item.Extensions == nil {
		return ""
	}
	mediaNS, ok := item.Extensions["media"]
	if !ok {
		return ""
	}
	thumbnails, ok := mediaNS["thumbnail"]
	if !ok || len(thumbnails) == 0 {
		return ""
	}
	best := ""
	bestArea := 0
	for _, t := range thumbnails {
		u := t.Attrs["url"]
		if !e.isValidImageURL(u) {
			continue
		}
		tw, th := parseIntAttr(t.Attrs["width"]), parseIntAttr(t.Attrs["height"])
		area := tw * th
		if area > bestArea || best == "" {
			best = u
			bestArea = area
		}
	}
	return best
}

func (e *ImageExtractor) extractFromMediaContent(item *gofeed.Item) string {
	if item.Extensions == nil {
		return ""
	}
	mediaNS, ok := item.Extensions["media"]
	if !ok {
		return ""
	}
	contents, ok := mediaNS["content"]
	if !ok || len(contents) == 0 {
		return ""
	}
	best := ""
	bestArea := 0
	for _, c := range contents {
		medium := c.Attrs["medium"]
		typ := c.Attrs["type"]
		if medium != "image" && !strings.HasPrefix(typ, "image/") {
			continue
		}
		u := c.Attrs["url"]
		if !e.isValidImageURL(u) {
			continue
		}
		pw, ph := parseIntAttr(c.Attrs["width"]), parseIntAttr(c.Attrs["height"])
		area := pw * ph
		if area > bestArea || best == "" {
			best = u
			bestArea = area
		}
	}
	return best
}

func (e *ImageExtractor) extractFromItemImage(item *gofeed.Item) string {
	if item.Image != nil && e.isValidImageURL(item.Image.URL) {
		return item.Image.URL
	}
	return ""
}

func (e *ImageExtractor) extractFromEnclosures(item *gofeed.Item) string {
	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}
	return ""
}

func (e *ImageExtractor) extractFromITunes(item *gofeed.Item) string {
	content := item.Content
	if content == "" {
		content = item.Description
	}
	if content == "" {
		return ""
	}
	pat := regexp.MustCompile(`<itunes:image[^>]+href=["']([^"']+)["']`)
	if m := pat.FindStringSubmatch(content); len(m) > 1 {
		return m[1]
	}
	return ""
}

func (e *ImageExtractor) extractImgFromHTML(htmlContent string) string {
	if htmlContent == "" {
		return ""
	}
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	for {
		tok := tokenizer.Next()
		if tok == html.ErrorToken {
			break
		}
		if tok == html.StartTagToken || tok == html.SelfClosingTagToken {
			t := tokenizer.Token()
			if t.Data == "img" {
				for _, attr := range t.Attr {
					if attr.Key == "src" || attr.Key == "data-src" || attr.Key == "data-original" || attr.Key == "data-lazy-src" {
						if e.isValidImageURL(attr.Val) {
							return attr.Val
						}
					}
				}
			}
		}
	}
	return ""
}

func (e *ImageExtractor) isValidImageURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return false
	}
	path := rawURL
	if idx := strings.IndexByte(rawURL, '?'); idx != -1 {
		path = rawURL[:idx]
	}
	lower := strings.ToLower(path)
	return !strings.HasSuffix(lower, ".gif") && !strings.HasSuffix(lower, ".svg")
}

var (
	ogImagePattern   = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	ogImagePattern2  = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)
	twitterImagePat  = regexp.MustCompile(`(?i)<meta[^>]+name=["']twitter:image["'][^>]+content=["']([^"']+)["']`)
	twitterImagePat2 = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']twitter:image["']`)
	itemPropPattern  = regexp.MustCompile(`(?i)<meta[^>]+itemprop=["']image["'][^>]+content=["']([^"']+)["']`)
	itemPropPattern2 = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+itemprop=["']image["']`)
)

func (e *ImageExtractor) extractFromWebPage(pageURL string) string {
	if pageURL == "" {
		return ""
	}
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; RSS-Bot/1.0)")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50000))
	if err != nil {
		return ""
	}
	html := string(body)
	for _, pat := range []*regexp.Regexp{ogImagePattern, ogImagePattern2, twitterImagePat, twitterImagePat2, itemPropPattern, itemPropPattern2} {
		if m := pat.FindStringSubmatch(html); len(m) > 1 && e.isValidImageURL(m[1]) {
			return m[1]
		}
	}
	return e.extractImgFromHTML(html)
}

func parseIntAttr(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
