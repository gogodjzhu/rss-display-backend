package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/esp32-rss-display/backend/server/domain/devices"
	"github.com/esp32-rss-display/backend/server/domain/feeds"
	"github.com/esp32-rss-display/backend/server/domain/items"
	"github.com/esp32-rss-display/backend/server/logger"
	"github.com/esp32-rss-display/backend/server/metrics"
)

var apiLog = logger.Get("api")

const PlaceholderItemID = items.PlaceholderItemID
const PlaceholderTitle = items.PlaceholderTitle

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

type DefaultHandler struct {
	deviceSvc devices.Service
	itemSvc   items.Service
	feedSvc   feeds.Service
	selector  items.ItemSelector
}

func NewDefaultHandler(deviceSvc devices.Service, itemSvc items.Service, feedSvc feeds.Service, selector items.ItemSelector) *DefaultHandler {
	return &DefaultHandler{
		deviceSvc: deviceSvc,
		itemSvc:   itemSvc,
		feedSvc:   feedSvc,
		selector:  selector,
	}
}

func (h *DefaultHandler) GetNextItem(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	metrics.DeviceNextRequestTotal.Add(1)

	device, err := h.deviceSvc.GetOrCreate(ctx, deviceID)
	if err != nil {
		http.Error(w, "Failed to load device", http.StatusInternalServerError)
		return
	}

	item, err := h.selector.Select(ctx, *device)
	if err != nil {
		http.Error(w, "Failed to select next item", http.StatusInternalServerError)
		return
	}

	feedName := "System"
	if item.FeedID != 0 {
		feed, err := h.feedSvc.FindByID(ctx, item.FeedID)
		if err == nil && feed != nil {
			feedName = feed.Name
		}
	}

	imageURL := ""
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	imageURL = fmt.Sprintf("%s://%s/image/%d.jpg", scheme, r.Host, item.ID)

	if item.ID != PlaceholderItemID {
		if err := h.deviceSvc.UpdateCurrentItem(ctx, deviceID, item.ID); err != nil {
			apiLog.Printf("failed to update current item: %v", err)
		}
		if err := h.deviceSvc.TouchLastSeen(ctx, deviceID); err != nil {
			apiLog.Printf("failed to touch last seen: %v", err)
		}
		if err := h.itemSvc.RecordShow(ctx, deviceID, item.ID); err != nil {
			apiLog.Printf("failed to record show: %v", err)
		}
	} else {
		if err := h.deviceSvc.TouchLastSeen(ctx, deviceID); err != nil {
			apiLog.Printf("failed to touch last seen: %v", err)
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

func (h *DefaultHandler) PostItemRating(w http.ResponseWriter, r *http.Request) {
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

	ctx := r.Context()
	if err := h.itemSvc.UpdateRating(ctx, req.DeviceID, uint(itemID), req.Rating); err != nil {
		if errors.Is(err, items.ErrNotFound) {
			http.Error(w, "item not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to save rating", http.StatusInternalServerError)
		return
	}
	apiLog.Printf("Updated rating for item %d from device %s: %d", itemID, req.DeviceID, req.Rating)

	w.WriteHeader(http.StatusNoContent)
}