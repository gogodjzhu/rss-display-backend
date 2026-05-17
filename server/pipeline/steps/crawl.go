package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

// CrawlStep fetches the article content for each Level-1 item and persists it
// to the database. It writes the list of attempted IDs to state under "crawl".
type CrawlStep struct {
	db                  *gorm.DB
	runner              *pipeline.PythonRunner
	rateLimitMinSeconds int
	rateLimitMaxSeconds int
}

// NewCrawlStep constructs a CrawlStep.
func NewCrawlStep(db *gorm.DB, runner *pipeline.PythonRunner, rateLimitMin, rateLimitMax int) *CrawlStep {
	return &CrawlStep{
		db:                  db,
		runner:              runner,
		rateLimitMinSeconds: rateLimitMin,
		rateLimitMaxSeconds: rateLimitMax,
	}
}

func (s *CrawlStep) Name() string { return "crawl" }

func (s *CrawlStep) Config() pipeline.StepConfig {
	return pipeline.StepConfig{
		Timeout:     30 * time.Minute,
		RetryPolicy: pipeline.RetryPolicy{MaxAttempts: 2, BaseDelay: 10 * time.Second, MaxDelay: 60 * time.Second},
	}
}

func (s *CrawlStep) Run(ctx context.Context, state pipeline.StateAccessor) error {
	l1, err := pipeline.GetState[FilterL1Output](state, "filter_l1")
	if err != nil {
		return err
	}

	if len(l1.Level1IDs) == 0 {
		return pipeline.SetState(state, s.Name(), CrawlOutput{CrawledIDs: nil})
	}

	var items []models.Item
	if err := s.db.Where("id IN ?", l1.Level1IDs).Find(&items).Error; err != nil {
		return fmt.Errorf("crawl: get items: %w", err)
	}

	type itemEntry struct {
		ID  uint   `json:"id"`
		URL string `json:"url"`
	}
	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, URL: item.URL}
	}

	pyInput := map[string]any{
		"rate_limit_min_seconds": s.rateLimitMinSeconds,
		"rate_limit_max_seconds": s.rateLimitMaxSeconds,
		"items":                  entries,
	}

	inPath, outPath, cleanup := tempIOPaths("crawl")
	defer cleanup()

	if err := s.runner.WriteJSON(inPath, pyInput); err != nil {
		return fmt.Errorf("crawl: write input: %w", err)
	}
	if err := s.runner.RunCtx(ctx, "crawl", inPath, outPath); err != nil {
		return fmt.Errorf("crawl: python: %w", err)
	}

	var pyOutput struct {
		Results []struct {
			ID      uint   `json:"id"`
			Content string `json:"content"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
		} `json:"results"`
	}
	if err := s.runner.ReadJSON(outPath, &pyOutput); err != nil {
		return fmt.Errorf("crawl: read output: %w", err)
	}

	var crawledIDs []uint
	for _, r := range pyOutput.Results {
		if r.Success && r.Content != "" {
			if err := s.db.Model(&models.Item{}).Where("id = ?", r.ID).
				Update("content", r.Content).Error; err != nil {
				return fmt.Errorf("crawl: update item %d: %w", r.ID, err)
			}
		}
		crawledIDs = append(crawledIDs, r.ID)
	}

	return pipeline.SetState(state, s.Name(), CrawlOutput{CrawledIDs: crawledIDs})
}
