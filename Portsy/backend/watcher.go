package backend

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v2/pkg/runtime"
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

	// DEBUG________________________________J
	log.Printf("[WatchProjectALS] watching %s (als=%s)", projectName, alsPath)
	runtime.EventsEmit(ctx, "log", fmt.Sprintf("[WatchProjectALS] watching %s (als=%s)", projectName, alsPath))

	alsPathLC := strings.ToLower(filepath.Clean(alsPath))
	alsBaseLC := strings.ToLower(filepath.Base(alsPathLC))
	projDirLC := strings.ToLower(filepath.Clean(projectPath))

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
			log.Printf("[WatchProjectALS] ctx done for %s", projectName)
			return ctx.Err()

		case ev := <-w.Events:
			log.Printf("[fsnotify] %s op=%v", ev.Name, ev.Op)
			runtime.EventsEmit(ctx, "log", fmt.Sprintf("[fsnotify] %s op=%v", ev.Name, ev.Op))

			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) == 0 {
				continue
			}
			nameLC := strings.ToLower(filepath.Clean(ev.Name))
			baseLC := strings.ToLower(filepath.Base(nameLC))

			if nameLC == alsPathLC {
				log.Printf("[WatchProjectALS] match direct %s", ev.Name)
				schedule()
				continue
			}
			if filepath.Dir(nameLC) == projDirLC && strings.HasSuffix(baseLC, ".als") && baseLC == alsBaseLC {
				log.Printf("[WatchProjectALS] match replace %s", ev.Name)
				schedule()
				continue
			}

		case err := <-w.Errors:
			if err != nil {
				log.Printf("[fsnotify:error] %v", err)
				runtime.EventsEmit(ctx, "log", fmt.Sprintf("[fsnotify:error] %v", err))
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
