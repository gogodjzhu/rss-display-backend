package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type PythonRunner struct {
	PythonPath string
	ScriptPath string
	DataDir    string
}

func NewPythonRunner(cfg PythonConfig) *PythonRunner {
	return &PythonRunner{
		PythonPath: cfg.PythonPath,
		ScriptPath: cfg.ScriptPath,
		DataDir:    cfg.DataDir,
	}
}

type PythonConfig struct {
	PythonPath string
	ScriptPath string
	DataDir    string
}

func (r *PythonRunner) EnsureDataDir() error {
	if err := os.MkdirAll(r.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create pipeline data dir: %w", err)
	}
	return nil
}

func (r *PythonRunner) InputPath(taskID uint, stage string) string {
	return fmt.Sprintf("%s/%d_%s_in.json", r.DataDir, taskID, stage)
}

func (r *PythonRunner) OutputPath(taskID uint, stage string) string {
	return fmt.Sprintf("%s/%d_%s_out.json", r.DataDir, taskID, stage)
}

func (r *PythonRunner) WriteJSON(path string, data any) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON to %s: %w", path, err)
	}
	return nil
}

func (r *PythonRunner) ReadJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(target); err != nil {
		return fmt.Errorf("failed to decode JSON from %s: %w", path, err)
	}
	return nil
}

func (r *PythonRunner) Run(mode, inputPath, outputPath string) error {
	start := time.Now()
	cmd := exec.Command(r.PythonPath, r.ScriptPath,
		"--mode", mode,
		"--input", inputPath,
		"--output", outputPath,
	)
	cmd.Dir = getProjectRoot()

	pipelineLog.Printf("running python: %s %s --mode %s --input %s --output %s",
		r.PythonPath, r.ScriptPath, mode, inputPath, outputPath)

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if len(output) > 0 {
		pipelineLog.Printf("python %s output (%s):\n%s", mode, elapsed, string(output))
	}

	if err != nil {
		return fmt.Errorf("python %s failed (%s): %w\noutput: %s", mode, elapsed, err, string(output))
	}

	pipelineLog.Printf("python %s completed in %s", mode, elapsed)
	return nil
}