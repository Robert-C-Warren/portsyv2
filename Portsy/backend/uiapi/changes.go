package uiapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"Portsy/backend/internal/als"
	"Portsy/backend/internal/core/hash"
	"Portsy/backend/internal/core/scan"
	syn "Portsy/backend/internal/sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// API is the Wails-exposed backend.
type API struct {
	ctx context.Context
}

// Called from your main on startup; keep the ctx.
func (a *API) SetContext(ctx context.Context) { a.ctx = ctx }

// DetectChangesResp is what the frontend can consume for details.
type DetectChangesResp struct {
	Files      []syn.Change           `json:"files"`
	Counts     map[syn.ChangeType]int `json:"counts"`
	SampleRefs []string               `json:"sampleRefs"`
}

// DetectChanges scans & diffs, emits events, returns details.
func (a *API) DetectChanges(ctx context.Context, projectRoot string) (*DetectChangesResp, error) {
	// prefer stored ctx for events/logs (Wails runtime is tied to startup ctx)
	if a.ctx == nil {
		a.ctx = ctx
	}

	runtime.LogInfof(a.ctx, "[diff] start projectRoot=%s", projectRoot)
	runtime.EventsEmit(a.ctx, "diff:status", map[string]any{
		"phase":     "start",
		"projectId": projectRoot, // you can swap to projectID if you have one
		"ts":        time.Now().UTC().Format(time.RFC3339),
	})

	// Load baseline hashmap (ignore if missing)
	baseline := make(map[string]string)
	_ = readJSON(filepath.Join(projectRoot, ".portsy", "hashmap.json"), &baseline)

	// Scan filesystem
	entries, err := scan.WalkProject(projectRoot, nil)
	if err != nil {
		runtime.LogErrorf(a.ctx, "[diff] scan error: %v", err)
		runtime.EventsEmit(a.ctx, "diff:status", map[string]any{
			"phase":     "error",
			"projectId": projectRoot,
			"error":     err.Error(),
		})
		return nil, err
	}

	current := make(map[string]string, len(entries))
	sizes := make(map[string]int64, len(entries))

	for _, e := range entries {
		h, err := hash.FileHash(e.Abs)
		if err != nil {
			runtime.LogErrorf(a.ctx, "[diff] hashing error on %s: %v", e.Rel, err)
			runtime.EventsEmit(a.ctx, "diff:status", map[string]any{
				"phase":     "error",
				"projectId": projectRoot,
				"error":     err.Error(),
			})
			return nil, err
		}
		current[e.Rel] = h
		sizes[e.Rel] = e.Size
	}

	cs := syn.Diff(current, baseline, sizes)

	// Enrich with .als sample refs if any .als changed
	var refs []string
	for _, ch := range cs.Files {
		if filepath.Ext(ch.Path) == ".als" {
			meta, err := als.Read(filepath.Join(projectRoot, ch.Path))
			if err == nil && len(meta.DetectedSamples) > 0 {
				refs = append(refs, meta.DetectedSamples...)
			}
		}
	}
	cs.SampleRefs = dedupe(refs)

	// Build a compact summary for the UI badge
	sum := SummaryFromChangeSet(projectRoot, cs)

	runtime.LogInfof(a.ctx, "[diff] done added=%d modified=%d deleted=%d",
		len(sum.Added), len(sum.Modified), len(sum.Deleted))

	// Push an event the Svelte side can listen for
	runtime.EventsEmit(a.ctx, "diff:status", map[string]any{
		"phase":     "done",
		"projectId": sum.ProjectID,
		"summary":   sum,
	})

	return &DetectChangesResp{
		Files: cs.Files, Counts: cs.Counts, SampleRefs: cs.SampleRefs,
	}, nil
}

func readJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func dedupe(in []string) []string {
	m := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := m[s]; ok {
			continue
		}
		m[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
