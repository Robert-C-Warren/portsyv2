package backend

import (
	"os"
	"path/filepath"
)

type AbletonProject struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	AlsFile    string      `json:"alsFile"`
	HasPortsy  string      `json:"hasPortsy"`
	LastCommit *CommitMeta `json:"lastCommit, omitempty"`
}

type CommitMeta struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// Scans rootPath for subfolders containing .als files
func ScanProjects(rootPath string) ([]AbletonProject, error) {
	projects := []AbletonProject{}
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			projectPath := filepath.Join(rootPath, entry.Name())
			alsPath := ""
			hasPortsy := false

			// Look for .als file directly inside project folder
			files, _ := os.ReadDir(projectPath)
			for _, f := range files {
				if filepath.Ext(f.Name()) == ".als" {
					alsPath = filepath.Join(projectPath, f.Name())
				}
			}

			// Only add if .als is present one level down
			if alsPath != "" {

			}
		}
	}
}
