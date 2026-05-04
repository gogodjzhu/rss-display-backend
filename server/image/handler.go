package image

import (
	"image"
	"log"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/metrics"
	"github.com/esp32-rss-display/backend/server/models"
	rssworker "github.com/esp32-rss-display/backend/server/rss"
)

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

	db := database.GetDB()
	metrics.ImageRenderTotal.Add(1)

	var item models.Item
	if err := db.Select("id", "title", "image_url", "published_at", "created_at").First(&item, id).Error; err != nil {
		http.NotFound(w, r)
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
			log.Printf("[image] render failed for item %d from %s: %v", item.ID, item.ImageURL, err)

			card := h.renderer.GenerateColorCard(item.Title)
			h.renderer.OverlayTextFull(card, item.Title, pubTime)
			canvas = card
		} else {
			photo := h.renderer.ResizeImage(src)
			h.renderer.OverlayText(photo, item.Title, pubTime)
			canvas = photo
		}
	} else {
		metrics.ImageColorCardTotal.Add(1)
		card := h.renderer.GenerateColorCard(item.Title)
		h.renderer.OverlayTextFull(card, item.Title, pubTime)
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
