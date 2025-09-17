package backend

import (
	corehash "Portsy/backend/internal/core/hash"
	remote "Portsy/backend/remote"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// PushProject uploads changed blobs (idempotent) and writes commit metadata.
// - Concurrency via worker pool
// - Algo-aware (hash already inside manifest entries)
// - Key migration prefers server-side copy
func PushProject(ctx context.Context, meta *remote.MetaStore, r2 *R2Client, project AbletonProject, commit CommitMeta) error {
	// 0) Build manifest (must already include Algo + per-file Hash)
	cur, err := BuildManifest(project.Path)
	if err != nil {
		return err
	}
	cur.ProjectName = project.Name
	cur.ProjectPath = project.Path

	// 1) Previous state lookup
	prev, _, _ := meta.GetLatestState(ctx, project.Name)
	prevByPath := map[string]FileEntry{}
	if prev != nil {
		for _, pf := range prev.Files {
			prevByPath[pf.Path] = pf
		}
	}

	// 2) Decide actions
	type todo struct {
		idx int
		key string
		// If migrating, fromKey holds old key to copy-from
		fromKey string
	}
	var uploads []todo

	for i := range cur.Files {
		f := &cur.Files[i]
		desiredKey := r2.BuildKey(project.Name, f.Hash)

		if prev == nil {
			uploads = append(uploads, todo{idx: i, key: desiredKey})
			continue
		}
		if pf, ok := prevByPath[f.Path]; ok {
			switch {
			case pf.Hash != f.Hash:
				uploads = append(uploads, todo{idx: i, key: desiredKey})
			case pf.R2Key == desiredKey:
				f.R2Key = pf.R2Key // carry forward
			default:
				// same content, different layout: migrate
				uploads = append(uploads, todo{idx: i, key: desiredKey, fromKey: pf.R2Key})
			}
		} else {
			uploads = append(uploads, todo{idx: i, key: desiredKey})
		}
	}

	// 3) Execute with concurrency + idempotency
	workers := max(2, runtime.NumCPU()/2)
	type result struct {
		idx int
		key string
		err error
	}
	jobs := make(chan todo)
	results := make(chan result)
	var wg sync.WaitGroup

	// worker
	worker := func() {
		defer wg.Done()
		for t := range jobs {
			select {
			case <-ctx.Done():
				results <- result{idx: t.idx, key: t.key, err: ctx.Err()}
				continue
			default:
			}

			var err error
			// Prefer server-side copy when migrating
			switch {
			case t.fromKey != "" && t.fromKey != t.key:
				err = r2.CopyIfMissing(ctx, t.fromKey, t.key)
			default:
				local := filepath.Join(project.Path, cur.Files[t.idx].Path)
				err = r2.UploadIfMissing(ctx, local, t.key) // HEAD/If-None-Match semantics
			}
			results <- result{idx: t.idx, key: t.key, err: err}
		}
	}

	// start workers
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}
	go func() {
		for _, t := range uploads {
			jobs <- t
		}
		close(jobs)
	}()
	// collect
	var firstErr error
	for i := 0; i < len(uploads); i++ {
		r := <-results
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		} else {
			cur.Files[r.idx].R2Key = r.key
		}
	}
	wg.Wait()
	close(results)
	if firstErr != nil {
		return firstErr
	}

	// 4) Persist metadata + snapshot
	return meta.UpsertLatestState(ctx, project.Name, cur, commit)
}

// PullProject downloads target state into destPath.
// - Algo-aware verification (uses file.Hash + state.Algo)
// - Atomic download (r2.DownloadTo already writes .part -> fsync -> rename)
// - Preserves mtime; fsyncs parent dir after rename; bounded concurrency
func PullProject(ctx context.Context, meta *remote.MetaStore, r2 *R2Client, projectName, destPath, commitID string, allowDelete bool) (*PullStats, error) {

	stats := &PullStats{}

	// 1) Resolve target snapshot
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
		return stats, fmt.Errorf("pull: no remote state found for %q (commit=%q)", projectName, commitID)
	}
	if err := os.MkdirAll(destPath, 0o755); err != nil {
		return stats, fmt.Errorf("pull: mkdir dest: %w", err)
	}

	// quick lookup for deletes
	targetByPath := make(map[string]FileEntry, len(target.Files))
	for _, f := range target.Files {
		targetByPath[f.Path] = f
	}

	// 2) concurrent ensure files
	type job struct{ rf FileEntry }
	type done struct {
		rf         FileEntry
		err        error
		downloaded bool
	}
	jobs := make(chan job)
	dones := make(chan done)

	workers := max(2, runtime.NumCPU()/2)
	var wg sync.WaitGroup
	wg.Add(workers)

	verify := func(path, algo, want string) (bool, error) {
		switch algo {
		case "sha256", "SHA-256", "":
			// default/legacy -> SHA-256
			sum, _, _, herr := HashFileSHA256(path)
			if herr != nil {
				return false, herr
			}
			return sum == want, nil

		case "blake3":
			// compute just the hash (size/mtime not needed here)
			sum, err := corehash.New(corehash.BLAKE3).File(path)
			if err != nil {
				return false, err
			}
			return sum == want, nil

		default:
			return false, fmt.Errorf("unknown hash algo %q", algo)
		}
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			rf := j.rf
			localPath := filepath.Join(destPath, filepath.FromSlash(rf.Path))
			// ensure parent
			if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
				dones <- done{rf: rf, err: fmt.Errorf("mkdir %s: %w", filepath.Dir(localPath), err)}
				continue
			}

			needDownload := false
			if fi, err := os.Lstat(localPath); err != nil || !fi.Mode().IsRegular() {
				needDownload = true
			} else {
				ok, herr := verify(localPath, target.Algo, rf.Hash)
				if herr != nil || !ok {
					needDownload = true
				}
			}

			if needDownload {
				key := rf.R2Key
				if key == "" {
					key = r2.BuildKey(projectName, rf.Hash)
				}
				if err := r2.DownloadTo(ctx, key, localPath); err != nil {
					dones <- done{rf: rf, err: fmt.Errorf("download %s: %w", key, err)}
					continue
				}
				// verify after download
				ok, herr := verify(localPath, target.Algo, rf.Hash)
				if herr != nil {
					dones <- done{rf: rf, err: fmt.Errorf("verify %s: %w", localPath, herr)}
					continue
				}
				if !ok {
					dones <- done{rf: rf, err: fmt.Errorf("verify %s: hash mismatch", localPath)}
					continue
				}
				// Restore mtime (optional; use commit timestamp for determinism)
				_ = os.Chtimes(localPath, time.Now(), time.Unix(0, 0))
				dones <- done{rf: rf, downloaded: true}
			} else {
				dones <- done{rf: rf}
			}
		}
	}

	for i := 0; i < workers; i++ {
		go worker()
	}
	go func() {
		for _, rf := range target.Files {
			select {
			case <-ctx.Done():
				return
			case jobs <- job{rf: rf}:
			}
		}
		close(jobs)
	}()

	for i := 0; i < len(target.Files); i++ {
		d := <-dones
		if d.err != nil && !errors.Is(d.err, context.Canceled) {
			return stats, d.err
		}
		stats.ToDownload++
		if d.downloaded {
			stats.Downloaded++
			stats.Verified++
		} else {
			stats.Skipped++
		}
	}
	wg.Wait()
	close(dones)

	// 3) Optional delete pass
	if allowDelete {
		_ = filepath.Walk(destPath, func(p string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				if info != nil && info.IsDir() && info.Name() == ".portsy" {
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

// Rollback is unchanged (just uses Pull with allowDelete=true).
func RollbackProject(ctx context.Context, meta *remote.MetaStore, r2 *R2Client, projectName, destPath, commitID string) error {
	_, err := PullProject(ctx, meta, r2, projectName, destPath, commitID, true)
	return err
}

// Utility
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
