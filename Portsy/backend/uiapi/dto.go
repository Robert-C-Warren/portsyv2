package uiapi

import (
	"Portsy/backend/internal/sync"
	"encoding/json"
	"strings"
)

// What the Svelte store expects
type DiffSummary struct {
	ProjectID string   `json:"projectId"`
	Added     []string `json:"added"`
	Modified  []string `json:"modified"`
	Deleted   []string `json:"deleted"`
}

// Optional: if you still parse JSON from GetDiffForProject()
type UIProjectDiff struct {
	Project      string   `json:"project"`
	ChangedCount int      `json:"changedCount"`
	Files        []UIFile `json:"files"`
}
type UIFile struct {
	Path   string `json:"path"`
	Status string `json:"status"` // "added" | "modified" | "deleted"
}

// Adapter from core ChangeSet -> UI summary
func SummaryFromChangeSet(projectID string, cs sync.ChangeSet) DiffSummary {
	sum := DiffSummary{
		ProjectID: projectID,
		Added:     []string{},
		Modified:  []string{},
		Deleted:   []string{},
	}
	for _, ch := range cs.Files {
		switch ch.Type {
		case sync.Added:
			sum.Added = append(sum.Added, ch.Path)
		case sync.Modified:
			sum.Modified = append(sum.Modified, ch.Path)
		case sync.Deleted:
			sum.Deleted = append(sum.Deleted, ch.Path)
		}
	}
	return sum
}

// Build DiffSummary from your existing per-project JSON (GetDiffForProject)
func BuildSummaryFromProjectJSON(projectName, js string) DiffSummary {
	if js == "" {
		return DiffSummary{ProjectID: projectName, Added: []string{}, Modified: []string{}, Deleted: []string{}}
	}
	var d UIProjectDiff
	if json.Unmarshal([]byte(js), &d) != nil {
		return DiffSummary{ProjectID: projectName, Added: []string{}, Modified: []string{}, Deleted: []string{}}
	}
	sum := DiffSummary{
		ProjectID: projectName,
		Added:     []string{},
		Modified:  []string{},
		Deleted:   []string{},
	}
	for _, f := range d.Files {
		switch strings.ToLower(f.Status) {
		case "added", "new", "a":
			sum.Added = append(sum.Added, f.Path)
		case "modified", "changed", "m":
			sum.Modified = append(sum.Modified, f.Path)
		case "deleted", "removed", "d":
			sum.Deleted = append(sum.Deleted, f.Path)
		}
	}
	return sum
}
