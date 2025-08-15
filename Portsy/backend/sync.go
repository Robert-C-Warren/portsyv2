package backend

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type PullStats struct {
	ToDownload int
	Downloaded int
	Verified   int
	Deleted    int
	Skipped    int // Existed locally with matching hash
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
func PullProject(ctx context.Context, meta *MetaStore, r2 *R2Client,
	projectName, destPath, commitID string, allowDelete bool) (*PullStats, error) {

	stats := &PullStats{}

	// 1) Resolve target state
	var target *ProjectState
	var err error
	if commitID == "" {
		target, _, err = meta.GetLatestState(ctx, projectName)
	} else {
		target, _, err = meta.GetStateByCommit(ctx, projectName, commitID)
	}
	if err != nil {
		return stats, fmt.Errorf("pull: read remote state: %w", err)
	}
	if target == nil {
		return stats, fmt.Errorf("pull: no remote state found for project %q (commit=%q)", projectName, commitID)
	}

	// 2) Quick lookup for deletes later
	targetByPath := make(map[string]FileEntry, len(target.Files))
	for _, f := range target.Files {
		targetByPath[f.Path] = f
	}

	// 3) Ensure files locally
	for _, rf := range target.Files {
		localPath := filepath.Join(destPath, filepath.FromSlash(rf.Path))
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return stats, fmt.Errorf("mkdir %s: %w", filepath.Dir(localPath), err)
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
			stats.ToDownload++
			key := rf.R2Key
			if key == "" {
				key = r2.BuildKey(projectName, rf.Hash) // must match your push scheme
			}
			log.Printf("pull: GET %s -> %s", key, localPath)
			if err := r2.DownloadTo(ctx, key, localPath); err != nil {
				return stats, fmt.Errorf("download %s: %w", key, err)
			}
			stats.Downloaded++

			// verify hash after download
			sum, _, _, herr := HashFileSHA256(localPath)
			if herr != nil {
				return stats, fmt.Errorf("verify %s: %w", localPath, herr)
			}
			if sum != rf.Hash {
				return stats, fmt.Errorf("verify %s: hash mismatch (got %s want %s)", localPath, sum, rf.Hash)
			}
			stats.Verified++
		} else {
			stats.Skipped++
		}
	}

	// 4) Optional delete
	if allowDelete {
		_ = filepath.Walk(destPath, func(p string, info os.FileInfo, walkErr error) error {
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
				if err := os.Remove(p); err == nil {
					stats.Deleted++
				}
			}
			return nil
		})
	}

	_ = EnsureAbletonFolderIcon(destPath)
	log.Printf("pull: done. toDownload=%d downloaded=%d verified=%d skipped=%d deleted=%d",
		stats.ToDownload, stats.Downloaded, stats.Verified, stats.Skipped, stats.Deleted)

	return stats, nil
}

// Convenience for rollback to a specific commit ID (in-place restore).
func RollbackProject(ctx context.Context, meta *MetaStore, r2 *R2Client, projectName, destPath, commitID string) error {
	_, err := PullProject(ctx, meta, r2, projectName, destPath, commitID, true)
	return err
}
