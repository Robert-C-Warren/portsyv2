package backend

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// CollectNewSamples:
//  1. gunzips the .als into memory
//  2. extracts sample file references (absolute + relative)
//  3. copies any files not already present to Samples/Imported (dedup by hash)
//  4. returns list of copied destination paths
//
// We do NOT modify the .als. We keep the original .als on disk.
// The ungzipped XML is never written to disk (memory only).
func CollectNewSamples(ctx context.Context, projectPath, alsPath string) ([]string, error) {
	xmlBytes, err := ungzipALS(alsPath)
	if err != nil {
		return nil, fmt.Errorf("ungzip als: %w", err)
	}

	paths := extractSamplePaths(xmlBytes)
	if len(paths) == 0 {
		return nil, nil
	}

	importDir := filepath.Join(projectPath, "Samples", "Imported")
	if err := os.MkdirAll(importDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir Imported: %w", err)
	}

	copied := make([]string, 0, len(paths))
	seenHash := map[string]struct{}{}

	for _, p := range paths {
		select {
		case <-ctx.Done():
			return copied, ctx.Err()
		default:
		}

		// Normalize & absolutize
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(projectPath, filepath.FromSlash(p))
		}
		abs = filepath.Clean(abs)

		// Skip non-existent files quietly
		srcInfo, err := os.Stat(abs)
		if err != nil || srcInfo.IsDir() {
			continue
		}

		// If already under Samples/Imported, skip
		if isSubpath(abs, importDir) {
			continue
		}
		// If already inside the project (but not in Samples/**), we *currently* skip copying;
		// Portsy will sync it anyway. Flip this if you prefer strict collecting.
		if isSubpath(abs, projectPath) && !strings.Contains(strings.ToLower(abs), string(filepath.Separator)+"samples"+string(filepath.Separator)) {
			continue
		}

		// Dedup by content hash
		srcHash, err := fileSHA256(abs)
		if err != nil {
			continue
		}
		if _, ok := seenHash[srcHash]; ok {
			continue
		}

		destBase := filepath.Base(abs)
		destPath := filepath.Join(importDir, destBase)

		// If same-named file exists: if identical => skip, else mint "(n)" name
		if dstInfo, err := os.Stat(destPath); err == nil && !dstInfo.IsDir() {
			if dstHash, _ := fileSHA256(destPath); dstHash == srcHash {
				seenHash[srcHash] = struct{}{}
				continue
			}
			destPath = nextSuffixPath(importDir, destBase)
		}

		if err := copyFile(abs, destPath); err != nil {
			continue
		}
		seenHash[srcHash] = struct{}{}
		copied = append(copied, destPath)
	}

	return copied, nil
}

func ungzipALS(alsPath string) ([]byte, error) {
	f, err := os.Open(alsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, gr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// extractSamplePaths scans Ableton's XML for common path shapes:
//   - file:/// URIs
//   - Windows absolute paths (C:\...)
//   - relative "Samples/..." paths
func extractSamplePaths(xml []byte) []string {
	text := string(xml)
	exts := `(?i)\.(wav|aif|aiff|flac|mp3|ogg)`

	uniq := map[string]struct{}{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p == "" {
			return
		}
		// normalize slashes; we'll absolutize later
		p = strings.ReplaceAll(p, `\`, string(filepath.Separator))
		// keep forward slashes for rel paths; Join will handle them
		if _, ok := uniq[p]; !ok {
			uniq[p] = struct{}{}
		}
	}

	// 1) file:// and file://localhost URLs
	rURI := regexp.MustCompile(`file://(?:localhost/)?(?:[A-Za-z]:/|/)[^"<>\s]+` + exts)
	for _, m := range rURI.FindAllString(text, -1) {
		u := strings.TrimPrefix(m, "file://")
		u = strings.TrimPrefix(u, "localhost/")
		if dec, err := url.PathUnescape(u); err == nil {
			u = dec
		}
		add(u)
	}

	// 2) Absolute Windows paths
	rWin := regexp.MustCompile(`[A-Za-z]:\\[^"<>\r\n]+` + exts)
	for _, m := range rWin.FindAllString(text, -1) {
		add(m)
	}

	// 3) Relative "Samples/..." (also allow ./Samples/...)
	rRel := regexp.MustCompile(`(?:^|[/"'=])(?:\.?/)?(?:Samples/[^"'\r\n]+` + exts + `)`)
	for _, m := range rRel.FindAllString(text, -1) {
		m = strings.TrimLeft(m, `"'=/`)
		m = strings.TrimPrefix(m, "./")
		add(m)
	}

	// 4) <FileRef> blocks (Ableton's main schema)
	rBlock := regexp.MustCompile(`(?is)<FileRef[^>]*>.*?</FileRef>`)
	blocks := rBlock.FindAllString(text, -1)
	if len(blocks) > 0 {
		reAbs := regexp.MustCompile(`(?i)AbsolutePath\s+Value="([^"]+` + exts + `)"`)
		reUrl := regexp.MustCompile(`(?i)Url\s+Value="(file:[^"]+)"`)
		reRelAttr := regexp.MustCompile(`(?i)(?:RelativePath|Path)\s+Value="([^"]+)"`)
		reName := regexp.MustCompile(`(?i)(?:FileName|Name)\s+Value="([^"]+` + exts + `)"`)
		for _, b := range blocks {
			if m := reAbs.FindStringSubmatch(b); m != nil {
				add(m[1])
				continue
			}
			if m := reUrl.FindStringSubmatch(b); m != nil {
				u := strings.TrimPrefix(m[1], "file://")
				u = strings.TrimPrefix(u, "localhost/")
				if dec, err := url.PathUnescape(u); err == nil {
					u = dec
				}
				add(u)
			}
			var rel string
			if m := reRelAttr.FindStringSubmatch(b); m != nil {
				rel = m[1]
			}
			if m := reName.FindStringSubmatch(b); m != nil {
				if rel != "" {
					// avoid double slashes
					sep := "/"
					if strings.HasSuffix(rel, "/") || strings.HasSuffix(rel, `\`) {
						sep = ""
					}
					add(rel + sep + m[1])
				} else {
					add(m[1])
				}
			} else if rel != "" && regexp.MustCompile(exts+`$`).MatchString(rel) {
				// Relative path already includes filename
				add(rel)
			}
		}
	}

	out := make([]string, 0, len(uniq))
	for p := range uniq {
		out = append(out, p)
	}
	return out
}

func nextSuffixPath(dir, base string) string {
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	for i := 1; i < 1000; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", name, i, ext))
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return cand
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, time.Now().Unix(), ext))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func isSubpath(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	// rel == "." or a nested path means it's inside parent
	return rel != ".." && !strings.HasPrefix(rel, fmt.Sprintf("..%c", filepath.Separator))
}

// WatchAllProjects watches 'root' for any immediate child folder that contains a top-level .als.
// It spawns a WatchProjectALS for each, and picks up new projects created later.
func WatchAllProjects(
	ctx context.Context,
	root string,
	debounce time.Duration,
	onSave func(SaveEvent),
) error {
	root = filepath.Clean(root)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Add(root); err != nil {
		return fmt.Errorf("watch root: %w", err)
	}

	type cancelFn = context.CancelFunc
	watchers := map[string]cancelFn{} // key: projectPath

	start := func(projectPath string) {
		projectPath = filepath.Clean(projectPath)
		if _, ok := watchers[projectPath]; ok {
			return
		}
		name := filepath.Base(projectPath)
		cctx, cancel := context.WithCancel(ctx)
		watchers[projectPath] = cancel
		go func() {
			_ = WatchProjectALS(cctx, name, projectPath, debounce, onSave)
			// When WatchProjectALS returns (ctx canceled), we just exit goroutine
		}()
	}

	// Initial scan
	if projs, _ := findProjectsUnderRoot(root); len(projs) > 0 {
		for _, p := range projs {
			start(p)
		}
	}

	// Debounced rescan on root changes
	var rescanT *time.Timer
	rescan := func() {
		if rescanT != nil {
			rescanT.Stop()
		}
		rescanT = time.AfterFunc(300*time.Millisecond, func() {
			if projs, _ := findProjectsUnderRoot(root); len(projs) > 0 {
				for _, p := range projs {
					start(p)
				}
			}
		})
	}

	for {
		select {
		case <-ctx.Done():
			for _, cancel := range watchers {
				cancel()
			}
			return ctx.Err()
		case ev := <-w.Events:
			// Any creation/rename of an .als one level below the root triggers rescan
			if strings.EqualFold(filepath.Ext(ev.Name), ".als") {
				parent := filepath.Dir(ev.Name)
				if filepath.Dir(parent) == root {
					rescan()
				}
			} else if ev.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
				// new folder under root - rescan
				if filepath.Dir(ev.Name) == root {
					rescan()
				}
			}
		case err := <-w.Errors:
			if err != nil {
				_ = err // log if you have logger
			}
		}
	}
}

// findProjectsUnderRoot returns child directories of 'root' that contain a top-level .als.
func findProjectsUnderRoot(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pp := filepath.Join(root, e.Name())
		glob, _ := filepath.Glob(filepath.Join(pp, "*.als"))
		if len(glob) > 0 {
			out = append(out, pp)
		}
	}
	return out, nil
}
