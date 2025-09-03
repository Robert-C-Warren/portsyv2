package sync

import "maps"

type ChangeType string
const (
	Added ChangeType = "added"
	Modified ChangeType = "modified"
	Deleted ChangeType = "deleted"
	Renamed ChangeType = "renamed"
)

type Change struct {
	Path string
	Type ChangeType
	OldHash string
	NewHash string
	ByteDelta int64
}

type ChangeSet struct {
	Files []Change
	Counts map[ChangeType]int
	SampleRefs []string // from als.Meta, for the log panel
}

func Diff(local map[string]string, remote map[string]string, sizes map[string]int64) ChangeSet {
	cs := ChangeSet{Counts: map[ChangeType]int{}}
	visited := map[string]struct{}{}

	for p, h := range local {
		visited[p] = struct{}{}
		if rh, ok := remote[p]; !ok {
			cs.Files = append(cs.Files, Change{Path: p, Type: Added, OldHash: "", NewHash: h, ByteDelta: sizes[p]})
			cs.Counts[Added]++
		} else if rh != h {
			cs.Files = append(cs.Files, Change{Path: p, Type: Modified, OldHash: rh, NewHash: ByteDelta: sizes[p]})
			cs.Counts[Modified]++
		}
	}
	for p, rh := range remote {
		if _, ok := visited[p]; !ok {
			cs.Files = append(cs.Files, Change{Path: p, Type: Deleted, OldHash: rh, NewHash: "", ByteDelta: 0})
			cs.Counts[Deleted]++
		}
	}
	return cs
}