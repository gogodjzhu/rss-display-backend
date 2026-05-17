package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	admindomain "github.com/esp32-rss-display/backend/server/domain/admin"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDashboardRendersSummarySections(t *testing.T) {
	db := newTestDB(t)
	seedAdminData(t, db)
	handler := NewHandler(admindomain.NewGORMReadModel(db))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.Dashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"RSS Admin", "Dashboard", "Recent Items", "Recent Shows", "Recent Reads", "Recent Ratings"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected dashboard body to contain %q, got %s", want, body)
		}
	}
}

func TestAdminDetailPagesRenderExistingRecords(t *testing.T) {
	db := newTestDB(t)
	seed := seedAdminData(t, db)
	handler := NewHandler(admindomain.NewGORMReadModel(db))

	tests := []struct {
		name    string
		path    string
		setPath func(*http.Request)
		want    []string
		handle  func(http.ResponseWriter, *http.Request)
	}{
		{
			name: "feed detail",
			path: "/admin/feeds/1",
			setPath: func(req *http.Request) {
				req.SetPathValue("id", uintToString(seed.feed.ID))
			},
			want:   []string{seed.feed.Name, seed.item.Title, "Shows"},
			handle: handler.ShowFeed,
		},
		{
			name: "item detail",
			path: "/admin/items/1",
			setPath: func(req *http.Request) {
				req.SetPathValue("id", uintToString(seed.item.ID))
			},
			want:   []string{seed.item.Title, "Show Records", "Read Records", "Rating Records"},
			handle: handler.ShowItem,
		},
		{
			name: "device detail",
			path: "/admin/devices/esp32-1",
			setPath: func(req *http.Request) {
				req.SetPathValue("device_id", seed.device.DeviceID)
			},
			want:   []string{seed.device.DeviceID, "Show Records", "Read Records", "Rating Records", seed.item.Title},
			handle: handler.ShowDevice,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			tc.setPath(req)
			rr := httptest.NewRecorder()

			tc.handle(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			body := rr.Body.String()
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Fatalf("expected body to contain %q, got %s", want, body)
				}
			}
		})
	}
}

func TestAdminDetailPagesReturnNotFoundForMissingRecords(t *testing.T) {
	db := newTestDB(t)
	handler := NewHandler(admindomain.NewGORMReadModel(db))

	tests := []struct {
		name   string
		path   string
		setPath func(*http.Request)
		handle func(http.ResponseWriter, *http.Request)
	}{
		{name: "missing feed", path: "/admin/feeds/999", setPath: func(req *http.Request) { req.SetPathValue("id", "999") }, handle: handler.ShowFeed},
		{name: "missing item", path: "/admin/items/999", setPath: func(req *http.Request) { req.SetPathValue("id", "999") }, handle: handler.ShowItem},
		{name: "missing device", path: "/admin/devices/missing", setPath: func(req *http.Request) { req.SetPathValue("device_id", "missing") }, handle: handler.ShowDevice},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			tc.setPath(req)
			rr := httptest.NewRecorder()

			tc.handle(rr, req)

			if rr.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAdminListPagesArePaginated(t *testing.T) {
	db := newTestDB(t)
	seedAdminListData(t, db, 25)
	handler := NewHandler(admindomain.NewGORMReadModel(db))

	tests := []struct {
		name   string
		path   string
		handle func(http.ResponseWriter, *http.Request)
		want   []string
		notWant []string
	}{
		{name: "feeds page 1", path: "/admin/feeds?page=1", handle: handler.ListFeeds, want: []string{"Feed 00", "Page 1 / 2", "Next"}, notWant: []string{"Feed 24"}},
		{name: "feeds page 2", path: "/admin/feeds?page=2", handle: handler.ListFeeds, want: []string{"Feed 24", "Page 2 / 2", "Previous"}, notWant: []string{"Feed 00"}},
		{name: "items page 2", path: "/admin/items?page=2", handle: handler.ListItems, want: []string{"Item 04", "Page 2 / 2"}, notWant: []string{"Item 24"}},
		{name: "devices page 2", path: "/admin/devices?page=2", handle: handler.ListDevices, want: []string{"device-04", "Page 2 / 2"}, notWant: []string{"device-24"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()

			tc.handle(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			body := rr.Body.String()
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Fatalf("expected body to contain %q, got %s", want, body)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(body, notWant) {
					t.Fatalf("expected body not to contain %q, got %s", notWant, body)
				}
			}
		})
	}
}

func TestFeedDetailItemsArePaginated(t *testing.T) {
	db := newTestDB(t)
	feed := seedAdminFeedDetailData(t, db, 25)
	handler := NewHandler(admindomain.NewGORMReadModel(db))

	req := httptest.NewRequest(http.MethodGet, "/admin/feeds/"+uintToString(feed.ID)+"?page=2", nil)
	req.SetPathValue("id", uintToString(feed.ID))
	rr := httptest.NewRecorder()

	handler.ShowFeed(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Item 04") || !strings.Contains(body, "Page 2 / 2") {
		t.Fatalf("expected paginated feed detail, got %s", body)
	}
	if strings.Contains(body, "Item 24") {
		t.Fatalf("expected second page not to contain first-page items, got %s", body)
	}
}

func TestItemsListFiltersAndSorts(t *testing.T) {
	db := newTestDB(t)
	seedAdminFilterData(t, db)
	handler := NewHandler(admindomain.NewGORMReadModel(db))

	req := httptest.NewRequest(http.MethodGet, "/admin/items?title=Beta&feed_id=2&from=2026-05-02&to=2026-05-03&sort=shows_desc", nil)
	rr := httptest.NewRecorder()

	handler.ListItems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Beta Story") {
		t.Fatalf("expected filtered item to appear, got %s", body)
	}
	for _, unwanted := range []string{"Alpha Story", "Gamma Story"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("expected filtered items to exclude %q, got %s", unwanted, body)
		}
	}
}

func TestItemsListSortsByShowsDescending(t *testing.T) {
	db := newTestDB(t)
	seedAdminFilterData(t, db)
	handler := NewHandler(admindomain.NewGORMReadModel(db))

	req := httptest.NewRequest(http.MethodGet, "/admin/items?sort=shows_desc", nil)
	rr := httptest.NewRecorder()

	handler.ListItems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	first := strings.Index(body, "Gamma Story")
	second := strings.Index(body, "Beta Story")
	third := strings.Index(body, "Alpha Story")
	if !(first >= 0 && second >= 0 && third >= 0 && first < second && second < third) {
		t.Fatalf("expected shows_desc ordering, got %s", body)
	}
}

type adminSeed struct {
	feed   models.Feed
	item   models.Item
	device models.Device
}

func seedAdminData(t *testing.T, db *gorm.DB) adminSeed {
	t.Helper()

	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	feed := models.Feed{Name: "feed-a", URL: "https://example.com/feed", Enabled: true, CreatedAt: now}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	item := models.Item{FeedID: feed.ID, Title: "Item A", URL: "https://example.com/item-a", ImageURL: "https://example.com/item-a.jpg", CreatedAt: now, PublishedAt: &now}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	device := models.Device{DeviceID: "esp32-1", CurrentItemID: &item.ID, LastSeen: now, CreatedAt: now}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	show := models.ItemShow{ItemID: item.ID, DeviceID: device.DeviceID, CreatedAt: now}
	if err := db.Create(&show).Error; err != nil {
		t.Fatalf("failed to create show: %v", err)
	}
	read := models.ItemRead{ItemID: item.ID, DeviceID: device.DeviceID, CreatedAt: now}
	if err := db.Create(&read).Error; err != nil {
		t.Fatalf("failed to create read: %v", err)
	}
	rating := models.ItemRating{ItemID: item.ID, DeviceID: device.DeviceID, Rating: 4, CreatedAt: now}
	if err := db.Create(&rating).Error; err != nil {
		t.Fatalf("failed to create rating: %v", err)
	}

	return adminSeed{feed: feed, item: item, device: device}
}

func seedAdminListData(t *testing.T, db *gorm.DB, count int) models.Feed {
	t.Helper()

	base := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	var firstFeed models.Feed
	for i := 0; i < count; i++ {
		feed := models.Feed{Name: fmt.Sprintf("Feed %02d", i), URL: fmt.Sprintf("https://example.com/feed-%02d", i), Enabled: true, CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		if err := db.Create(&feed).Error; err != nil {
			t.Fatalf("failed to create feed %d: %v", i, err)
		}
		if i == 0 {
			firstFeed = feed
		}
		itemTime := base.Add(time.Duration(i) * time.Minute)
		item := models.Item{FeedID: feed.ID, Title: fmt.Sprintf("Item %02d", i), URL: fmt.Sprintf("https://example.com/item-%02d", i), ImageURL: "https://example.com/image.jpg", CreatedAt: itemTime, PublishedAt: &itemTime}
		if err := db.Create(&item).Error; err != nil {
			t.Fatalf("failed to create item %d: %v", i, err)
		}
		deviceID := fmt.Sprintf("device-%02d", i)
		device := models.Device{DeviceID: deviceID, CurrentItemID: &item.ID, LastSeen: itemTime, CreatedAt: itemTime}
		if err := db.Create(&device).Error; err != nil {
			t.Fatalf("failed to create device %d: %v", i, err)
		}
		if err := db.Create(&models.ItemShow{ItemID: item.ID, DeviceID: deviceID, CreatedAt: itemTime}).Error; err != nil {
			t.Fatalf("failed to create show %d: %v", i, err)
		}
		if err := db.Create(&models.ItemRead{ItemID: item.ID, DeviceID: deviceID, CreatedAt: itemTime}).Error; err != nil {
			t.Fatalf("failed to create read %d: %v", i, err)
		}
		if err := db.Create(&models.ItemRating{ItemID: item.ID, DeviceID: deviceID, Rating: (i%5 + 1), CreatedAt: itemTime}).Error; err != nil {
			t.Fatalf("failed to create rating %d: %v", i, err)
		}
	}

	return firstFeed
}

func seedAdminFeedDetailData(t *testing.T, db *gorm.DB, count int) models.Feed {
	t.Helper()

	base := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	feed := models.Feed{Name: "Feed Detail", URL: "https://example.com/feed-detail", Enabled: true, CreatedAt: base}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("failed to create feed detail seed: %v", err)
	}

	for i := 0; i < count; i++ {
		itemTime := base.Add(time.Duration(i) * time.Minute)
		item := models.Item{FeedID: feed.ID, Title: fmt.Sprintf("Item %02d", i), URL: fmt.Sprintf("https://example.com/feed-detail-item-%02d", i), ImageURL: "https://example.com/image.jpg", CreatedAt: itemTime, PublishedAt: &itemTime}
		if err := db.Create(&item).Error; err != nil {
			t.Fatalf("failed to create feed detail item %d: %v", i, err)
		}
		deviceID := fmt.Sprintf("feed-detail-device-%02d", i)
		if err := db.Create(&models.ItemShow{ItemID: item.ID, DeviceID: deviceID, CreatedAt: itemTime}).Error; err != nil {
			t.Fatalf("failed to create feed detail show %d: %v", i, err)
		}
		if err := db.Create(&models.ItemRead{ItemID: item.ID, DeviceID: deviceID, CreatedAt: itemTime}).Error; err != nil {
			t.Fatalf("failed to create feed detail read %d: %v", i, err)
		}
		if err := db.Create(&models.ItemRating{ItemID: item.ID, DeviceID: deviceID, Rating: 5, CreatedAt: itemTime}).Error; err != nil {
			t.Fatalf("failed to create feed detail rating %d: %v", i, err)
		}
	}

	return feed
}

func seedAdminFilterData(t *testing.T, db *gorm.DB) {
	t.Helper()

	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	feeds := []models.Feed{
		{Name: "Feed A", URL: "https://example.com/a", Enabled: true, CreatedAt: base},
		{Name: "Feed B", URL: "https://example.com/b", Enabled: true, CreatedAt: base.Add(time.Hour)},
	}
	for i := range feeds {
		if err := db.Create(&feeds[i]).Error; err != nil {
			t.Fatalf("failed to create filter feed %d: %v", i, err)
		}
	}

	items := []struct {
		feedID uint
		title  string
		time   time.Time
		shows  int
		reads  int
		rates  int
	}{
		{feedID: feeds[0].ID, title: "Alpha Story", time: base, shows: 1, reads: 1, rates: 1},
		{feedID: feeds[1].ID, title: "Beta Story", time: base.Add(24 * time.Hour), shows: 3, reads: 1, rates: 2},
		{feedID: feeds[1].ID, title: "Gamma Story", time: base.Add(48 * time.Hour), shows: 5, reads: 2, rates: 1},
	}
	for idx, itemSpec := range items {
		published := itemSpec.time
		item := models.Item{FeedID: itemSpec.feedID, Title: itemSpec.title, URL: fmt.Sprintf("https://example.com/item-%d", idx), ImageURL: "https://example.com/image.jpg", CreatedAt: itemSpec.time, PublishedAt: &published}
		if err := db.Create(&item).Error; err != nil {
			t.Fatalf("failed to create filter item %d: %v", idx, err)
		}
		for i := 0; i < itemSpec.shows; i++ {
			deviceID := fmt.Sprintf("show-device-%d-%d", idx, i)
			if err := db.Create(&models.ItemShow{ItemID: item.ID, DeviceID: deviceID, CreatedAt: itemSpec.time.Add(time.Duration(i) * time.Minute)}).Error; err != nil {
				t.Fatalf("failed to create show %d-%d: %v", idx, i, err)
			}
		}
		for i := 0; i < itemSpec.reads; i++ {
			deviceID := fmt.Sprintf("read-device-%d-%d", idx, i)
			if err := db.Create(&models.ItemRead{ItemID: item.ID, DeviceID: deviceID, CreatedAt: itemSpec.time.Add(time.Duration(i) * time.Minute)}).Error; err != nil {
				t.Fatalf("failed to create read %d-%d: %v", idx, i, err)
			}
		}
		for i := 0; i < itemSpec.rates; i++ {
			deviceID := fmt.Sprintf("rate-device-%d-%d", idx, i)
			if err := db.Create(&models.ItemRating{ItemID: item.ID, DeviceID: deviceID, Rating: 4, CreatedAt: itemSpec.time.Add(time.Duration(i) * time.Minute)}).Error; err != nil {
				t.Fatalf("failed to create rating %d-%d: %v", idx, i, err)
			}
		}
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "admin-test.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.Device{}, &models.Feed{}, &models.Item{}, &models.ItemShow{}, &models.ItemRead{}, &models.ItemRating{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	return db
}

func uintToString(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
