package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/esp32-rss-display/backend/server/database"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/esp32-rss-display/backend/server/pipeline"
	"github.com/esp32-rss-display/backend/server/pipeline/steps"
	"gorm.io/gorm"
)

var defaultRunner *pipeline.Runner

// InitRunner sets the global Runner used by job endpoints.
func InitRunner(r *pipeline.Runner) {
	defaultRunner = r
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

func PutDevicePreference(w http.ResponseWriter, r *http.Request) {
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

	db := database.GetDB()

	var device models.Device
	err := db.Where("device_id = ?", deviceID).First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		device = models.Device{
			DeviceID:   deviceID,
			Role:       req.Role,
			Preference: req.Preference,
			LastSeen:   time.Now(),
			CreatedAt:  time.Now(),
		}
		if err := db.Create(&device).Error; err != nil {
			http.Error(w, "failed to create device", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		http.Error(w, "failed to load device", http.StatusInternalServerError)
		return
	} else {
		updates := map[string]any{}
		if req.Role != "" {
			updates["role"] = req.Role
		}
		if req.Preference != "" {
			updates["preference"] = req.Preference
		}
		if len(updates) > 0 {
			if err := db.Model(&device).Updates(updates).Error; err != nil {
				http.Error(w, "failed to update preference", http.StatusInternalServerError)
				return
			}
		}
	}

	if err := db.Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		http.Error(w, "failed to reload device", http.StatusInternalServerError)
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

func PostDeviceJob(w http.ResponseWriter, r *http.Request) {
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

	input := steps.RSSJobInput{
		DeviceID:       deviceID,
		TimeRangeStart: timeStart,
		TimeRangeEnd:   timeEnd,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	job, err := defaultRunner.Submit(context.Background(), steps.PipelineName, inputJSON)
	if err != nil {
		http.Error(w, "failed to start pipeline job", http.StatusInternalServerError)
		return
	}

	// Associate device with the job record.
	db := database.GetDB()
	db.Model(&models.Job{}).Where("id = ?", job.ID).
		Updates(map[string]any{"device_id": deviceID, "updated_at": time.Now()})

	resp := jobRecordToResponse(job, deviceID, timeStart, timeEnd)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func GetDeviceJob(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	db := database.GetDB()
	var job models.Job
	if err := db.Where("device_id = ?", deviceID).Order("created_at DESC, id DESC").First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "no jobs found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobModelToResponse(&job))
}

func GetDeviceJobByID(w http.ResponseWriter, r *http.Request) {
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

	db := database.GetDB()
	var job models.Job
	if err := db.Where("id = ? AND device_id = ?", jobID, deviceID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobModelToResponse(&job))
}

// jobModelToResponse converts a models.Job row to a JobResponse.
// It parses the Input JSON to extract the original time range.
func jobModelToResponse(job *models.Job) JobResponse {
	resp := JobResponse{
		ID:          job.ID,
		DeviceID:    job.DeviceID,
		Status:      job.Status,
		CurrentStep: job.CurrentStep,
		Level2IDs:   job.Level2IDs,
		Report:      job.Report,
		Error:       job.Error,
		CreatedAt:   job.CreatedAt.Format(time.RFC3339),
	}
	if job.CompletedAt != nil {
		s := job.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &s
	}

	// Parse time range from the stored Input JSON.
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

// jobRecordToResponse builds a JobResponse directly from the freshly-created JobRecord
// (before any DB round-trip is needed for time range data).
func jobRecordToResponse(job *pipeline.JobRecord, deviceID string, timeStart, timeEnd time.Time) JobResponse {
	resp := JobResponse{
		ID:       job.ID,
		DeviceID: deviceID,
		Status:   string(job.Status),
		Error:    job.Error,
	}
	s := timeStart.Format(time.RFC3339)
	e := timeEnd.Format(time.RFC3339)
	resp.TimeRangeStart = &s
	resp.TimeRangeEnd = &e
	resp.CreatedAt = job.CreatedAt.Format(time.RFC3339)
	return resp
}
