package backend

import (
	"os"
	"path/filepath"
	"strings"
)

type AbletonProject struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	AlsFile    string      `json:"alsFile"`
	HasPortsy  bool        `json:"hasPortsy"`
	LastCommit *CommitMeta `json:"lastCommit,omitempty"`
}

// Scans rootPath for subfolders containing .als files
func ScanProjects(rootPath string) ([]AbletonProject, error) {
	var projects []AbletonProject

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectName := entry.Name()
		projectPath := filepath.Join(rootPath, projectName)

		files, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}

		var alsPath string
		var firstALS string
		preferred := projectName + ".als"

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			if strings.EqualFold(filepath.Ext(f.Name()), ".als") {
				fp := filepath.Join(projectPath, f.Name())
				if firstALS == "" {
					firstALS = fp
				}
				// Prefer <FolderName>.als (case-insensitive)
				if strings.EqualFold(f.Name(), preferred) {
					alsPath = fp
					break
				}
			}
		}
		if alsPath == "" {
			alsPath = firstALS
		}
		if alsPath == "" {
			continue // No .als directly inside folder
		}

		hasPortsy := false
		if fi, err := os.Stat(filepath.Join(projectPath, ".portsy")); err == nil && fi.IsDir() {
			hasPortsy = true
		}

		projects = append(projects, AbletonProject{
			Name:      projectName,
			Path:      projectPath,
			AlsFile:   alsPath,
			HasPortsy: hasPortsy,
		})
	}

	return projects, nil
}
