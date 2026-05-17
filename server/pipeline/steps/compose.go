package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"gorm.io/gorm"
)

// ComposeStep builds the final personalised report from Level-2 items.
// It writes the report and Level-2 IDs back to the models.Job row so
// the HTTP API can return them, and also stores the output in state under "compose".
type ComposeStep struct {
	db     *gorm.DB
	runner *pipeline.PythonRunner
}

// NewComposeStep constructs a ComposeStep.
func NewComposeStep(db *gorm.DB, runner *pipeline.PythonRunner) *ComposeStep {
	return &ComposeStep{db: db, runner: runner}
}

func (s *ComposeStep) Name() string { return "compose" }

func (s *ComposeStep) Config() pipeline.StepConfig {
	return pipeline.StepConfig{
		Timeout:     10 * time.Minute,
		RetryPolicy: pipeline.RetryPolicy{MaxAttempts: 2, BaseDelay: 5 * time.Second, MaxDelay: 30 * time.Second},
	}
}

func (s *ComposeStep) Run(ctx context.Context, state pipeline.StateAccessor) error {
	input, err := pipeline.GetState[RSSJobInput](state, "job_input")
	if err != nil {
		return err
	}

	l2, err := pipeline.GetState[FilterL2Output](state, "filter_l2")
	if err != nil {
		return err
	}

	// Retrieve the job ID that the Runner stored in state.
	jobID, err := pipeline.GetState[uint](state, "job_id")
	if err != nil {
		return err
	}

	device, err := getDevice(s.db, input.DeviceID)
	if err != nil {
		return fmt.Errorf("compose: get device: %w", err)
	}

	var items []models.Item
	if len(l2.Level2IDs) > 0 {
		if err := s.db.Where("id IN ?", l2.Level2IDs).Find(&items).Error; err != nil {
			return fmt.Errorf("compose: get items: %w", err)
		}
	}

	type itemEntry struct {
		ID       uint   `json:"id"`
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
		URL      string `json:"url"`
	}
	entries := make([]itemEntry, len(items))
	for i, item := range items {
		entries[i] = itemEntry{ID: item.ID, Title: item.Title, Abstract: item.Abstract, URL: item.URL}
	}

	pyInput := map[string]any{
		"device": map[string]string{
			"role":       device.Role,
			"preference": device.Preference,
		},
		"items": entries,
	}

	inPath, outPath, cleanup := tempIOPaths("compose")
	defer cleanup()

	if err := s.runner.WriteJSON(inPath, pyInput); err != nil {
		return fmt.Errorf("compose: write input: %w", err)
	}
	if err := s.runner.RunCtx(ctx, "compose", inPath, outPath); err != nil {
		return fmt.Errorf("compose: python: %w", err)
	}

	var pyOutput struct {
		Report string `json:"report"`
	}
	if err := s.runner.ReadJSON(outPath, &pyOutput); err != nil {
		return fmt.Errorf("compose: read output: %w", err)
	}

	// Persist report and level2_ids back to the Job row for API consumers.
	level2IDsJSON := marshalIDs(l2.Level2IDs)
	if err := s.db.Model(&models.Job{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"report":     pyOutput.Report,
			"level2_ids": level2IDsJSON,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return fmt.Errorf("compose: persist to job: %w", err)
	}

	return pipeline.SetState(state, s.Name(), ComposeOutput{Report: pyOutput.Report})
}

func marshalIDs(ids []uint) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		b, _ := json.Marshal(id)
		parts[i] = string(b)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
