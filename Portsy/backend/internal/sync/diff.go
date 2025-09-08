package sync

import (
	"sort"
)

type ChangeType string

const (
	Added    ChangeType = "added"
	Modified ChangeType = "modified"
	Deleted  ChangeType = "deleted"
	Renamed  ChangeType = "renamed" // reserved for later
)

type Change struct {
	Path      string
	Type      ChangeType
	OldHash   string
	NewHash   string
	ByteDelta int64 // For push: bytes to upload (Added/Modified = size, Deleted = 0)
}

type ChangeSet struct {
	Files      []Change
	Counts     map[ChangeType]int
	SampleRefs []string // optional enrichment from .als parsing
}

// Diff computes local→remote changes.
// - Added: in local, not in remote
// - Modified: hashes differ
// - Deleted: in remote, not in local
// sizes: local path -> size (used to estimate upload cost)
//
// ByteDelta policy (push-oriented):
//
//	Added/Modified = sizes[p] (full file upload for now)
//	Deleted        = 0 (no upload; remote deletion is metadata-only)
//
// NOTE: Inputs may be nil; treated as empty maps.
func Diff(local map[string]string, remote map[string]string, sizes map[string]int64) ChangeSet {
	if local == nil {
		local = map[string]string{}
	}
	if remote == nil {
		remote = map[string]string{}
	}
	// sizes may be nil; zero-value lookups yield 0 safely.

	cs := ChangeSet{
		Counts: make(map[ChangeType]int, 3),
		Files:  make([]Change, 0, len(local)+len(remote)),
	}
	visited := make(map[string]struct{}, len(local))

	for p, h := range local {
		visited[p] = struct{}{}
		if rh, ok := remote[p]; !ok {
			cs.Files = append(cs.Files, Change{
				Path: p, Type: Added, OldHash: "", NewHash: h, ByteDelta: sizes[p],
			})
			cs.Counts[Added]++
		} else if rh != h {
			cs.Files = append(cs.Files, Change{
				Path: p, Type: Modified, OldHash: rh, NewHash: h, ByteDelta: sizes[p],
			})
			cs.Counts[Modified]++
		}
	}

	for p, rh := range remote {
		if _, ok := visited[p]; !ok {
			cs.Files = append(cs.Files, Change{
				Path: p, Type: Deleted, OldHash: rh, NewHash: "", ByteDelta: 0,
			})
			cs.Counts[Deleted]++
		}
	}

	// Deterministic ordering: Type priority, then path lexicographically.
	sort.Slice(cs.Files, func(i, j int) bool {
		pi, pj := cs.Files[i], cs.Files[j]
		order := func(t ChangeType) int {
			switch t {
			case Added:
				return 0
			case Modified:
				return 1
			case Deleted:
				return 2
			case Renamed:
				return 3
			default:
				return 4
			}
		}
		oi, oj := order(pi.Type), order(pj.Type)
		if oi != oj {
			return oi < oj
		}
		return pi.Path < pj.Path
	})

	return cs
}

// HasChanges is a convenience for UI logic.
func (cs ChangeSet) HasChanges() bool {
	return len(cs.Files) > 0
}

// TotalBytes estimates bytes to push (Added+Modified total).
func (cs ChangeSet) TotalBytes() int64 {
	var n int64
	for _, c := range cs.Files {
		if c.Type == Added || c.Type == Modified {
			n += c.ByteDelta
		}
	}
	return n
}
