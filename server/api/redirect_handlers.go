package api

import (
	"errors"
	"net/http"

	"github.com/esp32-rss-display/backend/server/domain/devices"
	"github.com/esp32-rss-display/backend/server/domain/items"
)

type RedirectHandler struct {
	deviceSvc devices.Service
	itemSvc   items.Service
}

func NewRedirectHandler(deviceSvc devices.Service, itemSvc items.Service) *RedirectHandler {
	return &RedirectHandler{
		deviceSvc: deviceSvc,
		itemSvc:   itemSvc,
	}
}

func (h *RedirectHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	device, err := h.deviceSvc.GetOrCreate(ctx, deviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.CurrentItemID == nil {
		http.Error(w, "No current item for device", http.StatusNotFound)
		return
	}

	item, err := h.itemSvc.FindByIDFull(ctx, *device.CurrentItemID)
	if errors.Is(err, items.ErrNotFound) {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load item", http.StatusInternalServerError)
		return
	}

	if item.URL == "" {
		http.Error(w, "No URL for current item", http.StatusNotFound)
		return
	}

	if err := h.itemSvc.RecordRead(ctx, deviceID, item.ID); err != nil {
		http.Error(w, "Failed to record item read", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, item.URL, http.StatusFound)
}
