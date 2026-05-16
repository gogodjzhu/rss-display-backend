package image

import (
	"image"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/esp32-rss-display/backend/server/api"
	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/metrics"
	"github.com/esp32-rss-display/backend/server/models"
	rssworker "github.com/esp32-rss-display/backend/server/rss"
)

var imageLog = logger.Get("image")

type Handler struct {
	renderer *rssworker.Renderer
}

func New(cfg *config.RSSConfig) *Handler {
	return &Handler{renderer: rssworker.NewRenderer(cfg)}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	base := filepath.Base(r.URL.Path)
	idStr := base[:len(base)-len(filepath.Ext(base))]

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	if id == uint64(api.PlaceholderItemID) {
		card := h.renderer.GenerateColorCard(api.PlaceholderTitle)
		h.renderer.OverlayTextFull(card, "System", api.PlaceholderTitle, time.Now().UTC())
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

	db := database.GetDB()
	metrics.ImageRenderTotal.Add(1)

	var item models.Item
	if err := db.Select("id", "feed_id", "title", "image_url", "published_at", "created_at").First(&item, id).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	var feed models.Feed
	if err := db.Select("name").First(&feed, item.FeedID).Error; err != nil {
		http.Error(w, "Feed not found", http.StatusInternalServerError)
		return
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
			h.renderer.OverlayTextFull(card, feed.Name, item.Title, pubTime)
			canvas = card
		} else {
			photo := h.renderer.ResizeImage(src)
			h.renderer.OverlayText(photo, feed.Name, item.Title, pubTime)
			canvas = photo
		}
	} else {
		metrics.ImageColorCardTotal.Add(1)
		card := h.renderer.GenerateColorCard(item.Title)
		h.renderer.OverlayTextFull(card, feed.Name, item.Title, pubTime)
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

func Mount(mux *http.ServeMux, cfg *config.RSSConfig) {
	handler := New(cfg)
	mux.Handle("/image/", handler)
}
