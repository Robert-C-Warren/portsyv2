package uiapi

import (
	"Portsy/backend/internal/als"
	"Portsy/backend/internal/core/hash"
	"Portsy/backend/internal/core/scan"
	"Portsy/backend/internal/sync"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

type DetectChangesResp struct {
	Files      []sync.Change
	Counts     map[sync.ChangeType]int
	SampleRefs []string
}

func (a *API) DetectChanges(ctx context.Context, projectRoot string) (*DetectChangesResp, error) {
	// load baseling
	baseline := make(map[string]string)
	_ = readJSON(filepath.Join(projectRoot, ".portsy", "hashmap.json"), &baseline)

	// scan
	entries, err := scan.WalkProject(projectRoot, nil)
	if err != nil {
		return nil, err
	}

	current := make(map[string]string)
	sizes := make(map[string]int64)
	for _, e := range entries {
		// quick path: if size/mt match baseline and you're caching stat info, skip rehash
		h, err := hash.FileHash(e.Abs)
		if err != nil {
			return nil, err
		}
		current[e.Rel] = h
		sizes[e.Rel] = e.Size
	}

	cs := sync.Diff(current, baseline, sizes)

	// enrich with ALS refs if any .als changed
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

func readJSON(p string, v interface{}) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func dedupe(in []string) []string {
	m := map[string]struct{}{}
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
