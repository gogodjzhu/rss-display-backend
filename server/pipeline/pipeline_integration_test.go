package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"gorm.io/gorm"
)

func createSeedData(t *testing.T, db *gorm.DB) (models.Device, []models.Item) {
	t.Helper()

	device := models.Device{
		DeviceID:   "dev-test-001",
		Role:       "software engineer",
		Preference: "Go, distributed systems, cloud native",
		CreatedAt:  time.Now(),
		LastSeen:   time.Now(),
	}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	feed := models.Feed{Name: "TechBlog", URL: "https://example.com/rss", Enabled: true}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	publishedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	items := []models.Item{
		{
			FeedID:      feed.ID,
			Title:       "Go 1.24 Runtime Improvements",
			URL:         "https://example.com/go-runtime",
			PublishedAt: &publishedAt,
		},
		{
			FeedID:      feed.ID,
			Title:       "Kubernetes Scaling Patterns",
			URL:         "https://example.com/k8s-scaling",
			PublishedAt: &publishedAt,
		},
		{
			FeedID:      feed.ID,
			Title:       "Celebrity Gossip Daily",
			URL:         "https://example.com/gossip",
			PublishedAt: &publishedAt,
		},
		{
			FeedID:      feed.ID,
			Title:       "Cloud Native Observability",
			URL:         "https://example.com/observability",
			PublishedAt: &publishedAt,
		},
		{
			FeedID:      feed.ID,
			Title:       "Best Pizza Places in NYC",
			URL:         "https://example.com/pizza",
			PublishedAt: &publishedAt,
		},
	}
	for i := range items {
		if err := db.Create(&items[i]).Error; err != nil {
			t.Fatalf("failed to create item %d: %v", i, err)
		}
	}

	return device, items
}

func createTask(t *testing.T, db *gorm.DB, deviceID string, start, end time.Time) *models.Task {
	t.Helper()
	task := &models.Task{
		DeviceID:       deviceID,
		Status:         "pending",
		TimeRangeStart: &start,
		TimeRangeEnd:   &end,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	return task
}

func resetRunning() {
	runningMu.Lock()
	runningTaskID = nil
	runningMu.Unlock()
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

// ======================================================================
// Stage 1: buildFilterL1Input — verifies input JSON structure
// ======================================================================

func TestBuildFilterL1Input_StructureAndTimeRange(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	device, items := createSeedData(t, db)

	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)

	task := createTask(t, db, device.DeviceID, start, end)

	t.Run("has_correct_device_info", func(t *testing.T) {
		input, err := p.buildFilterL1Input(task, db)
		if err != nil {
			t.Fatalf("buildFilterL1Input failed: %v", err)
		}

		inputMap := input.(map[string]any)

		devMap, ok := inputMap["device"].(map[string]string)
		if !ok {
			t.Fatalf("expected device as map[string]string, got %T", inputMap["device"])
		}
		if devMap["role"] != "software engineer" {
			t.Errorf("expected role=software engineer, got %q", devMap["role"])
		}
		if devMap["preference"] != "Go, distributed systems, cloud native" {
			t.Errorf("expected preference=Go, distributed systems, cloud native, got %q", devMap["preference"])
		}
	})

	t.Run("has_correct_item_count_and_fields", func(t *testing.T) {
		input, err := p.buildFilterL1Input(task, db)
		if err != nil {
			t.Fatalf("buildFilterL1Input failed: %v", err)
		}

		inputMap := input.(map[string]any)

		itemsBytes, _ := json.Marshal(inputMap["items"])
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

		for i, pi := range parsedItems {
			if pi.ID == 0 {
				t.Errorf("item %d has zero ID", i)
			}
			if pi.Title == "" {
				t.Errorf("item %d has empty title", i)
			}
			if pi.URL == "" {
				t.Errorf("item %d has empty URL", i)
			}
		}
	})

	t.Run("excludes_items_outside_time_range", func(t *testing.T) {
		outsideItem := models.Item{
			FeedID:      items[0].FeedID,
			Title:       "Old Article From 2025",
			URL:         "https://example.com/old",
			PublishedAt: ptrTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		}
		if err := db.Create(&outsideItem).Error; err != nil {
			t.Fatalf("failed to create outside item: %v", err)
		}

		input, err := p.buildFilterL1Input(task, db)
		if err != nil {
			t.Fatalf("buildFilterL1Input failed: %v", err)
		}

		inputMap := input.(map[string]any)
		itemsBytes, _ := json.Marshal(inputMap["items"])
		var parsedItems []struct {
			ID    uint   `json:"id"`
			Title string `json:"title"`
			URL   string `json:"url"`
		}
		json.Unmarshal(itemsBytes, &parsedItems)

		for _, pi := range parsedItems {
			if pi.Title == "Old Article From 2025" {
				t.Error("outside-range item should not appear in filter_l1 input")
			}
		}
	})
}

// ======================================================================
// Stage 2: buildCrawlInput — verifies crawl input JSON structure
// ======================================================================

func TestBuildCrawlInput_StructureAndIDs(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	selectedIDs := []uint{items[0].ID, items[1].ID, items[3].ID}
	idsJSON, _ := json.Marshal(selectedIDs)

	task := &models.Task{
		DeviceID:  "dev-test-001",
		Status:    "crawling",
		Level1IDs: string(idsJSON),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	t.Run("only_includes_level1_id_items", func(t *testing.T) {
		input, err := p.buildCrawlInput(task, db)
		if err != nil {
			t.Fatalf("buildCrawlInput failed: %v", err)
		}

		inputMap := input.(map[string]any)

		itemsBytes, _ := json.Marshal(inputMap["items"])
		var parsedItems []struct {
			ID  uint   `json:"id"`
			URL string `json:"url"`
		}
		if err := json.Unmarshal(itemsBytes, &parsedItems); err != nil {
			t.Fatalf("failed to unmarshal items: %v", err)
		}

		if len(parsedItems) != 3 {
			t.Fatalf("expected 3 items (matching level1_ids), got %d", len(parsedItems))
		}

		gotIDs := make([]uint, len(parsedItems))
		for i, pi := range parsedItems {
			gotIDs[i] = pi.ID
			if pi.URL == "" {
				t.Errorf("item %d has empty URL", i)
			}
		}
		sort.Slice(gotIDs, func(i, j int) bool { return gotIDs[i] < gotIDs[j] })

		expectedIDs := make([]uint, len(selectedIDs))
		copy(expectedIDs, selectedIDs)
		sort.Slice(expectedIDs, func(i, j int) bool { return expectedIDs[i] < expectedIDs[j] })

		for i := range gotIDs {
			if gotIDs[i] != expectedIDs[i] {
				t.Errorf("item[%d]: expected ID %d, got %d", i, expectedIDs[i], gotIDs[i])
			}
		}
	})

	t.Run("includes_rate_limits", func(t *testing.T) {
		input, err := p.buildCrawlInput(task, db)
		if err != nil {
			t.Fatalf("buildCrawlInput failed: %v", err)
		}

		inputMap := input.(map[string]any)
		minVal, ok := inputMap["rate_limit_min_seconds"].(int)
		if !ok || minVal != 30 {
			t.Errorf("expected rate_limit_min_seconds=30, got %v", inputMap["rate_limit_min_seconds"])
		}
		maxVal, ok := inputMap["rate_limit_max_seconds"].(int)
		if !ok || maxVal != 90 {
			t.Errorf("expected rate_limit_max_seconds=90, got %v", inputMap["rate_limit_max_seconds"])
		}
	})
}

// ======================================================================
// Stage 3: buildSummarizeInput — verifies summarize input JSON structure
// ======================================================================

func TestBuildSummarizeInput_StructureAndContent(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	items[0].Content = "Full article content about Go runtime."
	items[1].Content = "Kubernetes scaling deep dive content."
	items[3].Content = "Observability with OpenTelemetry."
	for _, i := range []int{0, 1, 3} {
		if err := db.Save(&items[i]).Error; err != nil {
			t.Fatalf("failed to save item %d: %v", i, err)
		}
	}

	selectedIDs := []uint{items[0].ID, items[1].ID, items[3].ID}
	idsJSON, _ := json.Marshal(selectedIDs)

	task := &models.Task{
		DeviceID:  "dev-test-001",
		Status:    "summarizing",
		Level1IDs: string(idsJSON),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	t.Run("includes_items_with_content", func(t *testing.T) {
		input, err := p.buildSummarizeInput(task, db)
		if err != nil {
			t.Fatalf("buildSummarizeInput failed: %v", err)
		}

		inputMap := input.(map[string]any)

		itemsBytes, _ := json.Marshal(inputMap["items"])
		var parsedItems []struct {
			ID      uint   `json:"id"`
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(itemsBytes, &parsedItems); err != nil {
			t.Fatalf("failed to unmarshal items: %v", err)
		}

		if len(parsedItems) != 3 {
			t.Errorf("expected 3 items, got %d", len(parsedItems))
		}

		for _, pi := range parsedItems {
			if pi.Content == "" {
				t.Errorf("item %d should have non-empty content", pi.ID)
			}
		}
	})

	t.Run("excludes_items_with_empty_content", func(t *testing.T) {
		emptyContentItem := models.Item{
			FeedID:  items[0].FeedID,
			Title:   "No Content Article",
			URL:     "https://example.com/no-content",
			Content: "",
		}
		if err := db.Create(&emptyContentItem).Error; err != nil {
			t.Fatalf("failed to create item: %v", err)
		}

		idsWithEmpty := append(selectedIDs, emptyContentItem.ID)
		idsJSON2, _ := json.Marshal(idsWithEmpty)
		task2 := &models.Task{
			DeviceID:  "dev-test-001",
			Status:    "summarizing",
			Level1IDs: string(idsJSON2),
		}
		db.Create(task2)

		input, err := p.buildSummarizeInput(task2, db)
		if err != nil {
			t.Fatalf("buildSummarizeInput failed: %v", err)
		}

		inputMap := input.(map[string]any)
		itemsBytes, _ := json.Marshal(inputMap["items"])
		var parsedItems []struct {
			ID      uint   `json:"id"`
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		json.Unmarshal(itemsBytes, &parsedItems)

		for _, pi := range parsedItems {
			if pi.ID == emptyContentItem.ID {
				t.Error("item with empty content should be excluded from summarize input")
			}
		}
	})
}

// ======================================================================
// Stage 4: buildFilterL2Input — verifies L2 filter input JSON structure
// ======================================================================

func TestBuildFilterL2Input_StructureAndAbstract(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	items[0].Content = "Go content"
	items[0].Abstract = "Summary of Go runtime improvements."
	items[1].Content = "K8s content"
	items[1].Abstract = "Kubernetes scaling patterns overview."
	items[3].Content = "Observability content"
	items[3].Abstract = "Cloud native observability practices."
	for _, i := range []int{0, 1, 3} {
		if err := db.Save(&items[i]).Error; err != nil {
			t.Fatalf("failed to save item: %v", err)
		}
	}

	selectedIDs := []uint{items[0].ID, items[1].ID, items[3].ID}
	idsJSON, _ := json.Marshal(selectedIDs)

	task := &models.Task{
		DeviceID:  "dev-test-001",
		Status:    "filtering_l2",
		Level1IDs: string(idsJSON),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	t.Run("includes_device_role_and_preference", func(t *testing.T) {
		input, err := p.buildFilterL2Input(task, db)
		if err != nil {
			t.Fatalf("buildFilterL2Input failed: %v", err)
		}

		inputMap := input.(map[string]any)
		devMap, ok := inputMap["device"].(map[string]string)
		if !ok {
			t.Fatalf("expected device map, got %T", inputMap["device"])
		}
		if devMap["role"] != "software engineer" {
			t.Errorf("expected role=software engineer, got %q", devMap["role"])
		}
		if devMap["preference"] != "Go, distributed systems, cloud native" {
			t.Errorf("expected preference match, got %q", devMap["preference"])
		}
	})

	t.Run("items_have_title_and_abstract", func(t *testing.T) {
		input, err := p.buildFilterL2Input(task, db)
		if err != nil {
			t.Fatalf("buildFilterL2Input failed: %v", err)
		}

		inputMap := input.(map[string]any)
		itemsBytes, _ := json.Marshal(inputMap["items"])
		var parsedItems []struct {
			ID       uint   `json:"id"`
			Title    string `json:"title"`
			Abstract string `json:"abstract"`
		}
		if err := json.Unmarshal(itemsBytes, &parsedItems); err != nil {
			t.Fatalf("failed to unmarshal items: %v", err)
		}

		if len(parsedItems) != 3 {
			t.Fatalf("expected 3 items, got %d", len(parsedItems))
		}

		for _, pi := range parsedItems {
			if pi.Abstract == "" {
				t.Errorf("item %d should have non-empty abstract", pi.ID)
			}
		}
	})

	t.Run("excludes_items_without_abstract", func(t *testing.T) {
		noAbstractItem := models.Item{
			FeedID:   items[0].FeedID,
			Title:    "No Abstract Here",
			URL:      "https://example.com/no-abstract",
			Content:  "Some content",
			Abstract: "",
		}
		db.Create(&noAbstractItem)

		idsWithEmpty := append(selectedIDs, noAbstractItem.ID)
		idsJSON2, _ := json.Marshal(idsWithEmpty)
		task2 := &models.Task{
			DeviceID:  "dev-test-001",
			Status:    "filtering_l2",
			Level1IDs: string(idsJSON2),
		}
		db.Create(task2)

		input, _ := p.buildFilterL2Input(task2, db)
		inputMap := input.(map[string]any)
		itemsBytes, _ := json.Marshal(inputMap["items"])
		var parsedItems []struct {
			ID       uint   `json:"id"`
			Title    string `json:"title"`
			Abstract string `json:"abstract"`
		}
		json.Unmarshal(itemsBytes, &parsedItems)

		for _, pi := range parsedItems {
			if pi.ID == noAbstractItem.ID {
				t.Error("item with empty abstract should be excluded")
			}
		}
	})
}

// ======================================================================
// Stage 5: buildComposeInput — verifies compose input JSON structure
// ======================================================================

func TestBuildComposeInput_StructureAndIDs(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	items[0].Abstract = "Go runtime summary"
	items[1].Abstract = "K8s scaling summary"
	items[3].Abstract = "Observability summary"
	for _, i := range []int{0, 1, 3} {
		db.Save(&items[i])
	}

	selectedIDs := []uint{items[0].ID, items[1].ID, items[3].ID}
	l1JSON, _ := json.Marshal(selectedIDs)
	l2IDs := []uint{items[0].ID, items[3].ID}
	l2JSON, _ := json.Marshal(l2IDs)

	task := &models.Task{
		DeviceID:  "dev-test-001",
		Status:    "composing",
		Level1IDs: string(l1JSON),
		Level2IDs: string(l2JSON),
	}
	db.Create(task)

	t.Run("uses_level2_ids_for_items", func(t *testing.T) {
		input, err := p.buildComposeInput(task, db)
		if err != nil {
			t.Fatalf("buildComposeInput failed: %v", err)
		}

		inputMap := input.(map[string]any)
		itemsBytes, _ := json.Marshal(inputMap["items"])
		var parsedItems []struct {
			ID       uint   `json:"id"`
			Title    string `json:"title"`
			Abstract string `json:"abstract"`
			URL      string `json:"url"`
		}
		if err := json.Unmarshal(itemsBytes, &parsedItems); err != nil {
			t.Fatalf("failed to unmarshal items: %v", err)
		}

		if len(parsedItems) != 2 {
			t.Fatalf("expected 2 items (from level2_ids), got %d", len(parsedItems))
		}

		gotIDs := make([]uint, len(parsedItems))
		for i, pi := range parsedItems {
			gotIDs[i] = pi.ID
			if pi.URL == "" {
				t.Errorf("item %d has empty URL", i)
			}
		}
		sort.Slice(gotIDs, func(i, j int) bool { return gotIDs[i] < gotIDs[j] })

		if gotIDs[0] != items[0].ID || gotIDs[1] != items[3].ID {
			t.Errorf("expected IDs [%d, %d], got %v", items[0].ID, items[3].ID, gotIDs)
		}
	})

	t.Run("includes_device_role_and_preference", func(t *testing.T) {
		input, _ := p.buildComposeInput(task, db)
		inputMap := input.(map[string]any)
		devMap, ok := inputMap["device"].(map[string]string)
		if !ok {
			t.Fatalf("expected device map, got %T", inputMap["device"])
		}
		if devMap["role"] != "software engineer" {
			t.Errorf("expected role=software engineer, got %q", devMap["role"])
		}
	})
}

// ======================================================================
// applyResult stage 1: applyFilterL1Result — writes level1_ids to task
// ======================================================================

func TestApplyResult_FilterL1(t *testing.T) {
	db := setupDB(t)
	p, dataDir := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	task := createTask(t, db, "dev-test-001",
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
	)

	t.Run("writes_level1_ids_to_task", func(t *testing.T) {
		expectedIDs := []uint{items[0].ID, items[1].ID, items[3].ID}

		result := map[string]any{
			"level1_ids": expectedIDs,
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_filter_l1_out.json", task.ID))
		if err := p.runner.WriteJSON(outPath, result); err != nil {
			t.Fatalf("WriteJSON failed: %v", err)
		}

		if err := p.applyFilterL1Result(task, outPath, db); err != nil {
			t.Fatalf("applyFilterL1Result failed: %v", err)
		}

		var updatedTask models.Task
		db.First(&updatedTask, task.ID)

		var level1IDs []uint
		if err := json.Unmarshal([]byte(updatedTask.Level1IDs), &level1IDs); err != nil {
			t.Fatalf("failed to parse level1_ids: %v", err)
		}

		if len(level1IDs) != len(expectedIDs) {
			t.Errorf("expected %d level1_ids, got %d", len(expectedIDs), len(level1IDs))
		}

		sort.Slice(level1IDs, func(i, j int) bool { return level1IDs[i] < level1IDs[j] })
		sort.Slice(expectedIDs, func(i, j int) bool { return expectedIDs[i] < expectedIDs[j] })
		for i := range level1IDs {
			if level1IDs[i] != expectedIDs[i] {
				t.Errorf("level1_ids[%d]: expected %d, got %d", i, expectedIDs[i], level1IDs[i])
			}
		}
	})

	t.Run("empty_result_clears_ids", func(t *testing.T) {
		task2 := createTask(t, db, "dev-test-001",
			time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
		)

		result := map[string]any{
			"level1_ids": []uint{},
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_filter_l1_out.json", task2.ID))
		p.runner.WriteJSON(outPath, result)

		if err := p.applyFilterL1Result(task2, outPath, db); err != nil {
			t.Fatalf("applyFilterL1Result failed: %v", err)
		}

		var updatedTask models.Task
		db.First(&updatedTask, task2.ID)

		var level1IDs []uint
		json.Unmarshal([]byte(updatedTask.Level1IDs), &level1IDs)
		if len(level1IDs) != 0 {
			t.Errorf("expected empty level1_ids, got %v", level1IDs)
		}
	})
}

// ======================================================================
// applyResult stage 2: applyCrawlResult — updates item content
// ======================================================================

func TestApplyResult_Crawl(t *testing.T) {
	db := setupDB(t)
	p, dataDir := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	task := createTask(t, db, "dev-test-001",
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
	)

	t.Run("updates_content_for_successful_crawls", func(t *testing.T) {
		result := map[string]any{
			"results": []map[string]any{
				{"id": float64(items[0].ID), "content": "# Go Runtime\n\nImproved GC.", "success": true},
				{"id": float64(items[1].ID), "content": "# K8s Scaling\n\nHPA patterns.", "success": true},
			},
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_crawl_out.json", task.ID))
		p.runner.WriteJSON(outPath, result)

		if err := p.applyCrawlResult(task, outPath, db); err != nil {
			t.Fatalf("applyCrawlResult failed: %v", err)
		}

		var item0 models.Item
		db.First(&item0, items[0].ID)
		if item0.Content != "# Go Runtime\n\nImproved GC." {
			t.Errorf("item0 content not updated, got %q", item0.Content)
		}

		var item1 models.Item
		db.First(&item1, items[1].ID)
		if item1.Content != "# K8s Scaling\n\nHPA patterns." {
			t.Errorf("item1 content not updated, got %q", item1.Content)
		}
	})

	t.Run("skips_failed_crawls_and_empty_content", func(t *testing.T) {
		task2 := createTask(t, db, "dev-test-001",
			time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
		)

		result := map[string]any{
			"results": []map[string]any{
				{"id": float64(items[2].ID), "content": "", "success": false, "error": "timeout"},
				{"id": float64(items[3].ID), "content": "", "success": true},
			},
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_crawl_out.json", task2.ID))
		p.runner.WriteJSON(outPath, result)

		p.applyCrawlResult(task2, outPath, db)

		var item2 models.Item
		db.First(&item2, items[2].ID)
		if item2.Content != "" {
			t.Errorf("item2 content should not be updated for failed crawl, got %q", item2.Content)
		}

		var item3 models.Item
		db.First(&item3, items[3].ID)
		if item3.Content != "" {
			t.Errorf("item3 content should not be updated for success with empty content, got %q", item3.Content)
		}
	})
}

// ======================================================================
// applyResult stage 3: applySummarizeResult — updates item abstract
// ======================================================================

func TestApplyResult_Summarize(t *testing.T) {
	db := setupDB(t)
	p, dataDir := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	task := createTask(t, db, "dev-test-001",
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
	)

	t.Run("updates_abstract_for_summary_results", func(t *testing.T) {
		result := map[string]any{
			"results": []map[string]any{
				{"id": float64(items[0].ID), "abstract": "Go runtime improved with better GC."},
				{"id": float64(items[1].ID), "abstract": "K8s HPA scaling best practices."},
			},
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_summarize_out.json", task.ID))
		p.runner.WriteJSON(outPath, result)

		if err := p.applySummarizeResult(task, outPath, db); err != nil {
			t.Fatalf("applySummarizeResult failed: %v", err)
		}

		var item0 models.Item
		db.First(&item0, items[0].ID)
		if item0.Abstract != "Go runtime improved with better GC." {
			t.Errorf("item0 abstract not updated, got %q", item0.Abstract)
		}

		var item1 models.Item
		db.First(&item1, items[1].ID)
		if item1.Abstract != "K8s HPA scaling best practices." {
			t.Errorf("item1 abstract not updated, got %q", item1.Abstract)
		}
	})

	t.Run("skips_empty_abstract", func(t *testing.T) {
		task2 := createTask(t, db, "dev-test-001",
			time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
		)

		result := map[string]any{
			"results": []map[string]any{
				{"id": float64(items[2].ID), "abstract": ""},
				{"id": float64(items[3].ID), "abstract": "Cloud observability intro."},
			},
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_summarize_out.json", task2.ID))
		p.runner.WriteJSON(outPath, result)

		p.applySummarizeResult(task2, outPath, db)

		var item2 models.Item
		db.First(&item2, items[2].ID)
		if item2.Abstract != "" {
			t.Errorf("item2 abstract should stay empty, got %q", item2.Abstract)
		}

		var item3 models.Item
		db.First(&item3, items[3].ID)
		if item3.Abstract != "Cloud observability intro." {
			t.Errorf("item3 abstract not updated, got %q", item3.Abstract)
		}
	})
}

// ======================================================================
// applyResult stage 4: applyFilterL2Result — writes level2_ids to task
// ======================================================================

func TestApplyResult_FilterL2(t *testing.T) {
	db := setupDB(t)
	p, dataDir := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	_, items := createSeedData(t, db)

	task := createTask(t, db, "dev-test-001",
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
	)

	t.Run("writes_level2_ids_to_task", func(t *testing.T) {
		expectedIDs := []uint{items[0].ID, items[3].ID}

		result := map[string]any{
			"level2_ids": expectedIDs,
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_filter_l2_out.json", task.ID))
		p.runner.WriteJSON(outPath, result)

		if err := p.applyFilterL2Result(task, outPath, db); err != nil {
			t.Fatalf("applyFilterL2Result failed: %v", err)
		}

		var updatedTask models.Task
		db.First(&updatedTask, task.ID)

		var level2IDs []uint
		json.Unmarshal([]byte(updatedTask.Level2IDs), &level2IDs)
		if len(level2IDs) != 2 {
			t.Fatalf("expected 2 level2_ids, got %d", len(level2IDs))
		}

		sort.Slice(level2IDs, func(i, j int) bool { return level2IDs[i] < level2IDs[j] })
		if level2IDs[0] != items[0].ID || level2IDs[1] != items[3].ID {
			t.Errorf("expected IDs [%d, %d], got %v", items[0].ID, items[3].ID, level2IDs)
		}
	})
}

// ======================================================================
// applyResult stage 5: applyComposeResult — writes report to task
// ======================================================================

func TestApplyResult_Compose(t *testing.T) {
	db := setupDB(t)
	p, dataDir := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	task := createTask(t, db, "dev-test-001",
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
	)

	t.Run("writes_report_to_task", func(t *testing.T) {
		report := "# Reading Report\n\n- [Go 1.24](https://example.com/go): Improved GC\n- [Observability](https://example.com/obs): OTel practices"
		result := map[string]any{
			"report": report,
		}
		outPath := filepath.Join(dataDir, fmt.Sprintf("%d_compose_out.json", task.ID))
		p.runner.WriteJSON(outPath, result)

		if err := p.applyComposeResult(task, outPath, db); err != nil {
			t.Fatalf("applyComposeResult failed: %v", err)
		}

		var updatedTask models.Task
		db.First(&updatedTask, task.ID)

		if updatedTask.Report != report {
			t.Errorf("expected report %q, got %q", report, updatedTask.Report)
		}
	})
}

// ======================================================================
// Helper: getItemsInRange — time range filtering
// ======================================================================

func TestGetItemsInRange(t *testing.T) {
	db := setupDB(t)
	_, _ = newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	feed := models.Feed{Name: "Test", URL: "https://example.com/rss", Enabled: true}
	db.Create(&feed)

	testStart := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	testEnd := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)

	inRange := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	beforeRange := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	publishedInRange := models.Item{
		FeedID: feed.ID, Title: "In Range",
		URL: "https://example.com/in-range", PublishedAt: &inRange,
	}
	publishedBefore := models.Item{
		FeedID: feed.ID, Title: "Before Range",
		URL: "https://example.com/before-range", PublishedAt: &beforeRange,
	}
	noPublishedButCreatedInRange := models.Item{
		FeedID: feed.ID, Title: "No PublishedAt but created in range",
		URL:         "https://example.com/no-pub",
		PublishedAt: nil,
		CreatedAt:   testStart.Add(6 * time.Hour),
	}
	noPublishedCreatedLongAgo := models.Item{
		FeedID: feed.ID, Title: "No PublishedAt created long ago",
		URL:         "https://example.com/no-pub-old",
		PublishedAt: nil,
		CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	db.Create(&publishedInRange)
	db.Create(&publishedBefore)
	db.Create(&noPublishedButCreatedInRange)
	db.Create(&noPublishedCreatedLongAgo)

	items, err := getItemsInRange(db, testStart, testEnd)
	if err != nil {
		t.Fatalf("getItemsInRange failed: %v", err)
	}

	foundInRange := false
	foundBefore := false
	foundNoPubInRange := false
	foundNoPubOld := false
	for _, item := range items {
		if item.ID == publishedInRange.ID {
			foundInRange = true
		}
		if item.ID == publishedBefore.ID {
			foundBefore = true
		}
		if item.ID == noPublishedButCreatedInRange.ID {
			foundNoPubInRange = true
		}
		if item.ID == noPublishedCreatedLongAgo.ID {
			foundNoPubOld = true
		}
	}

	if !foundInRange {
		t.Error("item with published_at in range should be included")
	}
	if foundBefore {
		t.Error("item with published_at outside range should be excluded")
	}
	if !foundNoPubInRange {
		t.Error("item with nil published_at and created_at in range should be included")
	}
	if foundNoPubOld {
		t.Error("item with nil published_at and created_at outside range should be excluded")
	}
}

// ======================================================================
// Helper: ensureDevice — auto-creation
// ======================================================================

func TestEnsureDevice(t *testing.T) {
	db := setupDB(t)
	_, _ = newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	t.Run("creates_new_device", func(t *testing.T) {
		device, err := ensureDevice(db, "brand-new-device")
		if err != nil {
			t.Fatalf("ensureDevice failed: %v", err)
		}
		if device.DeviceID != "brand-new-device" {
			t.Errorf("expected device_id=brand-new-device, got %s", device.DeviceID)
		}

		var count int64
		db.Model(&models.Device{}).Where("device_id = ?", "brand-new-device").Count(&count)
		if count != 1 {
			t.Errorf("expected 1 device, got %d", count)
		}
	})

	t.Run("returns_existing_device", func(t *testing.T) {
		existing := models.Device{
			DeviceID:  "existing-device",
			Role:      "admin",
			CreatedAt: time.Now(),
			LastSeen:  time.Now(),
		}
		db.Create(&existing)

		device, err := ensureDevice(db, "existing-device")
		if err != nil {
			t.Fatalf("ensureDevice failed: %v", err)
		}
		if device.Role != "admin" {
			t.Errorf("expected role=admin for existing device, got %q", device.Role)
		}

		var count int64
		db.Model(&models.Device{}).Where("device_id = ?", "existing-device").Count(&count)
		if count != 1 {
			t.Errorf("should not create duplicate, expected 1 device, got %d", count)
		}
	})
}

// ======================================================================
// Full pipeline end-to-end with mock Python script
// ======================================================================

func TestFullPipelineEndToEnd(t *testing.T) {
	db := setupDB(t)

	mockScript := filepath.Join(t.TempDir(), "mock_pipeline.sh")
	scriptContent := `#!/bin/bash
# Mock pipeline script - produces predictable outputs for each mode
MODE=""
INPUT=""
OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2 ;;
    --input) INPUT="$2"; shift 2 ;;
    --output) OUTPUT="$2"; shift 2 ;;
    *) shift ;;
  esac
done

case "$MODE" in
  filter_l1)
    python3 -c "
import json, sys
data = json.load(open('$INPUT'))
ids = [item['id'] for item in data['items']]
print(json.dumps({'level1_ids': ids}))
" > "$OUTPUT"
    ;;
  crawl)
    python3 -c "
import json, sys
data = json.load(open('$INPUT'))
results = []
for item in data['items']:
    results.append({'id': item['id'], 'content': 'Crawled content for ' + str(item['id']), 'success': True})
print(json.dumps({'results': results}))
" > "$OUTPUT"
    ;;
  summarize)
    python3 -c "
import json, sys
data = json.load(open('$INPUT'))
results = []
for item in data['items']:
    results.append({'id': item['id'], 'abstract': 'Summary of item ' + str(item['id'])})
print(json.dumps({'results': results}))
" > "$OUTPUT"
    ;;
  filter_l2)
    python3 -c "
import json, sys
data = json.load(open('$INPUT'))
ids = [item['id'] for item in data['items']]
print(json.dumps({'level2_ids': ids}))
" > "$OUTPUT"
    ;;
  compose)
    echo '{"report": "# Reading Report\n\nMock report content."}' > "$OUTPUT"
    ;;
  *)
    echo '{"error": "unknown mode"}' > "$OUTPUT"
    ;;
esac
`
	if err := os.WriteFile(mockScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}

	p, dataDir := newPipeline(t, "/bin/bash", mockScript)
	defer resetRunning()

	device, items := createSeedData(t, db)
	_ = items

	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)

	task, err := p.StartPipeline(device.DeviceID, start, end)
	if err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	time.Sleep(5 * time.Second)

	var finalTask models.Task
	if err := db.First(&finalTask, task.ID).Error; err != nil {
		t.Fatalf("failed to load task: %v", err)
	}

	t.Run("task_completes_successfully", func(t *testing.T) {
		if finalTask.Status == "failed" {
			t.Fatalf("task failed: %s", finalTask.Error)
		}
		if finalTask.Status != "completed" {
			t.Errorf("expected status=completed, got %s", finalTask.Status)
		}
	})

	t.Run("level1_ids_populated", func(t *testing.T) {
		if finalTask.Level1IDs == "" {
			t.Error("level1_ids should not be empty after pipeline completes")
		}
		var level1IDs []uint
		if err := json.Unmarshal([]byte(finalTask.Level1IDs), &level1IDs); err != nil {
			t.Fatalf("failed to parse level1_ids: %v", err)
		}
		if len(level1IDs) == 0 {
			t.Error("level1_ids should have at least 1 item")
		}
	})

	t.Run("items_have_content_after_crawl", func(t *testing.T) {
		var itemsWithContent []models.Item
		db.Where("content != ''").Find(&itemsWithContent)
		if len(itemsWithContent) == 0 {
			t.Error("expected some items to have content after crawl stage")
		}
		for _, item := range itemsWithContent {
			if item.Content == "" {
				t.Errorf("item %d should have non-empty content", item.ID)
			}
		}
	})

	t.Run("items_have_abstract_after_summarize", func(t *testing.T) {
		var itemsWithAbstract []models.Item
		db.Where("abstract != ''").Find(&itemsWithAbstract)
		if len(itemsWithAbstract) == 0 {
			t.Error("expected some items to have abstract after summarize stage")
		}
		for _, item := range itemsWithAbstract {
			if item.Abstract == "" {
				t.Errorf("item %d should have non-empty abstract", item.ID)
			}
		}
	})

	t.Run("level2_ids_populated", func(t *testing.T) {
		if finalTask.Level2IDs == "" {
			t.Error("level2_ids should not be empty after pipeline completes")
		}
		var level2IDs []uint
		if err := json.Unmarshal([]byte(finalTask.Level2IDs), &level2IDs); err != nil {
			t.Fatalf("failed to parse level2_ids: %v", err)
		}
		if len(level2IDs) == 0 {
			t.Error("level2_ids should have at least 1 item")
		}
	})

	t.Run("report_populated", func(t *testing.T) {
		if finalTask.Report == "" {
			t.Error("report should not be empty after pipeline completes")
		}
		if finalTask.Report != "# Reading Report\n\nMock report content." {
			t.Errorf("unexpected report content: %q", finalTask.Report)
		}
	})

	t.Run("completed_at_set", func(t *testing.T) {
		if finalTask.CompletedAt == nil {
			t.Error("completed_at should be set after pipeline completes")
		}
	})

	t.Run("intermediate_files_preserved", func(t *testing.T) {
		stages := []string{"filter_l1", "crawl", "summarize", "filter_l2", "compose"}
		for _, stage := range stages {
			inPath := fmt.Sprintf("%s/%d_%s_in.json", dataDir, task.ID, stage)
			outPath := fmt.Sprintf("%s/%d_%s_out.json", dataDir, task.ID, stage)

			if _, err := os.Stat(inPath); os.IsNotExist(err) {
				t.Errorf("input file for %s should be preserved: %s", stage, inPath)
			}
			if _, err := os.Stat(outPath); os.IsNotExist(err) {
				t.Errorf("output file for %s should be preserved: %s", stage, outPath)
			}
		}
	})

	t.Run("input_output_content_verified", func(t *testing.T) {
		for _, stage := range []string{"filter_l1", "crawl", "summarize", "filter_l2", "compose"} {
			inPath := fmt.Sprintf("%s/%d_%s_in.json", dataDir, task.ID, stage)
			outPath := fmt.Sprintf("%s/%d_%s_out.json", dataDir, task.ID, stage)

			inData, err := os.ReadFile(inPath)
			if err != nil {
				t.Errorf("failed to read input for %s: %v", stage, err)
				continue
			}
			if len(inData) == 0 {
				t.Errorf("input for %s is empty", stage)
			}

			outData, err := os.ReadFile(outPath)
			if err != nil {
				t.Errorf("failed to read output for %s: %v", stage, err)
				continue
			}
			if len(outData) == 0 {
				t.Errorf("output for %s is empty", stage)
			}
		}
	})

	t.Run("filter_l1_input_contains_device_and_items", func(t *testing.T) {
		inPath := fmt.Sprintf("%s/%d_filter_l1_in.json", dataDir, task.ID)
		inData, err := os.ReadFile(inPath)
		if err != nil {
			t.Fatalf("failed to read filter_l1 input: %v", err)
		}

		var input map[string]any
		if err := json.Unmarshal(inData, &input); err != nil {
			t.Fatalf("failed to parse filter_l1 input: %v", err)
		}

		dev, ok := input["device"].(map[string]any)
		if !ok {
			t.Fatal("filter_l1 input missing 'device' key")
		}
		if dev["role"] != "software engineer" {
			t.Errorf("expected role=software engineer, got %v", dev["role"])
		}

		itemsAny, ok := input["items"].([]any)
		if !ok {
			t.Fatal("filter_l1 input missing 'items' key")
		}
		if len(itemsAny) == 0 {
			t.Error("filter_l1 input should have items")
		}
	})

	t.Run("crawl_input_contains_urls", func(t *testing.T) {
		inPath := fmt.Sprintf("%s/%d_crawl_in.json", dataDir, task.ID)
		inData, err := os.ReadFile(inPath)
		if err != nil {
			t.Fatalf("failed to read crawl input: %v", err)
		}

		var input map[string]any
		if err := json.Unmarshal(inData, &input); err != nil {
			t.Fatalf("failed to parse crawl input: %v", err)
		}

		itemsAny, ok := input["items"].([]any)
		if !ok {
			t.Fatal("crawl input missing 'items' key")
		}
		for _, itemAny := range itemsAny {
			item, ok := itemAny.(map[string]any)
			if !ok {
				t.Fatal("crawl input item is not a map")
			}
			if item["url"] == nil || item["url"] == "" {
				t.Error("crawl input item should have non-empty 'url'")
			}
		}
	})
}

func TestPipelineStatusTransitions(t *testing.T) {
	db := setupDB(t)

	mockScript := filepath.Join(t.TempDir(), "mock_pipeline_status.sh")
	scriptContent := `#!/bin/bash
MODE=""
INPUT=""
OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2 ;;
    --input) INPUT="$2"; shift 2 ;;
    --output) OUTPUT="$2"; shift 2 ;;
    *) shift ;;
  esac
done

case "$MODE" in
  filter_l1) echo '{"level1_ids": []}' > "$OUTPUT" ;;
  crawl) echo '{"results": []}' > "$OUTPUT" ;;
  summarize) echo '{"results": []}' > "$OUTPUT" ;;
  filter_l2) echo '{"level2_ids": []}' > "$OUTPUT" ;;
  compose) echo '{"report": "empty report"}' > "$OUTPUT" ;;
esac
`
	os.WriteFile(mockScript, []byte(scriptContent), 0755)

	p, _ := newPipeline(t, "/bin/bash", mockScript)
	defer resetRunning()

	device, _ := createSeedData(t, db)

	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)

	task, err := p.StartPipeline(device.DeviceID, start, end)
	if err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	time.Sleep(3 * time.Second)

	var finalTask models.Task
	db.First(&finalTask, task.ID)

	t.Run("final_status_is_completed", func(t *testing.T) {
		if finalTask.Status == "failed" {
			t.Fatalf("task failed: %s", finalTask.Error)
		}
		if finalTask.Status != "completed" {
			t.Errorf("expected status=completed, got %s", finalTask.Status)
		}
	})

	t.Run("task_can_be_restarted_after_completion", func(t *testing.T) {
		_, err := p.StartPipeline(device.DeviceID, start, end)
		if err != nil {
			t.Errorf("should be able to start new task after previous completes: %v", err)
		}
	})

	resetRunning()
}

func TestPipelineErrorHandling(t *testing.T) {
	db := setupDB(t)

	mockScript := filepath.Join(t.TempDir(), "mock_pipeline_fail.sh")
	scriptContent := `#!/bin/bash
MODE=""
INPUT=""
OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="$2"; shift 2 ;;
    --input) INPUT="$2"; shift 2 ;;
    --output) OUTPUT="$2"; shift 2 ;;
    *) shift ;;
  esac
done

if [ "$MODE" = "filter_l1" ]; then
  echo "Intentional failure" >&2
  exit 1
fi
echo '{"level1_ids": []}' > "$OUTPUT"
`
	os.WriteFile(mockScript, []byte(scriptContent), 0755)

	p, _ := newPipeline(t, "/bin/bash", mockScript)
	defer resetRunning()

	device, _ := createSeedData(t, db)

	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)

	task, err := p.StartPipeline(device.DeviceID, start, end)
	if err != nil {
		t.Fatalf("StartPipeline failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	var finalTask models.Task
	db.First(&finalTask, task.ID)

	if finalTask.Status != "failed" {
		t.Errorf("expected status=failed, got %s", finalTask.Status)
	}
	if finalTask.Error == "" {
		t.Error("expected error message to be set on failed task")
	}
}

func TestConcurrencyGuard(t *testing.T) {
	db := setupDB(t)
	p, _ := newPipeline(t, "python3", "py/pipeline.py")
	defer resetRunning()

	device := models.Device{
		DeviceID:  "concurrent-dev",
		Role:      "tester",
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}
	db.Create(&device)

	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)

	t.Run("second_pipeline_returns_error_while_first_runs", func(t *testing.T) {
		runningMu.Lock()
		fakeID := uint(999)
		runningTaskID = &fakeID
		runningMu.Unlock()

		_, err := p.StartPipeline(device.DeviceID, start, end)
		if err != ErrTaskRunning {
			t.Errorf("expected ErrTaskRunning, got %v", err)
		}

		resetRunning()
	})

	t.Run("can_start_after_previous_completes", func(t *testing.T) {
		resetRunning()

		task, err := p.StartPipeline(device.DeviceID, start, end)
		if err != nil {
			t.Fatalf("should be able to start pipeline: %v", err)
		}

		runningMu.Lock()
		runningTaskID = nil
		runningMu.Unlock()

		_, err = p.StartPipeline(device.DeviceID, start, end)
		if err == ErrTaskRunning {
			t.Error("should not get ErrTaskRunning after previous task cleared")
		}

		_ = task
	})
}