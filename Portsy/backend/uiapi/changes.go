package uiapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"Portsy/backend/internal/als"
	"Portsy/backend/internal/core/hash"
	"Portsy/backend/internal/core/scan"
	syn "Portsy/backend/internal/sync"
)

// API is the Wails-exposed backend. If you already define this elsewhere,
// delete this and keep your original definition.
type API struct{}

// DetectChangesResp is what the frontend consumes for badges/logs.
type DetectChangesResp struct {
	Files      []syn.Change
	Counts     map[syn.ChangeType]int
	SampleRefs []string
}

func (a *API) DetectChanges(ctx context.Context, projectRoot string) (*DetectChangesResp, error) {
	// Load baseline hashmap
	baseline := make(map[string]string)
	_ = readJSON(filepath.Join(projectRoot, ".portsy", "hashmap.json"), &baseline)

	// Scan filesystem
	entries, err := scan.WalkProject(projectRoot, nil)
	if err != nil {
		return nil, err
	}

	current := make(map[string]string)
	sizes := make(map[string]int64)

	for _, e := range entries {
		h, err := hash.FileHash(e.Abs)
		if err != nil {
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
