package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

type stubSelector struct {
	item       models.Item
	err        error
	seenDevice models.Device
}

func (s *stubSelector) SelectNext(_ context.Context, _ *gorm.DB, device models.Device) (models.Item, error) {
	s.seenDevice = device
	return s.item, s.err
}

func TestGetNextItemRegistersDeviceAndUpdatesState(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)
	feed := createTestFeed(t, db, "feed-a", true)
	item := createTestItem(t, db, feed.ID, "fresh", now.Add(-time.Hour), nil)
	selector := &stubSelector{item: item}
	handler := &Handler{
		selector: selector,
		now: func() time.Time {
			return now
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/device/esp32-1/next", nil)
	req.Host = "example.com"
	req.SetPathValue("device_id", "esp32-1")
	rr := httptest.NewRecorder()

	handler.GetNextItem(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp NextItemResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Title != item.Title {
		t.Fatalf("expected title %q, got %q", item.Title, resp.Title)
	}
	if resp.Source != feed.Name {
		t.Fatalf("expected source %q, got %q", feed.Name, resp.Source)
	}
	if resp.ImageURL != "http://example.com/image/"+uintToString(item.ID)+".jpg" {
		t.Fatalf("unexpected image url %q", resp.ImageURL)
	}

	if selector.seenDevice.DeviceID != "esp32-1" {
		t.Fatalf("selector did not receive registered device, got %q", selector.seenDevice.DeviceID)
	}
	if selector.seenDevice.CurrentItemID != nil {
		t.Fatalf("new device should not have current item yet")
	}

	var device models.Device
	if err := db.First(&device, "device_id = ?", "esp32-1").Error; err != nil {
		t.Fatalf("failed to load device: %v", err)
	}
	if device.CurrentItemID == nil || *device.CurrentItemID != item.ID {
		t.Fatalf("expected current item %d, got %+v", item.ID, device.CurrentItemID)
	}
	if !device.LastSeen.Equal(now) {
		t.Fatalf("expected last_seen %s, got %s", now, device.LastSeen)
	}
	if !device.CreatedAt.Equal(now) {
		t.Fatalf("expected created_at %s, got %s", now, device.CreatedAt)
	}
}

func TestGetNextItemReturnsPlaceholderWhenSelectorHasNoItems(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)
	handler := &Handler{
		selector: &stubSelector{item: models.Item{ID: PlaceholderItemID, Title: PlaceholderTitle}},
		now: func() time.Time {
			return now
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/device/esp32-1/next", nil)
	req.Host = "example.com"
	req.SetPathValue("device_id", "esp32-1")
	rr := httptest.NewRecorder()

	handler.GetNextItem(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp NextItemResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Title != PlaceholderTitle {
		t.Fatalf("expected title %q, got %q", PlaceholderTitle, resp.Title)
	}
	if resp.Source != "System" {
		t.Fatalf("expected source %q, got %q", "System", resp.Source)
	}
	if resp.ImageURL != "http://example.com/image/0.jpg" {
		t.Fatalf("unexpected image url %q", resp.ImageURL)
	}

	var device models.Device
	if err := db.First(&device, "device_id = ?", "esp32-1").Error; err != nil {
		t.Fatalf("failed to load device: %v", err)
	}
	if device.CurrentItemID != nil {
		t.Fatalf("placeholder item should not update current item id")
	}
	if !device.LastSeen.Equal(now) {
		t.Fatalf("expected last_seen %s, got %s", now, device.LastSeen)
	}
}

func TestGetNextItemReturnsServerErrorWhenSelectorFails(t *testing.T) {
	_ = newTestDB(t)
	handler := &Handler{
		selector: &stubSelector{err: errors.New("boom")},
		now:      time.Now,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/device/esp32-1/next", nil)
	req.SetPathValue("device_id", "esp32-1")
	rr := httptest.NewRecorder()

	handler.GetNextItem(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func uintToString(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
