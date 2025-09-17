package main

import (
	"Portsy/backend"
	ui "Portsy/backend/uiapi"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx         context.Context
	cliPath     string
	meta        *MetaStore
	currentRoot string
}

type RootStatsResult struct {
	DirCount    int  `json:"dirCount"`
	IsDriveRoot bool `json:"isDriveRoot"`
}

var (
	watchCancel context.CancelFunc // global cancel for the watcher
)

func NewApp() *App { return &App{} }

// ---- lifecycle ----

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Load .env so GUI has the same env as CLI
	_ = godotenv.Overload(".env", "../.env", "../../.env")

	// ---- locate CLI (as you had) ----
	if p := os.Getenv("PORTSY_CLI"); p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			a.cliPath = abs
		}
	}
	if a.cliPath == "" {
		if exe, err := os.Executable(); err == nil {
			try := filepath.Join(filepath.Dir(exe), "portsy.exe")
			if _, err := os.Stat(try); err == nil {
				a.cliPath = try
			}
		}
	}
	if a.cliPath == "" {
		if _, err := os.Stat("portsy.exe"); err == nil {
			if abs, err := filepath.Abs("portsy.exe"); err == nil {
				a.cliPath = abs
			}
		}
	}
	if a.cliPath == "" {
		if lp, err := exec.LookPath("portsy.exe"); err == nil {
			if abs, err := filepath.Abs(lp); err == nil {
				a.cliPath = abs
			}
		}
	}

	if a.cliPath == "" {
		runtime.EventsEmit(a.ctx, "log",
			"ERROR: portsy.exe not found. Build it:\n  go build -o .\\portsy.exe .\\cmd\\portsy\nor set PORTSY_CLI to the full path.")
	} else {
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("CLI resolved: %s", a.cliPath))
	}

	// ---- init Firestore MetaStore for GUI calls (ListRemoteProjects etc.) ----
	// Needs GCP_PROJECT_ID and GOOGLE_APPLICATION_CREDENTIALS
	proj := os.Getenv("GCP_PROJECT_ID")
	cred := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if strings.HasPrefix(cred, ".") {
		if abs, err := filepath.Abs(cred); err == nil {
			cred = abs
		}
	}
	if proj == "" || cred == "" {
		runtime.EventsEmit(a.ctx, "log", "Firestore not configured (set GCP_PROJECT_ID and GOOGLE_APPLICATION_CREDENTIALS). ListRemoteProjects will be unavailable.")
		return
	}
	if _, err := os.Stat(cred); err != nil {
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS not found at %q: %v", cred, err))
		return
	}
	metaCfg := backend.MetaStoreConfig{
		GCPProjectID:      proj,
		ServiceAccountKey: cred,
	}
	m, err := backend.NewMetaStore(ctx, metaCfg)
	if err != nil {
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("Firestore init error: %v", err))
		return
	}
	a.meta = m
	runtime.EventsEmit(a.ctx, "log", "Firestore connected ✓")
}

// ---- utilities ----

func (a *App) runCmd(ctx context.Context, args ...string) (string, error) {
	if a.cliPath == "" {
		return "", fmt.Errorf("portsy CLI not found (set PORTSY_CLI or place portsy.exe next to the app)")
	}
	runtime.EventsEmit(ctx, "log", fmt.Sprintf("CLI: %s %v", a.cliPath, args))

	if ctx == nil {
		ctx = a.ctx
	}
	cmd := exec.CommandContext(ctx, a.cliPath, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()

	stdout := out.String()
	stderr := errb.String()

	// Surface CLI output to the UI log (useful even on success)
	if stdout != "" {
		runtime.EventsEmit(ctx, "log", stdout)
	}
	if err != nil {
		// Include stderr in the error so the UI can show it
		if stderr != "" {
			return "", fmt.Errorf("%v\n%s", err, stderr)
		}
		return "", err
	}
	return stdout, nil
}

// RootStats returns immediate subdir count and whether the path is a drive root (e.g., "C:\").
func (a *App) RootStats(path string) (RootStatsResult, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return RootStatsResult{}, err
	}
	dirs := 0
	for _, e := range entries {
		if e.IsDir() {
			dirs++
		}
	}
	p := filepath.Clean(path)
	p = strings.ReplaceAll(p, "/", "\\")
	isDrive := len(p) == 3 && p[1] == ':' && p[2] == '\\'
	return RootStatsResult{DirCount: dirs, IsDriveRoot: isDrive}, nil
}

// ---- dialogs ----

// PickRoot opens a native directory chooser and returns the selected path
func (a *App) PickRoot() (string, error) {
	opts := runtime.OpenDialogOptions{
		Title:                "Select Ableton Projects Root",
		CanCreateDirectories: true,
		ShowHiddenFiles:      true,
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, opts)
	if err != nil {
		return "", err
	}
	return dir, nil
}

// ---- direct (non-CLI) convenience, optional ----

func (a *App) ScanProjects(rootPath string) ([]backend.AbletonProject, error) {
	return backend.ScanProjects(rootPath)
}

// ---- CLI passthroughs ----

func (a *App) ScanJSON(root string) (string, error) {
	return a.runCmd(a.ctx, "-mode=scan", "-root", root, "-json")
}

func (a *App) PendingJSON(root string) (string, error) {
	return a.runCmd(a.ctx, "-mode=pending", "-root", root, "-json")
}

func (a *App) DiffJSON(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("no root selected")
	}
	out, err := a.runCmd(a.ctx, "-mode", "diff", "-root", root, "-json")
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(out)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		// Convert usage/log text on stdout into a real error for the UI.
		firstLine := t
		if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
			firstLine = firstLine[:i]
		}
		return "", fmt.Errorf("CLI did not return JSON: %s", firstLine)
	}
	return t, nil
}

func (a *App) Push(root, project, msg string) (string, error) {
	if msg == "" {
		msg = "GUI push: " + time.Now().Format(time.RFC3339)
	}
	return a.runCmd(a.ctx, "-mode=push", "-root", root, "-project", project, "-msg", msg)
}

func (a *App) Pull(project, dest, commit string, force bool) (string, error) {
	args := []string{"-mode=pull", "-project", project}
	if dest != "" {
		args = append(args, "-dest", dest)
	}
	if commit != "" {
		args = append(args, "-commit", commit)
	}
	if force {
		args = append(args, "-force")
	}
	return a.runCmd(a.ctx, args...)
}

func (a *App) Rollback(project, dest, commit string) (string, error) {
	args := []string{"-mode=rollback", "-project", project}
	if dest != "" {
		args = append(args, "-dest", dest)
	}
	if commit != "" {
		args = append(args, "-commit", commit)
	}
	return a.runCmd(a.ctx, args...)
}

// ---- watcher (in-process), emits UI events ----
func (a *App) StartWatcherAll(root string, autopush bool) error {
	a.currentRoot = root
	if watchCancel != nil {
		watchCancel()
		watchCancel = nil
	}
	ctx, cancel := context.WithCancel(a.ctx)
	watchCancel = cancel

	log.Printf("[StartWatcherAll] root=%s autopush=%v", root, autopush)
	runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("[StartWatcherAll] root=%s autopush=%v", root, autopush))

	go func() {
		log.Printf("[StartWatcherAll] entering WatchAllProjects on %s", root)
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("[StartWatcherAll] entering WatchAllProjects on %s", root))

		_ = backend.WatchAllProjects(ctx, root, 750*time.Millisecond, func(evt backend.SaveEvent) {
			// existing logs...
			_, _ = backend.CollectNewSamples(ctx, evt.ProjectPath, evt.ALSPath)

			// --- NEW: build & emit a DiffSummary ---
			js, err := a.GetDiffForProject(evt.ProjectName)
			if err != nil {
				log.Printf("[Diff] %s error: %v", evt.ProjectName, err)
			}
			summary := ui.BuildSummaryFromProjectJSON(evt.ProjectName, js)
			// defend against nil slices
			if summary.Added == nil {
				summary.Added = []string{}
			}
			if summary.Modified == nil {
				summary.Modified = []string{}
			}
			if summary.Deleted == nil {
				summary.Deleted = []string{}
			}
			runtime.EventsEmit(a.ctx, "project:diff", summary)

			runtime.EventsEmit(a.ctx, "alsSaved", map[string]any{
				"project": evt.ProjectName,
				"path":    evt.ALSPath,
				"at":      time.Now().Format(time.RFC3339),
				"summary": func() string {
					js, err := a.GetDiffForProject(evt.ProjectName)
					if err != nil || js == "" {
						return ""
					}
					var d ui.UIProjectDiff
					if json.Unmarshal([]byte(js), &d) != nil || len(d.Files) == 0 {
						return ""
					}
					max := 5
					if len(d.Files) < max {
						max = len(d.Files)
					}
					var parts []string
					for _, f := range d.Files[:max] {
						parts = append(parts, fmt.Sprintf("%s: %s", f.Status, f.Path))
					}
					if len(d.Files) > max {
						parts = append(parts, fmt.Sprintf("(+%d more)", len(d.Files)-max))
					}
					return strings.Join(parts, ", ")
				}(),
			})

			if autopush {
				_, _ = a.runCmd(a.ctx, "-mode=push", "-root", root, "-project", evt.ProjectName, "-msg", "autosync: "+time.Now().Format(time.RFC3339))
				runtime.EventsEmit(a.ctx, "pushDone", map[string]any{"project": evt.ProjectName})
			}
		})

		log.Printf("[StartWatcherAll] WatchAllProjects returned (ctx canceled?)")
		runtime.EventsEmit(a.ctx, "log", "[StartWatcherAll] WatchAllProjects returned (ctx canceled?)")
	}()

	runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("Watcher started on: %s (autopush=%v)", root, autopush))
	log.Printf("Watcher started on: %s (autopush=%v)", root, autopush)

	return nil
}

func (a *App) StopWatcherAll() {
	if watchCancel != nil {
		watchCancel()
		watchCancel = nil
		runtime.EventsEmit(a.ctx, "log", "Watcher stopped")
	}
}

func (a *App) ListRemoteProjects() ([]backend.ProjectDoc, error) {
	if a.meta == nil {
		return nil, fmt.Errorf("firestore not configured in GUI (set GCP_PROJECT_ID and GOOGLE_APPLICATION_CREDENTIALS, or check Startup logs)")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.meta.ListProjects(ctx)
}

// GetDiffForProject returns a single project's diff in the UI shape:
// { project, changedCount, files:[{path,status}] }
func (a *App) GetDiffForProject(name string) (string, error) {
	// Determine root (use whatever you store when StartWatcherAll runs)
	root := a.currentRoot
	if root == "" {
		// Fall back to an empty diff rather than erroring
		empty := ui.UIProjectDiff{Project: name, ChangedCount: 0, Files: []ui.UIFile{}}
		b, _ := json.Marshal(empty)
		return string(b), nil
	}

	// Ask the CLI/backing code for the FULL diff JSON, then slice here.
	rawJSON, err := a.DiffProjectJSON(root, name)
	if err != nil {
		return "", err
	}
	if rawJSON == "" {
		empty := ui.UIProjectDiff{Project: name, ChangedCount: 0, Files: []ui.UIFile{}}
		b, _ := json.Marshal(empty)
		return string(b), nil
	}

	var full any
	if err := json.Unmarshal([]byte(rawJSON), &full); err != nil {
		return "", fmt.Errorf("parse diff json: %w", err)
	}

	// --------------- Find the per-project slice ---------------
	var asMap map[string]any
	var asArr []any

	switch t := full.(type) {
	case []any:
		asArr = t
	case map[string]any:
		asMap = t
	default:
		// Unknown payload; return empty
		empty := ui.UIProjectDiff{Project: name, ChangedCount: 0, Files: []ui.UIFile{}}
		b, _ := json.Marshal(empty)
		return string(b), nil
	}

	// Case A: array
	if len(asArr) > 0 {
		for _, it := range asArr {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			n := ""
			if v, ok := m["project"].(string); ok {
				n = v
			} else if v, ok := m["name"].(string); ok {
				n = v
			} else if v, ok := m["projectName"].(string); ok {
				n = v
			}
			p := ""
			if v, ok := m["projectPath"].(string); ok {
				p = v
			} else if v, ok := m["path"].(string); ok {
				p = v
			}
			if sameProjectKey(n, name) || sameProjectKey(p, name) {
				out := normalizeProjectDiffFromMap(name, m)
				b, _ := json.Marshal(out)
				return string(b), nil
			}
		}
	}

	// Case B: object with "projects" map
	if projAny, ok := asMap["projects"]; ok {
		if projMap, ok := projAny.(map[string]any); ok {
			for k, v := range projMap {
				m, ok := v.(map[string]any)
				if !ok {
					continue
				}
				n := ""
				if val, ok := m["project"].(string); ok {
					n = val
				} else if val, ok := m["name"].(string); ok {
					n = val
				} else if val, ok := m["projectName"].(string); ok {
					n = val
				}
				p := ""
				if val, ok := m["projectPath"].(string); ok {
					p = val
				} else if val, ok := m["path"].(string); ok {
					p = val
				}
				if sameProjectKey(k, name) || sameProjectKey(n, name) || sameProjectKey(p, name) {
					out := normalizeProjectDiffFromMap(name, m)
					b, _ := json.Marshal(out)
					return string(b), nil
				}
			}
		}
	}

	// Case C: plain object keyed by name/path
	if len(asMap) > 0 {
		for k, v := range asMap {
			if k == "projects" {
				continue
			}
			if sameProjectKey(k, name) {
				if m, ok := v.(map[string]any); ok {
					out := normalizeProjectDiffFromMap(name, m)
					b, _ := json.Marshal(out)
					return string(b), nil
				}
				// if it's not a map, still return empty normalized
				break
			}
			// also check identity hints inside the value
			if m, ok := v.(map[string]any); ok {
				n := ""
				if val, ok := m["project"].(string); ok {
					n = val
				} else if val, ok := m["name"].(string); ok {
					n = val
				} else if val, ok := m["projectName"].(string); ok {
					n = val
				}
				p := ""
				if val, ok := m["projectPath"].(string); ok {
					p = val
				} else if val, ok := m["path"].(string); ok {
					p = val
				}
				if sameProjectKey(n, name) || sameProjectKey(p, name) {
					out := normalizeProjectDiffFromMap(name, m)
					b, _ := json.Marshal(out)
					return string(b), nil
				}
			}
		}
	}

	// Not found → empty normalized object (never return undefined values)
	empty := ui.UIProjectDiff{Project: name, ChangedCount: 0, Files: []ui.UIFile{}}
	b, _ := json.Marshal(empty)
	return string(b), nil
}

func baseName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Handle both / and \ on Windows
	s = strings.ReplaceAll(s, "\\", "/")
	parts := strings.Split(s, "/")
	return parts[len(parts)-1]
}

func sameProjectKey(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	al := strings.ToLower(strings.TrimSpace(a))
	bl := strings.ToLower(strings.TrimSpace(b))
	return al == bl || baseName(al) == baseName(bl)
}

// normalize a per-project "raw" diff object into UI shape
func normalizeProjectDiffFromMap(name string, m map[string]any) ui.UIProjectDiff {
	// Already UI-like?
	if filesAny, ok := m["files"]; ok {
		out := ui.UIProjectDiff{Project: name}
		if cc, ok := m["changedCount"].(float64); ok {
			out.ChangedCount = int(cc)
		}
		if arr, ok := filesAny.([]any); ok {
			out.Files = make([]ui.UIFile, 0, len(arr))
			for _, it := range arr {
				if obj, ok := it.(map[string]any); ok {
					f := ui.UIFile{}
					if p, ok := obj["path"].(string); ok {
						f.Path = p
					}
					if s, ok := obj["status"].(string); ok {
						f.Status = s
					}
					if f.Path != "" {
						out.Files = append(out.Files, f)
					}
				}
			}
			if out.ChangedCount == 0 {
				out.ChangedCount = len(out.Files)
			}
			return out
		}
	}

	// CLI-like: Added/Changed/Removed (items may be {"path": "..."} or just strings)
	pushAll := func(arr any, status string, into *[]ui.UIFile) {
		list, _ := arr.([]any)
		for _, it := range list {
			switch v := it.(type) {
			case string:
				if v != "" {
					*into = append(*into, ui.UIFile{Path: v, Status: status})
				}
			case map[string]any:
				if p, ok := v["path"].(string); ok && p != "" {
					*into = append(*into, ui.UIFile{Path: p, Status: status})
					continue
				}
				if p, ok := v["Path"].(string); ok && p != "" {
					*into = append(*into, ui.UIFile{Path: p, Status: status})
				}
			}
		}
	}

	files := make([]ui.UIFile, 0, 16)
	pushAll(m["Added"], "added", &files)
	pushAll(m["Changed"], "modified", &files)
	pushAll(m["Removed"], "deleted", &files)

	return ui.UIProjectDiff{
		Project:      name,
		ChangedCount: len(files),
		Files:        files,
	}
}

func (a *App) DiffProjectJSON(root, project string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("no root selected")
	}
	if strings.TrimSpace(project) == "" {
		return "", fmt.Errorf("no project specified")
	}
	out, err := a.runCmd(a.ctx, "-mode", "diff", "-root", root, "-project", project, "-json")
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(out)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		firstLine := t
		if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
			firstLine = firstLine[:i]
		}
		return "", fmt.Errorf("CLI did not return JSON: %s", firstLine)
	}
	return t, nil
}
