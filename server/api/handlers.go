package api

import (
	"errors"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/metrics"
	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

type NextItemResponse struct {
	Title    string `json:"title"`
	ImageURL string `json:"image_url"`
	Source   string `json:"source"`
}

type Handler struct {
	selector NextItemSelector
	now      func() time.Time
}

func NewHandler(selector NextItemSelector) *Handler {
	if selector == nil {
		selector = NewWeightedNextItemSelector()
	}

	return &Handler{
		selector: selector,
		now:      time.Now,
	}
}

var defaultHandler = NewHandler(nil)

func GetNextItem(w http.ResponseWriter, r *http.Request) {
	defaultHandler.GetNextItem(w, r)
}

func (h *Handler) GetNextItem(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	db := database.GetDB()
	metrics.DeviceNextRequestTotal.Add(1)

	var device models.Device
	if err := db.Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Failed to load device", http.StatusInternalServerError)
			return
		}

		device = models.Device{
			DeviceID:  deviceID,
			CreatedAt: h.now(),
		}
		if err := db.Create(&device).Error; err != nil {
			http.Error(w, "Failed to register device", http.StatusInternalServerError)
			return
		}
		metrics.DeviceRegisteredTotal.Add(1)
	}

	item, err := h.selector.SelectNext(r.Context(), db, device)
	if err != nil {
		http.Error(w, "Failed to select next item", http.StatusInternalServerError)
		return
	}

	feedName := "System"
	if item.FeedID != 0 {
		var feed models.Feed
		if err := db.First(&feed, item.FeedID).Error; err != nil {
			http.Error(w, "Feed not found", http.StatusInternalServerError)
			return
		}
		feedName = feed.Name
	}

	imageURL := ""
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	imageURL = fmt.Sprintf("%s://%s/image/%d.jpg", scheme, r.Host, item.ID)

	if item.ID != PlaceholderItemID {
		device.CurrentItemID = &item.ID
	}
	device.LastSeen = h.now()
	if err := db.Save(&device).Error; err != nil {
		http.Error(w, "Failed to update device state", http.StatusInternalServerError)
		return
	}

	resp := NextItemResponse{
		Title:    item.Title,
		ImageURL: imageURL,
		Source:   feedName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
