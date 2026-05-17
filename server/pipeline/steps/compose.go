package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/esp32-rss-display/backend/server/pipeline"
)

// ComposeStep builds the final personalised report from Level-2 items.
// It writes the report and Level-2 IDs back to the models.Job row so
// the HTTP API can return them, and also stores the output in state under "compose".
type ComposeStep struct {
	devices DeviceGetter
	items   ItemFinder
	jobs    JobReporter
	runner  *pipeline.PythonRunner
}

// NewComposeStep constructs a ComposeStep.
func NewComposeStep(devices DeviceGetter, items ItemFinder, jobs JobReporter, runner *pipeline.PythonRunner) *ComposeStep {
	return &ComposeStep{devices: devices, items: items, jobs: jobs, runner: runner}
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

	jobID, err := pipeline.GetState[uint](state, "job_id")
	if err != nil {
		return err
	}

	device, err := s.devices.GetOrCreate(ctx, input.DeviceID)
	if err != nil {
		return fmt.Errorf("compose: get device: %w", err)
	}

	var itemList []struct {
		ID       uint
		Title    string
		Abstract string
		URL      string
	}
	if len(l2.Level2IDs) > 0 {
		items, err := s.items.FindByIDs(ctx, l2.Level2IDs)
		if err != nil {
			return fmt.Errorf("compose: get items: %w", err)
		}
		for _, item := range items {
			itemList = append(itemList, struct {
				ID       uint
				Title    string
				Abstract string
				URL      string
			}{ID: item.ID, Title: item.Title, Abstract: item.Abstract, URL: item.URL})
		}
	}

	type itemEntry struct {
		ID       uint   `json:"id"`
		Title    string `json:"title"`
		Abstract string `json:"abstract"`
		URL      string `json:"url"`
	}
	entries := make([]itemEntry, len(itemList))
	for i, item := range itemList {
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

	if err := s.jobs.UpdateReport(ctx, jobID, pyOutput.Report, l2.Level2IDs); err != nil {
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