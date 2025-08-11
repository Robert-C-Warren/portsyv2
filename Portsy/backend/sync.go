package backend

import (
	"context"
	"os"
	"path/filepath"
)

// Diff old vs new; return files that changed (by hash)
func diffChanged(old ProjectState, cur ProjectState) []FileEntry {
	oldMap := map[string]string{}
	for _, f := range old.Files {
		oldMap[f.Path] = f.Hash
	}
	var changed []FileEntry
	for _, f := range cur.Files {
		if oldMap[f.Path] != f.Hash {
			changed = append(changed, f)
		}
	}
	return changed
}

// PushProject uploads changed files to R2 and writes commit metadata+state to Firestore.
// It also migrates objects when the desired R2 key changes (e.g., key layout update).
func PushProject(ctx context.Context, meta *MetaStore, r2 *R2Client, project AbletonProject, commit CommitMeta) error {
	// Build current manifest
	cur, err := BuildManifest(project.Path)
	if err != nil {
		return err
	}
	cur.ProjectName = project.Name
	cur.ProjectPath = project.Path

	// Get previous state (if any)
	prev, _, _ := meta.GetLatestState(ctx, project.Name)

	// Build quick lookups from prev
	var prevByPath map[string]FileEntry
	if prev != nil {
		prevByPath = make(map[string]FileEntry, len(prev.Files))
		for _, pf := range prev.Files {
			prevByPath[pf.Path] = pf
		}
	}

	// Decide which files to upload
	type up struct {
		idx int
		key string
	}
	var uploads []up

	for i := range cur.Files {
		f := &cur.Files[i]
		desiredKey := r2.BuildKey(project.Name, f.Hash)

		var needUpload bool

		if prev == nil {
			// First push: upload everything
			needUpload = true
		} else if pf, ok := prevByPath[f.Path]; ok {
			// If content changed, upload
			if pf.Hash != f.Hash {
				needUpload = true
			} else {
				// Same content: migrate if the key scheme changed
				if pf.R2Key != desiredKey {
					needUpload = true
				} else {
					// Carry forward existing R2Key
					f.R2Key = pf.R2Key
				}
			}
		} else {
			// New file
			needUpload = true
		}

		if needUpload {
			uploads = append(uploads, up{idx: i, key: desiredKey})
		}
	}

	// Perform uploads (skip if object already exists at desired key)
	for _, u := range uploads {
		local := filepath.Join(project.Path, cur.Files[u.idx].Path)
		if exists, _ := r2.Exists(ctx, u.key); !exists {
			if err := ensureUploaded(ctx, r2, local, u.key); err != nil {
				return err
			}
		}
		// record key in current manifest
		cur.Files[u.idx].R2Key = u.key
	}

	// Persist metadata + snapshot to Firestore
	return meta.UpsertLatestState(ctx, project.Name, cur, commit)
}

func ensureUploaded(ctx context.Context, r2 *R2Client, local, key string) error {
	_, err := r2.UploadFile(ctx, local, key)
	return err
}

// PullProject syncs the remote state into destPath.
// If commitID == "", it pulls the latest. When allowDelete is true, files not in the
// target state will be deleted locally.
func PullProject(ctx context.Context, meta *MetaStore, r2 *R2Client, projectName, destPath, commitID string, allowDelete bool) error {
	// 1) Find target state (latest or by commit)
	var target *ProjectState
	var err error
	if commitID == "" {
		target, _, err = meta.GetLatestState(ctx, projectName)
	} else {
		target, _, err = meta.GetStateByCommit(ctx, projectName, commitID)
	}
	if err != nil {
		return err
	}
	if target == nil {
		return nil // nothing to do
	}

	// 2) Build quick lookups
	targetByPath := make(map[string]FileEntry, len(target.Files))
	for _, f := range target.Files {
		targetByPath[f.Path] = f
	}

	// 3) Ensure all target files exist locally (download if missing or hash differs)
	for _, rf := range target.Files {
		localPath := filepath.Join(destPath, filepath.FromSlash(rf.Path))
		// make parent dir
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return err
		}

		needDownload := false
		if fi, err := os.Stat(localPath); err != nil || !fi.Mode().IsRegular() {
			needDownload = true
		} else {
			sum, _, _, herr := HashFileSHA256(localPath)
			if herr != nil || sum != rf.Hash {
				needDownload = true
			}
		}
		if needDownload {
			key := rf.R2Key
			if key == "" {
				key = r2.BuildKey(projectName, rf.Hash)
			}
			if err := r2.DownloadTo(ctx, key, localPath); err != nil {
				return err
			}
		}
	}

	// 4) Optionally delete locals that are not in target (clean)
	if allowDelete {
		filepath.Walk(destPath, func(p string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if info.IsDir() {
				if info.Name() == ".portsy" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(destPath, p)
			rel = filepath.ToSlash(rel)
			if _, ok := targetByPath[rel]; !ok {
				_ = os.Remove(p) // best effort
			}
			return nil
		})
	}

	return nil
}

// Convenience for rollback to a specific commit ID (in-place restore).
func RollbackProject(ctx context.Context, meta *MetaStore, r2 *R2Client, projectName, destPath, commitID string) error {
	return PullProject(ctx, meta, r2, projectName, destPath, commitID, true)
}
