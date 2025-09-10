package uiapi

import (
	"encoding/json"
	"strings"
)

// UIFIle is the per-filpackage uiapi

// UIFile is the per-file entry used by the UI.
// Status is one of: "added" | "modified" | "deleted".
type UIFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// UIProjectDiff is the normalized, per-project diff that the UI consumes.
type UIProjectDiff struct {
	Project      string   `json:"project"`
	ChangedCount int      `json:"changedCount"`
	Files        []UIFile `json:"files"`
}

// UISummary is a compact summary we emit for quick toasts / event payloads.
type UISummary struct {
	Project      string   `json:"project"`
	ChangedCount int      `json:"changedCount"`
	Added        []string `json:"added"`
	Modified     []string `json:"modified"`
	Deleted      []string `json:"deleted"`
}

// BuildSummaryFromProjectJSON turns a normalized per-project diff JSON
// (what your GetDiffForProject returns) into a quick summary with Added/Modified/Deleted.
// It never panics; on parse issues it returns an empty summary for the given project.
func BuildSummaryFromProjectJSON(project string, raw string) UISummary {
	sum := UISummary{
		Project:      project,
		ChangedCount: 0,
		Added:        []string{},
		Modified:     []string{},
		Deleted:      []string{},
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sum
	}

	// The watcher calls GetDiffForProject first, which already returns
	// the normalized UIProjectDiff shape. We still keep this robust.
	var diff UIProjectDiff
	if err := json.Unmarshal([]byte(raw), &diff); err == nil && (len(diff.Files) > 0 || diff.ChangedCount >= 0) {
		sum.Project = project
		for _, f := range diff.Files {
			switch strings.ToLower(f.Status) {
			case "added":
				sum.Added = append(sum.Added, f.Path)
			case "changed", "modified":
				sum.Modified = append(sum.Modified, f.Path)
			case "removed", "deleted":
				sum.Deleted = append(sum.Deleted, f.Path)
			}
		}
		// If ChangedCount isn't provided, compute from files length.
		if diff.ChangedCount > 0 {
			sum.ChangedCount = diff.ChangedCount
		} else {
			sum.ChangedCount = len(diff.Files)
		}
		return sum
	}

	// Fallback: tolerate a looser payload {Added:[], Changed:[], Removed:[]}
	var loose map[string]any
	if err := json.Unmarshal([]byte(raw), &loose); err != nil {
		return sum
	}

	// Pull arrays if present
	if v, ok := loose["Added"]; ok {
		sum.Added = append(sum.Added, toStringList(v)...)
	}
	if v, ok := loose["Changed"]; ok {
		sum.Modified = append(sum.Modified, toStringList(v)...)
	}
	if v, ok := loose["Removed"]; ok {
		sum.Deleted = append(sum.Deleted, toStringList(v)...)
	}

	// Optionally look for a "files" array of objects with {path,status}
	if v, ok := loose["files"]; ok {
		if arr, ok := v.([]any); ok {
			for _, it := range arr {
				if m, ok := it.(map[string]any); ok {
					p, _ := m["path"].(string)
					if p == "" {
						continue
					}
					s, _ := m["status"].(string)
					switch strings.ToLower(s) {
					case "added":
						sum.Added = append(sum.Added, p)
					case "changed", "modified":
						sum.Modified = append(sum.Modified, p)
					case "removed", "deleted":
						sum.Deleted = append(sum.Deleted, p)
					}
				}
			}
		}
	}

	sum.ChangedCount = len(sum.Added) + len(sum.Modified) + len(sum.Deleted)
	return sum
}

// toStringList tolerates arrays of strings or arrays of {path: "..."} objects.
func toStringList(v any) []string {
	out := []string{}
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			switch x := it.(type) {
			case string:
				if x != "" {
					out = append(out, x)
				}
			case map[string]any:
				if p, ok := x["path"].(string); ok && p != "" {
					out = append(out, p)
				}
			}
		}
	}
	return out
}
