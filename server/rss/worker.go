package rssworker

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/metrics"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
	"gorm.io/gorm"
)

type Worker struct {
	fetchInterval time.Duration
	httpClient    *http.Client
	stopCh        chan struct{}
	refreshing    atomic.Bool
}

func New(cfg *config.RSSConfig) *Worker {
	feedTimeout := time.Duration(cfg.FeedFetchTimeoutSeconds) * time.Second
	if feedTimeout <= 0 {
		feedTimeout = 10 * time.Second
	}

	return &Worker{
		fetchInterval: time.Duration(cfg.FetchIntervalMinutes) * time.Minute,
		httpClient:    &http.Client{Timeout: feedTimeout},
		stopCh:        make(chan struct{}),
	}
}

func (w *Worker) Start() {
	log.Printf("[rss] worker started: interval=%s timeout=%s", w.fetchInterval, w.httpClient.Timeout)
	go w.fetchAllFeeds()
	go w.loop()
}

func (w *Worker) Stop() {
	close(w.stopCh)
}

func (w *Worker) loop() {
	ticker := time.NewTicker(w.fetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.fetchAllFeeds()
		case <-w.stopCh:
			return
		}
	}
}

func (w *Worker) fetchAllFeeds() {
	if !w.refreshing.CompareAndSwap(false, true) {
		log.Printf("[rss] refresh skipped: previous refresh still running")
		return
	}
	defer w.refreshing.Store(false)

	db := database.GetDB()
	var feeds []models.Feed

	if err := db.Where("enabled = ?", true).Find(&feeds).Error; err != nil {
		log.Printf("[rss] failed to load enabled feeds: %v", err)
		metrics.RSSFetchError.Add(1)
		return
	}

	startedAt := time.Now()
	log.Printf("[rss] refreshing %d enabled feeds", len(feeds))

	for _, feed := range feeds {
		w.fetchFeed(feed)
	}

	log.Printf("[rss] refresh finished in %s", time.Since(startedAt).Round(time.Millisecond))
}

func (w *Worker) fetchFeed(feed models.Feed) {
	metrics.RSSFetchTotal.Add(1)
	startedAt := time.Now()
	log.Printf("[rss] fetching feed %q", feed.Name)

	parser := gofeed.NewParser()
	parser.Client = w.httpClient
	parsed, err := parser.ParseURL(feed.URL)
	if err != nil {
		log.Printf("[rss] fetch failed for %q: %v", feed.Name, err)
		metrics.RSSFetchError.Add(1)
		return
	}

	db := database.GetDB()
	newItems := 0

	for _, item := range parsed.Items {
		title := item.Title
		itemURL := normalizeItemURL(item.Link)

		if itemURL == "" {
			continue
		}

		var existingItem models.Item
		if err := db.Where("feed_id = ? AND url = ?", feed.ID, itemURL).First(&existingItem).Error; err == nil {
			continue
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[rss] failed to check existing item %q: %v", title, err)
			continue
		}

		var publishedAt *time.Time
		if item.PublishedParsed != nil {
			t := *item.PublishedParsed
			publishedAt = &t
		}

		imageURL := w.extractImage(item)

		newItem := models.Item{
			FeedID:      feed.ID,
			Title:       title,
			URL:         itemURL,
			ImageURL:    imageURL,
			PublishedAt: publishedAt,
		}

		if err := db.Create(&newItem).Error; err != nil {
			if isDuplicateItemError(err) {
				continue
			}
			log.Printf("[rss] failed to save item %q: %v", title, err)
			continue
		}

		metrics.RSSItemsParsedTotal.Add(1)
		newItems++
	}

	log.Printf("[rss] feed %q refreshed: %d items fetched, %d new, took %s", feed.Name, len(parsed.Items), newItems, time.Since(startedAt).Round(time.Millisecond))
}

func normalizeItemURL(raw string) string {
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	parsed.Fragment = ""
	return parsed.String()
}

func isDuplicateItemError(err error) bool {
	if err == nil {
		return false
	}

	errText := strings.ToLower(err.Error())
	return strings.Contains(errText, "unique constraint") || strings.Contains(errText, "duplicate entry")
}

func (w *Worker) extractImage(item *gofeed.Item) string {
	if item == nil {
		return ""
	}

	// Stage 1: <media:thumbnail> — used by BBC, Yahoo, and many major news feeds
	if imgURL := w.extractFromMediaThumbnail(item); imgURL != "" {
		log.Printf("[image] media:thumbnail: %s", imgURL)
		return imgURL
	}

	// Stage 2: <media:content> with medium="image" — another Media RSS variant
	if imgURL := w.extractFromMediaContent(item); imgURL != "" {
		log.Printf("[image] media:content: %s", imgURL)
		return imgURL
	}

	// Stage 3: gofeed native item.Image field
	if imgURL := w.extractFromItemImage(item); imgURL != "" {
		log.Printf("[image] item.Image: %s", imgURL)
		return imgURL
	}

	// Stage 4: RSS <enclosure type="image/...">
	if imgURL := w.extractFromEnclosures(item); imgURL != "" {
		log.Printf("[image] enclosure: %s", imgURL)
		return imgURL
	}

	// Stage 5: <img src> inside item HTML content / description
	content := item.Content
	if content == "" {
		content = item.Description
	}
	if imgURL := w.extractImgFromHTML(content); imgURL != "" {
		log.Printf("[image] html img: %s", imgURL)
		return imgURL
	}

	// Stage 6: iTunes-style <itunes:image href="...">
	if imgURL := w.extractFromITunes(item); imgURL != "" {
		log.Printf("[image] itunes:image: %s", imgURL)
		return imgURL
	}

	// Stage 7: scrape the linked article page for og:image / twitter:image
	if item.Link != "" {
		if imgURL := w.extractFromWebPage(item.Link); imgURL != "" {
			log.Printf("[image] webpage: %s", imgURL)
			return imgURL
		}
	}

	log.Printf("[image] no image found for item: %s", item.Link)
	return ""
}

// extractFromMediaThumbnail handles <media:thumbnail url="..."> used by BBC,
// Yahoo Media RSS, Reuters, and many other major news publishers.
func (w *Worker) extractFromMediaThumbnail(item *gofeed.Item) string {
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
	// Pick the largest thumbnail if multiple are present
	best := ""
	bestArea := 0
	for _, t := range thumbnails {
		u := t.Attrs["url"]
		if !w.isValidImageURL(u) {
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

// extractFromMediaContent handles <media:content url="..." medium="image">.
func (w *Worker) extractFromMediaContent(item *gofeed.Item) string {
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
		// Accept if medium="image" or type starts with "image/"
		if medium != "image" && !strings.HasPrefix(typ, "image/") {
			continue
		}
		u := c.Attrs["url"]
		if !w.isValidImageURL(u) {
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

// extractFromItemImage uses gofeed's native item.Image field, which some
// parsers populate from <image> child elements inside an <item>.
func (w *Worker) extractFromItemImage(item *gofeed.Item) string {
	if item.Image != nil && w.isValidImageURL(item.Image.URL) {
		return item.Image.URL
	}
	return ""
}

// parseIntAttr converts a string attribute value to int, returning 0 on error.
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

func (w *Worker) extractFromEnclosures(item *gofeed.Item) string {
	if len(item.Enclosures) == 0 {
		return ""
	}

	for _, enc := range item.Enclosures {
		if strings.HasPrefix(enc.Type, "image/") && enc.URL != "" {
			return enc.URL
		}
	}

	return ""
}

func (w *Worker) extractFromITunes(item *gofeed.Item) string {
	content := item.Content
	if content == "" {
		content = item.Description
	}

	if content == "" {
		return ""
	}

	itunesImgPattern := regexp.MustCompile(`<itunes:image[^>]+href=["']([^"']+)["']`)
	if matches := itunesImgPattern.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func (w *Worker) extractImgFromHTML(htmlContent string) string {
	if htmlContent == "" {
		return ""
	}

	reader := strings.NewReader(htmlContent)
	tokenizer := html.NewTokenizer(reader)

	for {
		token := tokenizer.Next()
		if token == html.ErrorToken {
			break
		}

		if token == html.StartTagToken || token == html.SelfClosingTagToken {
			t := tokenizer.Token()
			if t.Data == "img" {
				for _, attr := range t.Attr {
					if attr.Key == "src" {
						url := attr.Val
						if w.isValidImageURL(url) {
							return url
						}
					}
					if attr.Key == "data-src" || attr.Key == "data-original" || attr.Key == "data-lazy-src" {
						url := attr.Val
						if w.isValidImageURL(url) {
							return url
						}
					}
				}
			}
		}
	}

	return ""
}

func (w *Worker) isValidImageURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return false
	}
	// Check only the path component (before any '?') to avoid false positives
	// on query parameters like "?format=webp".
	path := rawURL
	if idx := strings.IndexByte(rawURL, '?'); idx != -1 {
		path = rawURL[:idx]
	}
	lower := strings.ToLower(path)
	// Reject formats that Go's image decoder cannot handle, or that are
	// generally unsuitable for display (animated gif, vector svg).
	if strings.HasSuffix(lower, ".gif") || strings.HasSuffix(lower, ".svg") {
		return false
	}
	return true
}

var (
	ogImagePattern   = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	ogImagePattern2  = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)
	twitterImagePat  = regexp.MustCompile(`(?i)<meta[^>]+name=["']twitter:image["'][^>]+content=["']([^"']+)["']`)
	twitterImagePat2 = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']twitter:image["']`)
	itemPropPattern  = regexp.MustCompile(`(?i)<meta[^>]+itemprop=["']image["'][^>]+content=["']([^"']+)["']`)
	itemPropPattern2 = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+itemprop=["']image["']`)
)

func (w *Worker) extractFromWebPage(url string) string {
	if url == "" {
		return ""
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; RSS-Bot/1.0)")

	resp, err := http.DefaultClient.Do(req)
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

	htmlContent := string(body)

	patterns := []*regexp.Regexp{ogImagePattern, ogImagePattern2, twitterImagePat, twitterImagePat2, itemPropPattern, itemPropPattern2}
	for _, pattern := range patterns {
		if matches := pattern.FindStringSubmatch(htmlContent); len(matches) > 1 {
			url := matches[1]
			if w.isValidImageURL(url) {
				return url
			}
		}
	}

	if imgURL := w.extractImgFromHTML(htmlContent); imgURL != "" {
		return imgURL
	}

	return ""
}
