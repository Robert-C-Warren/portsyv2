package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"Portsy/backend"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

var (
	cliPath     = "portsy.exe"     // resolve from repo root in dev
	watchCancel context.CancelFunc // global cancel for the watcher
)

func runCLI(args ...string) (string, error) {
	cmd := exec.Command(cliPath, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if errb.Len() > 0 {
			return "", errors.New(errb.String())
		}
		return "", err
	}
	return out.String(), nil
}

func NewApp() *App {
	return &App{}
}

// Wails v2 uses context.Context here
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) ScanProjects(rootPath string) ([]backend.AbletonProject, error) {
	return backend.ScanProjects(rootPath)
}

// ---- JSON passthroughs to your CLI ----
func (a *App) ScanJSON(root string) (string, error) {
	return runCLI("-mode=scan", "-root", root, "-json")
}
func (a *App) PendingJSON(root string) (string, error) {
	return runCLI("-mode=pending", "-root", root, "-json")
}
func (a *App) DiffJSON(root, project string) (string, error) {
	return runCLI("-mode=diff", "-root", root, "-project", project, "-json")
}

// ---- actions ----
func (a *App) Push(root, project, msg string) (string, error) {
	if msg == "" {
		msg = "GUI push: " + time.Now().Format(time.RFC3339)
	}
	return runCLI("-mode=push", "-root", root, "-project", project, "-msg", msg)
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
	return runCLI(args...)
}
func (a *App) Rollback(project, dest, commit string) (string, error) {
	args := []string{"-mode=rollback", "-project", project}
	if dest != "" {
		args = append(args, "-dest", dest)
	}
	if commit != "" {
		args = append(args, "-commit", commit)
	}
	return runCLI(args...)
}

// ---- optional: in-process watcher + UI events ----
func (a *App) StartWatcherAll(root string, autopush bool) error {
	if watchCancel != nil {
		watchCancel()
		watchCancel = nil
	}
	ctx, cancel := context.WithCancel(a.ctx) // assumes your App already stores ctx in startup
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
				_, _ = runCLI("-mode=push", "-root", root, "-project", evt.ProjectName, "-msg", "autosync: "+time.Now().Format(time.RFC3339))
				runtime.EventsEmit(a.ctx, "pushDone", map[string]any{"project": evt.ProjectName})
			}
		})
	}()
	return nil
}

func (a *App) StopWatcherAll() {
	if watchCancel != nil {
		watchCancel()
		watchCancel = nil
	}
}
