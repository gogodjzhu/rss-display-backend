package pipeline

import (
	"os"
	"path/filepath"
)

func getProjectRoot() string {
	if root := os.Getenv("RSS_PROJECT_ROOT"); root != "" {
		return root
	}
	execPath, err := os.Executable()
	if err == nil {
		candidates := []string{
			filepath.Join(filepath.Dir(execPath), ".."),
			filepath.Join(filepath.Dir(execPath), "..", ".."),
		}
		for _, c := range candidates {
			abs, _ := filepath.Abs(c)
			if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
				return abs
			}
		}
	}
	wd, _ := os.Getwd()
	return wd
}