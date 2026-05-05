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

	var show models.ItemShow
	if err := db.First(&show).Error; err != nil {
		t.Fatalf("failed to load show record: %v", err)
	}
	if show.ItemID != item.ID || show.DeviceID != "esp32-1" {
		t.Fatalf("unexpected persisted show: %+v", show)
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

	var showCount int64
	if err := db.Model(&models.ItemShow{}).Count(&showCount).Error; err != nil {
		t.Fatalf("failed to count show records: %v", err)
	}
	if showCount != 0 {
		t.Fatalf("placeholder item should not create show records, got %d", showCount)
	}
}

func TestGetNextItemReturnsServerErrorWhenSelectorFails(t *testing.T) {
	db := newTestDB(t)
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

	var showCount int64
	if err := db.Model(&models.ItemShow{}).Count(&showCount).Error; err != nil {
		t.Fatalf("failed to count show records: %v", err)
	}
	if showCount != 0 {
		t.Fatalf("failed selector request should not create show records, got %d", showCount)
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

func TestNFCRedirectPersistsReadOnSuccessfulRedirect(t *testing.T) {
	db := newTestDB(t)
	feed := createTestFeed(t, db, "feed-a", true)
	item := createTestItem(t, db, feed.ID, "fresh", time.Now().UTC(), nil)
	device := models.Device{
		DeviceID:      "esp32-1",
		CurrentItemID: &item.ID,
		LastSeen:      time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/nfc/esp32-1", nil)
	req.SetPathValue("device_id", "esp32-1")
	rr := httptest.NewRecorder()

	NFCRedirect(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rr.Code, rr.Body.String())
	}
	if location := rr.Header().Get("Location"); location != item.URL {
		t.Fatalf("expected redirect to %q, got %q", item.URL, location)
	}

	var read models.ItemRead
	if err := db.First(&read).Error; err != nil {
		t.Fatalf("failed to load read record: %v", err)
	}
	if read.ItemID != item.ID || read.DeviceID != device.DeviceID {
		t.Fatalf("unexpected read record: %+v", read)
	}
}

func TestNFCRedirectDoesNotPersistReadForInvalidRequests(t *testing.T) {
	db := newTestDB(t)
	feed := createTestFeed(t, db, "feed-a", true)
	item := createTestItem(t, db, feed.ID, "fresh", time.Now().UTC(), nil)

	deviceWithNoCurrentItem := models.Device{
		DeviceID:  "esp32-no-current",
		LastSeen:  time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&deviceWithNoCurrentItem).Error; err != nil {
		t.Fatalf("failed to create device without current item: %v", err)
	}

	missingItemID := item.ID + 999
	deviceWithMissingItem := models.Device{
		DeviceID:      "esp32-missing-item",
		CurrentItemID: &missingItemID,
		LastSeen:      time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
	if err := db.Create(&deviceWithMissingItem).Error; err != nil {
		t.Fatalf("failed to create device with missing item: %v", err)
	}

	deviceWithNoURL := models.Device{
		DeviceID:      "esp32-no-url",
		CurrentItemID: &item.ID,
		LastSeen:      time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
	if err := db.Create(&deviceWithNoURL).Error; err != nil {
		t.Fatalf("failed to create device with current item: %v", err)
	}
	if err := db.Model(&models.Item{}).Where("id = ?", item.ID).Update("url", "").Error; err != nil {
		t.Fatalf("failed to blank item url: %v", err)
	}

	tests := []struct {
		name       string
		deviceID   string
		wantStatus int
	}{
		{name: "unknown device", deviceID: "missing-device", wantStatus: http.StatusNotFound},
		{name: "no current item", deviceID: "esp32-no-current", wantStatus: http.StatusNotFound},
		{name: "missing current item", deviceID: "esp32-missing-item", wantStatus: http.StatusNotFound},
		{name: "missing item url", deviceID: "esp32-no-url", wantStatus: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/nfc/"+tc.deviceID, nil)
			req.SetPathValue("device_id", tc.deviceID)
			rr := httptest.NewRecorder()

			NFCRedirect(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}

	var count int64
	if err := db.Model(&models.ItemRead{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count read records: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no read records for invalid redirects, got %d", count)
	}
}

func uintToString(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
