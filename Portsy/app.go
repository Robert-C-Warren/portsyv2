package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"Portsy/backend"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	cliPath string
}

var (
	watchCancel context.CancelFunc // global cancel for the watcher
)

func NewApp() *App { return &App{} }

// ---- lifecycle ----

// NOTE: Must be exported (capital S) for Wails to call it.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// 1) Allow override via env var
	if p := os.Getenv("PORTSY_CLI"); p != "" {
		a.cliPath = p
	}

	// 2) Try alongside the running Wails binary
	if a.cliPath == "" {
		if exe, err := os.Executable(); err == nil {
			try := filepath.Join(filepath.Dir(exe), "portsy.exe")
			if _, err := os.Stat(try); err == nil {
				a.cliPath = try
			}
		}
	}

	// 3) Try current working directory (useful in `wails dev`)
	if a.cliPath == "" {
		try := "portsy.exe"
		if _, err := os.Stat(try); err == nil {
			a.cliPath = try
		}
	}

	// 4) Finally, PATH
	if a.cliPath == "" {
		if p, err := exec.LookPath("portsy.exe"); err == nil {
			a.cliPath = p
		}
	}

	if a.cliPath == "" {
		runtime.EventsEmit(a.ctx, "log",
			"ERROR: portsy.exe not found. Build it:\n  go build -o .\\portsy.exe .\\cmd\\portsy\nor set PORTSY_CLI to the full path.")
	} else {
		runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("CLI resolved: %s", a.cliPath))
	}
}

// ---- utilities ----

func (a *App) runCmd(ctx context.Context, args ...string) (string, error) {
	if a.cliPath == "" {
		return "", fmt.Errorf("portsy CLI not found (set PORTSY_CLI or place portsy.exe next to the app)")
	}
	runtime.EventsEmit(ctx, "log", fmt.Sprintf("CLI: %s %v", a.cliPath, args))

	cmd := exec.CommandContext(ctx, a.cliPath, args...) // args slice preserves spaces in paths
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
func (a *App) DiffJSON(root, project string) (string, error) {
	return a.runCmd(a.ctx, "-mode=diff", "-root", root, "-project", project, "-json")
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
	if watchCancel != nil {
		watchCancel()
		watchCancel = nil
	}
	ctx, cancel := context.WithCancel(a.ctx)
	watchCancel = cancel

	go func() {
		_ = backend.WatchAllProjects(ctx, root, 750*time.Millisecond, func(evt backend.SaveEvent) {
			_, _ = backend.CollectNewSamples(ctx, evt.ProjectPath, evt.ALSPath)
			runtime.EventsEmit(a.ctx, "alsSaved", map[string]any{
				"project": evt.ProjectName,
				"path":    evt.ALSPath,
				"at":      evt.DetectedAt.Format(time.RFC3339),
			})
			if autopush {
				_, _ = a.runCmd(a.ctx, "-mode=push", "-root", root, "-project", evt.ProjectName, "-msg", "autosync: "+time.Now().Format(time.RFC3339))
				runtime.EventsEmit(a.ctx, "pushDone", map[string]any{"project": evt.ProjectName})
			}
		})
	}()
	runtime.EventsEmit(a.ctx, "log", fmt.Sprintf("Watcher started on: %s (autopush=%v)", root, autopush))
	return nil
}

func (a *App) StopWatcherAll() {
	if watchCancel != nil {
		watchCancel()
		watchCancel = nil
		runtime.EventsEmit(a.ctx, "log", "Watcher stopped")
	}
}
