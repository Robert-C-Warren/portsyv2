package backend

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ALSLogicalDiff struct {
	Samples struct {
		Added   []string `json:"added"`
		Removed []string `json:"removed"`
		Changed []string `json:"changed"`
	} `json:"samples"`
	MIDI struct {
		AddedClips   []string `json:"addedClips"`
		RemovedClips []string `json:"removedClips"`
		ChangedClips []string `json:"changedClips"`
	} `json:"midi"`
}

type HashLookup func(relPath string) string

// ComputeALSLogicalDiff compares PREV vs CURR ALS content and produces a logical diff.
// - prevALS: ungzipped XML bytes of previously committed .als (pass nil if none)
// - crrALSPath: path to CURR .als (gzipped on disk). We'll ungzip internally.
// - projectRoot: needed to resolve realtive sample paths and hash current sample files.
// - prevHash: lookup function to get previous content hash for a sample rel path (from your last commit manifest)
func ComputeALSLogicalDiff(prevALS []byte, currALSPath, projectRoot string, prevHash HashLookup) (*ALSLogicalDiff, error) {
	currXML, err := readALSXML(currALSPath)
	if err != nil {
		return nil, err
	}
	prevIdx := buildALSIndex(prevALS, projectRoot)
	currIdx := buildALSIndex(currXML, projectRoot)

	// Samples add/remove
	ps, cs := toSet(prevIdx.samplePaths), toSet(currIdx.samplePaths)
	diff := &ALSLogicalDiff{}
	for p := range cs {
		if _, ok := ps[p]; !ok {
			diff.Samples.Added = append(diff.Samples.Added, p)
		}
	}
	for p := range ps {
		if _, ok := cs[p]; !ok {
			diff.Samples.Removed = append(diff.Samples.Removed, p)
		}
	}

	// Samples changed (present in both, content hash differs)
	for p := range cs {
		if _, ok := ps[p]; !ok {
			continue
		}
		prevH := ""
		if prevHash != nil {
			prevH = prevHash(p)
		}
		currH := hashCurrentSample(projectRoot, p)
		if prevH != "" && currH != "" && !strings.EqualFold(prevH, currH) {
			diff.Samples.Changed = append(diff.Samples.Changed, p)
		}
	}

	// MIDI clip diffs by notes-hash
	for name, h := range currIdx.midiHash {
		if ph, ok := prevIdx.midiHash[name]; !ok {
			diff.MIDI.AddedClips = append(diff.MIDI.AddedClips, name)
		} else if ph != h {
			diff.MIDI.ChangedClips = append(diff.MIDI.ChangedClips, name)
		}
	}
	for name := range prevIdx.midiHash {
		if _, ok := currIdx.midiHash[name]; !ok {
			diff.MIDI.RemovedClips = append(diff.MIDI.RemovedClips, name)
		}
	}

	sort.Strings(diff.Samples.Added)
	sort.Strings(diff.Samples.Removed)
	sort.Strings(diff.Samples.Changed)
	sort.Strings(diff.MIDI.AddedClips)
	sort.Strings(diff.MIDI.RemovedClips)
	sort.Strings(diff.MIDI.ChangedClips)

	return diff, nil
}

type alsIndex struct {
	samplePaths []string          // normalized, relaive if under project
	midiHash    map[string]string // clip-name -> sha256(notes-subtree)
}

// buildALSIndex constructs an alsIndex from UNGZIPPED xml bytes.
// If xml==nil, returns an empty index.
func buildALSIndex(xml []byte, projectRoot string) alsIndex {
	idx := alsIndex{
		samplePaths: nil,
		midiHash:    map[string]string{},
	}
	if len(xml) == 0 {
		return idx
	}
	// 1) samples: reuse existing extractor
	paths := extractSamplePaths(xml)
	idx.samplePaths = normalizeRelPaths(paths, projectRoot)

	// 2) MIDI: hash each MidiCLips Notes subtree
	idx.midiHash = midiNotesHashes(xml)
	return idx
}

func normalizeRelPaths(paths []string, projectRoot string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, p := range paths {
		pp := strings.TrimSpace(p)
		if pp == "" {
			continue
		}
		// absolutize to check containment, then convert back to relative if inside project
		if !filepath.IsAbs(pp) {
			pp = filepath.Join(projectRoot, filepath.FromSlash(pp))
		}
		pp = filepath.Clean(pp)
		if rel, err := filepath.Rel(projectRoot, pp); err == nil && !strings.HasPrefix(rel, "..") {
			pp = filepath.ToSlash(rel)
		} else {
			// keep as-is if not under project (still useful for UI)
			pp = filepath.ToSlash(pp)
		}
		if _, ok := seen[pp]; ok {
			continue
		}
		seen[pp] = struct{}{}
		out = append(out, pp)
	}
	sort.Strings(out)
	return out
}

func midiNotesHashes(xmlBytes []byte) map[string]string {
	out := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(xmlBytes))
	dec.Strict = false

	readValueAttr := func(se xml.StartElement) (string, bool) {
		for _, a := range se.Attr {
			if strings.EqualFold(a.Name.Local, "Value") {
				return a.Value, true
			}
		}
		return "", false
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "MidiClip" {
				var name string
				h := sha256.New()

				// walk MidiClip subtree
				depth := 1
				for depth > 0 {
					stok, err := dec.Token()
					if err != nil {
						break
					}
					switch st := stok.(type) {
					case xml.StartElement:
						depth++
						switch st.Name.Local {
						case "Name", "Annotation":
							if name == "" {
								if v, ok := readValueAttr(st); ok {
									name = v
								}
							}
						case "Notes":
							// hash Notes subtree for stability
							var buf bytes.Buffer
							enc := xml.NewEncoder(&buf)
							nDepth := 1
							_ = enc.EncodeToken(st) // include <Notes>
							for nDepth > 0 {
								t2, err2 := dec.Token()
								if err2 != nil {
									break
								}
								switch nt := t2.(type) {
								case xml.StartElement:
									nDepth++
									_ = enc.EncodeToken(nt)
								case xml.EndElement:
									_ = enc.EncodeToken(nt)
									nDepth--
								case xml.CharData:
									_ = enc.EncodeToken(nt)
								}
							}
							_ = enc.Flush()
							_, _ = io.Copy(h, &buf)
						}
					case xml.EndElement:
						depth--
					}
				}
				sum := hex.EncodeToString(h.Sum(nil))
				if name == "" {
					name = fmt.Sprintf("clip-%d", len(out)+1)
				}
				out[name] = sum
			}
		}
	}
	return out
}

func readALSXML(currAlsGzPath string) ([]byte, error) {
	f, err := os.Open(currAlsGzPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(gr)
}

func toSet(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

func hashCurrentSample(projectRoot, relOrAbs string) string {
	// resolve rel to abs under projectRoot
	p := relOrAbs
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectRoot, filepath.FromSlash(relOrAbs))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

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
		runtime.EventsEmit(ctx, "log", fmt.Sprintf("[WatchAll] start %s (%s)", name, projectPath))
		log.Printf("[WatchAll] start %s (%s)", name, projectPath)

		cctx, cancel := context.WithCancel(ctx)
		watchers[projectPath] = cancel
		go func() {
			err := WatchProjectALS(cctx, name, projectPath, debounce, onSave)
			log.Printf("[WatchAll] WatchProjectALS exit %s err=%v", name, err)
			runtime.EventsEmit(ctx, "log", fmt.Sprintf("[WatchAll] WatchProjectALS exit %s err=%v", name, err))
		}()
	}

	// Initial scan
	if projs, _ := findProjectsUnderRoot(root); len(projs) > 0 {
		for _, p := range projs {
			start(p)
		}
	}

	// DEBUG_________________________________
	runtime.EventsEmit(ctx, "log", "[WatchAll] initial scan complete")

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

	runtime.EventsEmit(ctx, "log", "[WatchAll] rescan triggered")

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
