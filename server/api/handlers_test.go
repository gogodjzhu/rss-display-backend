package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	if resp.ItemID != item.ID {
		t.Fatalf("expected item_id %d, got %d", item.ID, resp.ItemID)
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
	if resp.ItemID != PlaceholderItemID {
		t.Fatalf("expected item_id %d, got %d", PlaceholderItemID, resp.ItemID)
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

func TestPostItemRatingPersistsRating(t *testing.T) {
	db := newTestDB(t)
	feed := createTestFeed(t, db, "feed-a", true)
	item := createTestItem(t, db, feed.ID, "fresh", time.Now().UTC(), nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/item/"+uintToString(item.ID)+"/rating", strings.NewReader(`{"rating":5,"device_id":"esp32-1"}`))
	req.SetPathValue("item_id", uintToString(item.ID))
	rr := httptest.NewRecorder()

	PostItemRating(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	var rating models.ItemRating
	if err := db.First(&rating).Error; err != nil {
		t.Fatalf("failed to load rating: %v", err)
	}
	if rating.ItemID != item.ID || rating.Rating != 5 || rating.DeviceID != "esp32-1" {
		t.Fatalf("unexpected persisted rating: %+v", rating)
	}
}

func TestPostItemRatingRejectsInvalidCases(t *testing.T) {
	db := newTestDB(t)
	feed := createTestFeed(t, db, "feed-a", true)
	item := createTestItem(t, db, feed.ID, "fresh", time.Now().UTC(), nil)

	tests := []struct {
		name       string
		itemID     string
		body       string
		wantStatus int
	}{
		{name: "placeholder", itemID: "0", body: `{"rating":3}`, wantStatus: http.StatusBadRequest},
		{name: "missing item", itemID: "9999", body: `{"rating":3}`, wantStatus: http.StatusNotFound},
		{name: "too low", itemID: uintToString(item.ID), body: `{"rating":0}`, wantStatus: http.StatusBadRequest},
		{name: "too high", itemID: uintToString(item.ID), body: `{"rating":6}`, wantStatus: http.StatusBadRequest},
		{name: "bad body", itemID: uintToString(item.ID), body: `{`, wantStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/item/"+tc.itemID+"/rating", strings.NewReader(tc.body))
			req.SetPathValue("item_id", tc.itemID)
			rr := httptest.NewRecorder()

			PostItemRating(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}

	var count int64
	if err := db.Model(&models.ItemRating{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count ratings: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no persisted ratings for invalid requests, got %d", count)
	}
}

func TestPostItemRatingRejectsInvalidItemID(t *testing.T) {
	_ = newTestDB(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/item/bad/rating", io.NopCloser(strings.NewReader(`{"rating":3}`)))
	req.SetPathValue("item_id", "bad")
	rr := httptest.NewRecorder()

	PostItemRating(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func uintToString(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
