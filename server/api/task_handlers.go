package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/esp32-rss-display/backend/server/domain/devices"
	"github.com/esp32-rss-display/backend/server/domain/jobs"
	"github.com/esp32-rss-display/backend/server/domain/items"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"github.com/esp32-rss-display/backend/server/pipeline/steps"
)

var defaultRunner *pipeline.Runner

func InitRunner(r *pipeline.Runner) {
	defaultRunner = r
}

type TaskHandler struct {
	deviceSvc devices.Service
	jobSvc    jobs.Service
}

func NewTaskHandler(deviceSvc devices.Service, jobSvc jobs.Service) *TaskHandler {
	return &TaskHandler{
		deviceSvc: deviceSvc,
		jobSvc:    jobSvc,
	}
}

type PreferenceRequest struct {
	Role       string `json:"role"`
	Preference string `json:"preference"`
}

type PreferenceResponse struct {
	DeviceID   string `json:"device_id"`
	Role       string `json:"role"`
	Preference string `json:"preference"`
}

type CreateJobRequest struct {
	TimeRangeStart string `json:"time_range_start"`
	TimeRangeEnd   string `json:"time_range_end"`
}

type JobResponse struct {
	ID             uint    `json:"id"`
	DeviceID       string  `json:"device_id"`
	Status         string  `json:"status"`
	CurrentStep    int     `json:"current_step,omitempty"`
	TimeRangeStart *string `json:"time_range_start,omitempty"`
	TimeRangeEnd   *string `json:"time_range_end,omitempty"`
	Level2IDs      string  `json:"level2_ids,omitempty"`
	Report         string  `json:"report,omitempty"`
	Error          string  `json:"error,omitempty"`
	CreatedAt      string  `json:"created_at"`
	CompletedAt    *string `json:"completed_at,omitempty"`
}

func (h *TaskHandler) PutDevicePreference(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	var req PreferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	device, err := h.deviceSvc.UpdatePreference(ctx, deviceID, req.Role, req.Preference)
	if err != nil {
		http.Error(w, "failed to update preference", http.StatusInternalServerError)
		return
	}

	resp := PreferenceResponse{
		DeviceID:   device.DeviceID,
		Role:       device.Role,
		Preference: device.Preference,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *TaskHandler) PostDeviceJob(w http.ResponseWriter, r *http.Request) {
	if defaultRunner == nil {
		http.Error(w, "pipeline runner not initialized", http.StatusInternalServerError)
		return
	}

	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.TimeRangeStart == "" || req.TimeRangeEnd == "" {
		http.Error(w, "time_range_start and time_range_end are required", http.StatusBadRequest)
		return
	}

	timeStart, err := time.Parse(time.RFC3339, req.TimeRangeStart)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid time_range_start format: %v", err), http.StatusBadRequest)
		return
	}
	timeEnd, err := time.Parse(time.RFC3339, req.TimeRangeEnd)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid time_range_end format: %v", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	job, err := h.jobSvc.CreateJob(ctx, deviceID, timeStart, timeEnd)
	if err != nil {
		http.Error(w, "failed to start pipeline job", http.StatusInternalServerError)
		return
	}

	input := steps.RSSJobInput{
		DeviceID:       deviceID,
		TimeRangeStart: timeStart,
		TimeRangeEnd:   timeEnd,
	}
	resp := jobModelToResponse(job, deviceID, input)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *TaskHandler) GetDeviceJob(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	job, err := h.jobSvc.GetLatestJob(ctx, deviceID)
	if errors.Is(err, items.ErrNotFound) {
		http.Error(w, "no jobs found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobToResponse(job))
}

func (h *TaskHandler) GetDeviceJobByID(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	jobIDStr := r.PathValue("job_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	var jobID uint
	if _, err := fmt.Sscanf(jobIDStr, "%d", &jobID); err != nil {
		http.Error(w, "invalid job_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	job, err := h.jobSvc.GetJobByID(ctx, jobID, deviceID)
	if errors.Is(err, items.ErrNotFound) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobToResponse(job))
}

func jobToResponse(job *models.Job) JobResponse {
	resp := JobResponse{
		ID:          job.ID,
		DeviceID:    job.DeviceID,
		Status:      job.Status,
		CurrentStep: job.CurrentStep,
		Level2IDs:   job.Level2IDs,
		Report:       job.Report,
		Error:       job.Error,
		CreatedAt:   job.CreatedAt.Format(time.RFC3339),
	}
	if job.CompletedAt != nil {
		s := job.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &s
	}

	var input steps.RSSJobInput
	if err := json.Unmarshal([]byte(job.Input), &input); err == nil {
		if !input.TimeRangeStart.IsZero() {
			s := input.TimeRangeStart.Format(time.RFC3339)
			resp.TimeRangeStart = &s
		}
		if !input.TimeRangeEnd.IsZero() {
			s := input.TimeRangeEnd.Format(time.RFC3339)
			resp.TimeRangeEnd = &s
		}
	}
	return resp
}

func jobModelToResponse(job *models.Job, deviceID string, input steps.RSSJobInput) JobResponse {
	resp := JobResponse{
		ID:       job.ID,
		DeviceID: deviceID,
		Status:   job.Status,
		Error:    job.Error,
	}
	s := input.TimeRangeStart.Format(time.RFC3339)
	e := input.TimeRangeEnd.Format(time.RFC3339)
	resp.TimeRangeStart = &s
	resp.TimeRangeEnd = &e
	resp.CreatedAt = job.CreatedAt.Format(time.RFC3339)
	return resp
}