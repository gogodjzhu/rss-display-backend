package api

import (
	"image"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/domain/feeds"
	"github.com/esp32-rss-display/backend/server/domain/items"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/metrics"
	rssworker "github.com/esp32-rss-display/backend/server/rss"
)

var imageLog = logger.Get("image")

type ImageHandler struct {
	renderer *rssworker.Renderer
	itemSvc  items.Service
	feedSvc  feeds.Service
}

func NewImageHandler(cfg *config.RSSConfig, itemSvc items.Service, feedSvc feeds.Service) *ImageHandler {
	return &ImageHandler{
		renderer: rssworker.NewRenderer(cfg),
		itemSvc:  itemSvc,
		feedSvc:  feedSvc,
	}
}

func (h *ImageHandler) ShowImage(w http.ResponseWriter, r *http.Request) {
	base := filepath.Base(r.URL.Path)
	idStr := base[:len(base)-len(filepath.Ext(base))]

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	if id == uint64(PlaceholderItemID) {
		card := h.renderer.GenerateColorCard(PlaceholderTitle)
		h.renderer.OverlayTextFull(card, "System", PlaceholderTitle, time.Now().UTC())
		encoded, err := h.renderer.EncodeJPEG(card)
		if err != nil {
			http.Error(w, "Failed to render image", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(encoded)
		return
	}

	ctx := r.Context()
	metrics.ImageRenderTotal.Add(1)

	item, err := h.itemSvc.FindByIDFull(ctx, uint(id))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	feedName := "System"
	if item.FeedID != 0 {
		feed, err := h.feedSvc.FindByID(ctx, item.FeedID)
		if err == nil && feed != nil {
			feedName = feed.Name
		}
	}

	pubTime := item.CreatedAt
	if item.PublishedAt != nil {
		pubTime = *item.PublishedAt
	}

	var canvas image.Image

	if item.ImageURL != "" {
		src, err := h.renderer.DownloadImage(item.ImageURL)
		if err != nil {
			metrics.ImageDownloadFailureTotal.Add(1)
			metrics.ImageColorCardTotal.Add(1)
			imageLog.Printf("render failed for item %d from %s: %v", item.ID, item.ImageURL, err)

			card := h.renderer.GenerateColorCard(item.Title)
			h.renderer.OverlayTextFull(card, feedName, item.Title, pubTime)
			canvas = card
		} else {
			photo := h.renderer.ResizeImage(src)
			h.renderer.OverlayText(photo, feedName, item.Title, pubTime)
			canvas = photo
		}
	} else {
		metrics.ImageColorCardTotal.Add(1)
		card := h.renderer.GenerateColorCard(item.Title)
		h.renderer.OverlayTextFull(card, feedName, item.Title, pubTime)
		canvas = card
	}

	encoded, err := h.renderer.EncodeJPEG(canvas)
	if err != nil {
		http.Error(w, "Failed to render image", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded)
}