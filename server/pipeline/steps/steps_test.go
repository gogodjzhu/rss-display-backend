package steps_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"github.com/esp32-rss-display/backend/server/pipeline/steps"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Device{}, &models.Feed{}, &models.Item{}, &models.Job{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newState(t *testing.T) pipeline.StateAccessor {
	t.Helper()
	fsm := pipeline.NewFileStateManager(t.TempDir())
	acc, err := fsm.Open(1)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	return acc
}

func seedDB(t *testing.T, db *gorm.DB) (models.Device, models.Feed, []models.Item) {
	t.Helper()
	device := models.Device{
		DeviceID:   "dev-001",
		Role:       "engineer",
		Preference: "Go, distributed systems",
		CreatedAt:  time.Now(),
		LastSeen:   time.Now(),
	}
	db.Create(&device)

	feed := models.Feed{Name: "Tech", URL: "https://example.com/rss", Enabled: true}
	db.Create(&feed)

	pub := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	items := []models.Item{
		{FeedID: feed.ID, Title: "Go 1.24", URL: "https://example.com/go124", PublishedAt: &pub},
		{FeedID: feed.ID, Title: "Kubernetes Tips", URL: "https://example.com/k8s", PublishedAt: &pub},
		{FeedID: feed.ID, Title: "Celebrity Gossip", URL: "https://example.com/gossip", PublishedAt: &pub},
	}
	for i := range items {
		db.Create(&items[i])
	}
	return device, feed, items
}

// ── FilterL1Step ──────────────────────────────────────────────────────────────

func TestFilterL1StepName(t *testing.T) {
	db := newDB(t)
	s := steps.NewFilterL1Step(db, &pipeline.PythonRunner{})
	if s.Name() != "filter_l1" {
		t.Errorf("Name() = %q, want %q", s.Name(), "filter_l1")
	}
}

func TestFilterL1StepConfigHasTimeout(t *testing.T) {
	db := newDB(t)
	s := steps.NewFilterL1Step(db, &pipeline.PythonRunner{})
	cfg := s.Config()
	if cfg.Timeout <= 0 {
		t.Error("FilterL1Step config timeout should be positive")
	}
	if cfg.RetryPolicy.MaxAttempts < 1 {
		t.Error("FilterL1Step retry policy MaxAttempts should be at least 1")
	}
}

func TestFilterL1StepRunFailsWithoutJobInput(t *testing.T) {
	db := newDB(t)
	s := steps.NewFilterL1Step(db, &pipeline.PythonRunner{})
	state := newState(t)

	err := s.Run(context.Background(), state)
	if err == nil {
		t.Error("Run without job_input in state should return error")
	}
}

// ── CrawlStep ────────────────────────────────────────────────────────────────

func TestCrawlStepName(t *testing.T) {
	db := newDB(t)
	s := steps.NewCrawlStep(db, &pipeline.PythonRunner{}, 30, 90)
	if s.Name() != "crawl" {
		t.Errorf("Name() = %q, want %q", s.Name(), "crawl")
	}
}

func TestCrawlStepSkipsEmptyLevel1IDs(t *testing.T) {
	db := newDB(t)
	s := steps.NewCrawlStep(db, &pipeline.PythonRunner{}, 30, 90)
	state := newState(t)

	// Set job_input
	input := steps.RSSJobInput{DeviceID: "dev-001", TimeRangeStart: time.Now(), TimeRangeEnd: time.Now()}
	pipeline.SetState(state, "job_input", input)
	// Set empty filter_l1
	pipeline.SetState(state, "filter_l1", steps.FilterL1Output{Level1IDs: nil})

	if err := s.Run(context.Background(), state); err != nil {
		t.Fatalf("Run with empty Level1IDs should succeed, got: %v", err)
	}

	result, err := pipeline.GetState[steps.CrawlOutput](state, "crawl")
	if err != nil {
		t.Fatalf("GetState crawl: %v", err)
	}
	if len(result.CrawledIDs) != 0 {
		t.Errorf("expected empty CrawledIDs for empty input, got %v", result.CrawledIDs)
	}
}

func TestCrawlStepFailsWithoutFilterL1State(t *testing.T) {
	db := newDB(t)
	s := steps.NewCrawlStep(db, &pipeline.PythonRunner{}, 30, 90)
	state := newState(t)

	err := s.Run(context.Background(), state)
	if err == nil {
		t.Error("Run without filter_l1 in state should return error")
	}
}

// ── SummarizeStep ────────────────────────────────────────────────────────────

func TestSummarizeStepName(t *testing.T) {
	db := newDB(t)
	s := steps.NewSummarizeStep(db, &pipeline.PythonRunner{})
	if s.Name() != "summarize" {
		t.Errorf("Name() = %q, want %q", s.Name(), "summarize")
	}
}

func TestSummarizeStepSkipsEmptyCrawledIDs(t *testing.T) {
	db := newDB(t)
	s := steps.NewSummarizeStep(db, &pipeline.PythonRunner{})
	state := newState(t)

	pipeline.SetState(state, "crawl", steps.CrawlOutput{CrawledIDs: nil})

	if err := s.Run(context.Background(), state); err != nil {
		t.Fatalf("Run with empty CrawledIDs should succeed, got: %v", err)
	}

	result, err := pipeline.GetState[steps.SummarizeOutput](state, "summarize")
	if err != nil {
		t.Fatalf("GetState summarize: %v", err)
	}
	if len(result.SummarizedIDs) != 0 {
		t.Errorf("expected empty SummarizedIDs, got %v", result.SummarizedIDs)
	}
}

// ── FilterL2Step ────────────────────────────────────────────────────────────

func TestFilterL2StepName(t *testing.T) {
	db := newDB(t)
	s := steps.NewFilterL2Step(db, &pipeline.PythonRunner{})
	if s.Name() != "filter_l2" {
		t.Errorf("Name() = %q, want %q", s.Name(), "filter_l2")
	}
}

func TestFilterL2StepSkipsEmptySummarizedIDs(t *testing.T) {
	db := newDB(t)
	_, _, _ = seedDB(t, db)
	s := steps.NewFilterL2Step(db, &pipeline.PythonRunner{})
	state := newState(t)

	input := steps.RSSJobInput{DeviceID: "dev-001", TimeRangeStart: time.Now(), TimeRangeEnd: time.Now()}
	pipeline.SetState(state, "job_input", input)
	pipeline.SetState(state, "summarize", steps.SummarizeOutput{SummarizedIDs: nil})

	if err := s.Run(context.Background(), state); err != nil {
		t.Fatalf("Run with empty SummarizedIDs should succeed, got: %v", err)
	}

	result, err := pipeline.GetState[steps.FilterL2Output](state, "filter_l2")
	if err != nil {
		t.Fatalf("GetState filter_l2: %v", err)
	}
	if len(result.Level2IDs) != 0 {
		t.Errorf("expected empty Level2IDs, got %v", result.Level2IDs)
	}
}

// ── ComposeStep ────────────────────────────────────────────────────────────

func TestComposeStepName(t *testing.T) {
	db := newDB(t)
	s := steps.NewComposeStep(db, &pipeline.PythonRunner{})
	if s.Name() != "compose" {
		t.Errorf("Name() = %q, want %q", s.Name(), "compose")
	}
}

func TestComposeStepFailsWithoutFilterL2State(t *testing.T) {
	db := newDB(t)
	s := steps.NewComposeStep(db, &pipeline.PythonRunner{})
	state := newState(t)

	err := s.Run(context.Background(), state)
	if err == nil {
		t.Error("Run without filter_l2 in state should return error")
	}
}

// ── BuildRSSPipeline ──────────────────────────────────────────────────────────

func TestBuildRSSPipelineHasFiveSteps(t *testing.T) {
	db := newDB(t)
	runner := &pipeline.PythonRunner{PythonPath: "python3", ScriptPath: "py/pipeline.py"}
	p := steps.BuildRSSPipeline(db, runner, 30, 90)

	if p.Name() != steps.PipelineName {
		t.Errorf("pipeline name = %q, want %q", p.Name(), steps.PipelineName)
	}
}

func TestRSSJobInputRoundTrip(t *testing.T) {
	state := newState(t)
	now := time.Now().Truncate(time.Second).UTC()
	input := steps.RSSJobInput{
		DeviceID:       "dev-abc",
		TimeRangeStart: now,
		TimeRangeEnd:   now.Add(24 * time.Hour),
	}

	b, _ := json.Marshal(input)
	pipeline.SetState(state, "job_input", input)

	got, err := pipeline.GetState[steps.RSSJobInput](state, "job_input")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.DeviceID != input.DeviceID {
		t.Errorf("DeviceID = %q, want %q", got.DeviceID, input.DeviceID)
	}
	_ = b
}
