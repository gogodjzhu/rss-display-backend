package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

func createTestItems(t *testing.T, db *gorm.DB, count int) []models.Item {
	t.Helper()
	feed := models.Feed{Name: "TestFeed", URL: "https://example.com/feed.xml", Enabled: true}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	items := make([]models.Item, count)
	for i := 0; i < count; i++ {
		publishedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour)
		items[i] = models.Item{
			FeedID:      feed.ID,
			Title:       "Test Item " + string(rune('A'+i)),
			URL:         "https://example.com/article-" + string(rune('A'+i)),
			PublishedAt: &publishedAt,
		}
		if err := db.Create(&items[i]).Error; err != nil {
			t.Fatalf("failed to create item %d: %v", i, err)
		}
	}
	return items
}

func TestPythonRunnerWriteAndReadJSON(t *testing.T) {
	runner := &PythonRunner{DataDir: t.TempDir()}

	data := map[string]any{
		"test":  "value",
		"count": 42,
	}
	path := filepath.Join(runner.DataDir, "test.json")

	if err := runner.WriteJSON(path, data); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var result map[string]any
	if err := runner.ReadJSON(path, &result); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}

	if result["test"] != "value" {
		t.Errorf("expected test=value, got %v", result["test"])
	}
	if count, ok := result["count"].(float64); !ok || count != 42 {
		t.Errorf("expected count=42, got %v", result["count"])
	}
}

func TestPythonRunnerInputOutputPaths(t *testing.T) {
	runner := &PythonRunner{DataDir: "/tmp/data/pipeline"}

	inPath := runner.InputPath(1, "filter_l1")
	expected := "/tmp/data/pipeline/1_filter_l1_in.json"
	if inPath != expected {
		t.Errorf("expected %s, got %s", expected, inPath)
	}

	outPath := runner.OutputPath(1, "filter_l1")
	expectedOut := "/tmp/data/pipeline/1_filter_l1_out.json"
	if outPath != expectedOut {
		t.Errorf("expected %s, got %s", expectedOut, outPath)
	}
}

func TestPipelineStartRejectsConcurrentTasks(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")

	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)

	device := models.Device{DeviceID: "test-device-1", CreatedAt: time.Now(), LastSeen: time.Now()}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	task1, err := p.StartPipeline("test-device-1", start, end)
	if err != nil {
		t.Fatalf("first StartPipeline should succeed, got: %v", err)
	}
	if task1.Status != "pending" {
		t.Errorf("expected status=pending, got %s", task1.Status)
	}

	_, err = p.StartPipeline("test-device-1", start, end)
	if err != ErrTaskRunning {
		t.Errorf("second StartPipeline should return ErrTaskRunning, got: %v", err)
	}

	runningMu.Lock()
	runningTaskID = nil
	runningMu.Unlock()
}

func TestPipelineStartCreatesTask(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")

	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)

	device := models.Device{DeviceID: "test-device-2", CreatedAt: time.Now(), LastSeen: time.Now()}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	runningMu.Lock()
	runningTaskID = nil
	runningMu.Unlock()

	task, err := p.StartPipeline("test-device-2", start, end)
	if err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	if task.DeviceID != "test-device-2" {
		t.Errorf("expected device_id=test-device-2, got %s", task.DeviceID)
	}
	if task.Status != "pending" {
		t.Errorf("expected status=pending, got %s", task.Status)
	}
	if task.TimeRangeStart == nil || !task.TimeRangeStart.Equal(start) {
		t.Error("time_range_start mismatch")
	}
	if task.TimeRangeEnd == nil || !task.TimeRangeEnd.Equal(end) {
		t.Error("time_range_end mismatch")
	}

	runningMu.Lock()
	runningTaskID = nil
	runningMu.Unlock()
}

func TestPipelineStartAutoCreatesDevice(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")

	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)

	runningMu.Lock()
	runningTaskID = nil
	runningMu.Unlock()

	task, err := p.StartPipeline("new-device-1", start, end)
	if err != nil {
		t.Fatalf("StartPipeline failed for new device: %v", err)
	}

	var device models.Device
	if err := db.Where("device_id = ?", "new-device-1").First(&device).Error; err != nil {
		t.Fatalf("device should have been auto-created: %v", err)
	}

	_ = task

	runningMu.Lock()
	runningTaskID = nil
	runningMu.Unlock()
}

func TestBuildFilterL1Input(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")

	device := models.Device{
		DeviceID:    "test-device-l1",
		Role:        "developer",
		Preference:  "Go, distributed systems",
		CreatedAt:   time.Now(),
		LastSeen:    time.Now(),
	}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	items := createTestItems(t, db, 3)

	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)

	task := &models.Task{
		DeviceID:       device.DeviceID,
		Status:         "filtering_l1",
		TimeRangeStart: &start,
		TimeRangeEnd:   &end,
		Level1IDs:      "",
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	input, err := p.buildFilterL1Input(task, db)
	if err != nil {
		t.Fatalf("buildFilterL1Input failed: %v", err)
	}

	inputMap, ok := input.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", input)
	}

	devMap, ok := inputMap["device"].(map[string]string)
	if !ok {
		t.Fatalf("expected device map, got %T", inputMap["device"])
	}
	if devMap["role"] != "developer" {
		t.Errorf("expected role=developer, got %s", devMap["role"])
	}

	itemsSlice, ok := inputMap["items"]
	if !ok {
		t.Fatalf("expected items key in input")
	}
	itemsBytes, err := json.Marshal(itemsSlice)
	if err != nil {
		t.Fatalf("failed to marshal items: %v", err)
	}
	var parsedItems []struct {
		ID    uint   `json:"id"`
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal(itemsBytes, &parsedItems); err != nil {
		t.Fatalf("failed to unmarshal items: %v", err)
	}
	if len(parsedItems) != len(items) {
		t.Errorf("expected %d items, got %d", len(items), len(parsedItems))
	}
}

func TestBuildCrawlInput(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")

	items := createTestItems(t, db, 2)

	idsJSON, _ := json.Marshal([]uint{items[0].ID, items[1].ID})
	task := &models.Task{
		DeviceID:  "test-device-crawl",
		Status:    "crawling",
		Level1IDs: string(idsJSON),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	input, err := p.buildCrawlInput(task, db)
	if err != nil {
		t.Fatalf("buildCrawlInput failed: %v", err)
	}

	inputMap, ok := input.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", input)
	}

	itemsSlice, ok := inputMap["items"]
	if !ok {
		t.Fatalf("expected items key in input")
	}
	itemsBytes, err := json.Marshal(itemsSlice)
	if err != nil {
		t.Fatalf("failed to marshal items: %v", err)
	}
	var parsedItems []struct {
		ID  uint   `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(itemsBytes, &parsedItems); err != nil {
		t.Fatalf("failed to unmarshal items: %v", err)
	}
	if len(parsedItems) != 2 {
		t.Errorf("expected 2 items, got %d", len(parsedItems))
	}
}

func TestApplyCrawlResult(t *testing.T) {
	db := setupDB(t)
	p, dataDir := newPipeline(t, "python3", "py/pipeline.py")

	items := createTestItems(t, db, 1)

	task := &models.Task{
		DeviceID:  "test-device-crawl-apply",
		Status:    "crawling",
		Level1IDs: "",
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	result := map[string]any{
		"results": []map[string]any{
			{"id": float64(items[0].ID), "content": "# Hello\n\nWorld", "success": true},
		},
	}
	outPath := filepath.Join(dataDir, "crawl_out.json")
	p.runner.WriteJSON(outPath, result)

	err := p.applyCrawlResult(task, outPath, db)
	if err != nil {
		t.Fatalf("applyCrawlResult failed: %v", err)
	}

	var updatedItem models.Item
	if err := db.First(&updatedItem, items[0].ID).Error; err != nil {
		t.Fatalf("failed to load item: %v", err)
	}
	if updatedItem.Content != "# Hello\n\nWorld" {
		t.Errorf("expected content to be updated, got %q", updatedItem.Content)
	}
}

func TestParseIDsFromJSON(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []uint
		hasErr bool
	}{
		{"empty string", "", nil, false},
		{"valid array", "[1, 2, 3]", []uint{1, 2, 3}, false},
		{"single element", "[42]", []uint{42}, false},
		{"invalid json", "not json", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := parseIDsFromJSON(tt.input)
			if tt.hasErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.hasErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.hasErr {
				if len(ids) != len(tt.expect) {
					t.Errorf("expected %v, got %v", tt.expect, ids)
				}
				for i, id := range ids {
					if id != tt.expect[i] {
						t.Errorf("index %d: expected %d, got %d", i, tt.expect[i], id)
					}
				}
			}
		})
	}
}

func TestEnsureDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "nested", "data")

	runner := &PythonRunner{DataDir: dataDir}
	if err := runner.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir failed: %v", err)
	}

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("data dir should exist")
	}
}