package rssworker

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/domain/feeds"
	"github.com/esp32-rss-display/backend/server/domain/items"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/metrics"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/mmcdole/gofeed"
)

var rssLog = logger.Get("rss")

type Worker struct {
	fetchInterval time.Duration
	httpClient    *http.Client
	extractor     *items.ImageExtractor
	feedSvc      feeds.Service
	itemSvc      items.Service
	stopCh       chan struct{}
	refreshing    atomic.Bool
}

func New(cfg *config.RSSConfig, feedSvc feeds.Service, itemSvc items.Service) *Worker {
	feedTimeout := time.Duration(cfg.FeedFetchTimeoutSeconds) * time.Second
	if feedTimeout <= 0 {
		feedTimeout = 10 * time.Second
	}

	return &Worker{
		fetchInterval: time.Duration(cfg.FetchIntervalMinutes) * time.Minute,
		httpClient:    &http.Client{Timeout: feedTimeout},
		extractor:     items.NewImageExtractor(),
		feedSvc:      feedSvc,
		itemSvc:      itemSvc,
		stopCh:       make(chan struct{}),
	}
}

func (w *Worker) Start() {
	rssLog.Printf("worker started: interval=%s timeout=%s", w.fetchInterval, w.httpClient.Timeout)
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
		rssLog.Printf("refresh skipped: previous refresh still running")
		return
	}
	defer w.refreshing.Store(false)

	ctx := context.Background()
	feedList, err := w.feedSvc.ListEnabled(ctx)
	if err != nil {
		rssLog.Printf("failed to load enabled feeds: %v", err)
		metrics.RSSFetchError.Add(1)
		return
	}

	startedAt := time.Now()
	rssLog.Printf("refreshing %d enabled feeds", len(feedList))

	for _, feed := range feedList {
		w.fetchFeed(feed)
	}

	rssLog.Printf("refresh finished in %s", time.Since(startedAt).Round(time.Millisecond))
}

func (w *Worker) fetchFeed(feed models.Feed) {
	metrics.RSSFetchTotal.Add(1)
	startedAt := time.Now()
	rssLog.Printf("fetching feed %q", feed.Name)

	parser := gofeed.NewParser()
	parser.Client = w.httpClient
	parsed, err := parser.ParseURL(feed.URL)
	if err != nil {
		rssLog.Printf("fetch failed for %q: %v", feed.Name, err)
		metrics.RSSFetchError.Add(1)
		return
	}

	ctx := context.Background()
	newItems := 0

	for _, item := range parsed.Items {
		title := item.Title
		itemURL := normalizeItemURL(item.Link)

		if itemURL == "" {
			continue
		}

		var publishedAt *time.Time
		if item.PublishedParsed != nil {
			t := *item.PublishedParsed
			publishedAt = &t
		}

		imageURL := w.extractor.Extract(item)

		newItem := &models.Item{
			FeedID:      feed.ID,
			Title:       title,
			URL:         itemURL,
			ImageURL:    imageURL,
			PublishedAt: publishedAt,
		}

		created, err := w.itemSvc.CreateIfNew(ctx, newItem)
		if err != nil {
			if !isDuplicateItemError(err) {
				rssLog.Printf("failed to save item %q: %v", title, err)
			}
			continue
		}
		if created {
			metrics.RSSItemsParsedTotal.Add(1)
			newItems++
		}
	}

	rssLog.Printf("feed %q refreshed: %d items fetched, %d new, took %s", feed.Name, len(parsed.Items), newItems, time.Since(startedAt).Round(time.Millisecond))
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