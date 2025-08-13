package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type SaveEvent struct {
	ProjectName string
	ProjectPath string
	ALSPath     string
	DetectedAt  time.Time
}

// WatchProjectALS watches the project root and debounces top-level .als saves.
func WatchProjectALS(
	ctx context.Context,
	projectName, projectPath string,
	debounce time.Duration,
	onSave func(SaveEvent),
) error {
	alsPath, err := findTopLevelALS(projectPath)
	if err != nil {
		return err
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := w.Add(projectPath); err != nil {
		return fmt.Errorf("watch add: %w", err)
	}

	var tmr *time.Timer
	schedule := func() {
		if tmr != nil {
			tmr.Stop()
		}
		tmr = time.AfterFunc(debounce, func() {
			if err := waitFileStable(alsPath, 150*time.Millisecond, 10); err == nil {
				onSave(SaveEvent{
					ProjectName: projectName,
					ProjectPath: projectPath,
					ALSPath:     alsPath,
					DetectedAt:  time.Now(),
				})
			}
		})
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-w.Events:
			if !strings.EqualFold(filepath.Clean(ev.Name), filepath.Clean(alsPath)) {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) != 0 {
				schedule()
			}
		case err := <-w.Errors:
			if err != nil {
				// Log in your logger if you have one; continue watching
				_ = err
			}
		}
	}
}

func findTopLevelALS(projectPath string) (string, error) {
	entries, err := filepath.Glob(filepath.Join(projectPath, "*.als"))
	if err != nil || len(entries) == 0 {
		return "", errors.New("no .als at project root")
	}
	// Prefer FolderName.als if present
	folder := filepath.Base(projectPath)
	for _, p := range entries {
		if strings.EqualFold(filepath.Base(p), folder+".als") {
			return p, nil
		}
	}
	return entries[0], nil
}

// waitFileStable waits until size stops changing for `attempts` cycles.
func waitFileStable(p string, interval time.Duration, attempts int) error {
	var last int64 = -1
	for i := 0; i < attempts; i++ {
		fi, err := os.Stat(p)
		if err != nil {
			time.Sleep(interval)
			continue
		}
		if fi.Size() == last {
			return nil
		}
		last = fi.Size()
		time.Sleep(interval)
	}
	return errors.New("file not stable")
}
