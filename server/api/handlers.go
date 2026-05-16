package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/metrics"
	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

var apiLog = logger.Get("api")
var ratingLog = logger.Get("rating")

type NextItemResponse struct {
	ItemID   uint   `json:"item_id"`
	Title    string `json:"title"`
	ImageURL string `json:"image_url"`
	Source   string `json:"source"`
}

type ItemRatingRequest struct {
	Rating   int    `json:"rating"`
	DeviceID string `json:"device_id"`
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
	if item.ID != PlaceholderItemID {
		show := models.ItemShow{
			ItemID:   item.ID,
			DeviceID: device.DeviceID,
		}
		if err := db.Create(&show).Error; err != nil {
			http.Error(w, "Failed to record item show", http.StatusInternalServerError)
			return
		}
	}

	resp := NextItemResponse{
		ItemID:   item.ID,
		Title:    item.Title,
		ImageURL: imageURL,
		Source:   feedName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func PostItemRating(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseUint(r.PathValue("item_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid item_id", http.StatusBadRequest)
		return
	}
	if itemID == uint64(PlaceholderItemID) {
		http.Error(w, "placeholder items cannot be rated", http.StatusBadRequest)
		return
	}

	var req ItemRatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		http.Error(w, "rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	db := database.GetDB()
	var item models.Item
	if err := db.Select("id").First(&item, itemID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load item", http.StatusInternalServerError)
		return
	}

	rating := models.ItemRating{
		ItemID:   uint(itemID),
		DeviceID: req.DeviceID,
		Rating:   req.Rating,
	}
	if err := db.Create(&rating).Error; err != nil {
		http.Error(w, "failed to save rating", http.StatusInternalServerError)
		return
	}
	ratingLog.Printf("Updated rating for item %d from device %s: %d", itemID, req.DeviceID, req.Rating)

	w.WriteHeader(http.StatusNoContent)
}
