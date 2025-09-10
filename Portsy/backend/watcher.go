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
	if onSave == nil {
		return errors.New("onSave callback is nil")
	}
	alsPath, err := findTopLevelALS(projectPath)
	if err != nil {
		return err
	}

	log.Printf("[WatchProjectALS] watching %s (als=%s)", projectName, alsPath)
	runtime.EventsEmit(ctx, "log", fmt.Sprintf("[WatchProjectALS] watching %s (als=%s)", projectName, alsPath))

	// Normalize/prefetch lowercase forms for case-insensitive filesystems
	mkLC := func(p string) string { return strings.ToLower(filepath.Clean(p)) }
	alsPathLC := mkLC(alsPath)
	alsBaseLC := strings.ToLower(filepath.Base(alsPathLC))
	projDirLC := mkLC(projectPath)

	// Helper: filter out backup/temporary .als variants
	isRealALS := func(baseLower string) bool {
		if !strings.HasSuffix(baseLower, ".als") {
			return false
		}
		// Ignore Ableton/backup suffixes like .als~ or .als.tmp
		if strings.HasSuffix(baseLower, ".als~") || strings.HasSuffix(baseLower, ".als.tmp") {
			return false
		}
		return true
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := w.Add(projectPath); err != nil {
		return fmt.Errorf("watch add: %w", err)
	}

	// Debounce with a proper time.Timer we can reset safely
	var tmr *time.Timer
	var tmrC <-chan time.Time // current timer's channel (nil when inactive)

	stopTimer := func() {
		if tmr == nil {
			return
		}
		if !tmr.Stop() {
			// Timer already fired or draining; don't block, just nil out the channel
			select {
			case <-tmrC:
			default:
			}
		}
		tmr = nil
		tmrC = nil
	}

	schedule := func() {
		// Restart the timer
		if tmr == nil {
			tmr = time.NewTimer(debounce)
			tmrC = tmr.C
		} else {
			if !tmr.Stop() {
				select {
				case <-tmrC:
				default:
				}
			}
			tmr.Reset(debounce)
		}
	}

	fireIfStable := func() {
		// Check file stability; if it moved, try to re-resolve the ALS path at top-level
		// (e.g., user renamed the .als but kept it top-level)
		if _, err := os.Stat(alsPath); err != nil {
			// Re-resolve if current ALS vanished
			if newALS, ferr := findTopLevelALS(projectPath); ferr == nil {
				alsPath = newALS
				alsPathLC = mkLC(alsPath)
				alsBaseLC = strings.ToLower(filepath.Base(alsPathLC))
				log.Printf("[WatchProjectALS] ALS path updated -> %s", alsPath)
				runtime.EventsEmit(ctx, "log", fmt.Sprintf("[WatchProjectALS] ALS path updated -> %s", alsPath))
			}
		}
		if err := waitFileStable(alsPath, 150*time.Millisecond, 10); err == nil {
			onSave(SaveEvent{
				ProjectName: projectName,
				ProjectPath: projectPath,
				ALSPath:     alsPath,
				DetectedAt:  time.Now(),
			})
		}
	}

	for {
		select {
		case <-ctx.Done():
			stopTimer()
			log.Printf("[WatchProjectALS] ctx done for %s", projectName)
			return ctx.Err()

		case ev := <-w.Events:
			// Only react to meaningful ops
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) == 0 {
				continue
			}
			nameLC := mkLC(ev.Name)
			baseLC := strings.ToLower(filepath.Base(nameLC))

			log.Printf("[fsnotify] %s op=%v", ev.Name, ev.Op)
			runtime.EventsEmit(ctx, "log", fmt.Sprintf("[fsnotify] %s op=%v", ev.Name, ev.Op))

			// Only care about top-level files in the project folder
			if filepath.Dir(nameLC) != projDirLC {
				continue
			}
			if !isRealALS(baseLC) {
				continue
			}

			// Direct path match (same file) or "replace" (same base name)
			if nameLC == alsPathLC || baseLC == alsBaseLC {
				// Update alsPath if we matched by base but path changed (e.g., temp->final)
				if baseLC == alsBaseLC && nameLC != alsPathLC {
					alsPath = filepath.Join(projectPath, filepath.Base(ev.Name))
					alsPathLC = mkLC(alsPath)
					alsBaseLC = strings.ToLower(filepath.Base(alsPathLC))
					log.Printf("[WatchProjectALS] path replaced -> %s", alsPath)
					runtime.EventsEmit(ctx, "log", fmt.Sprintf("[WatchProjectALS] path replaced -> %s", alsPath))
				}
				schedule()
				continue
			}

		case err := <-w.Errors:
			if err != nil {
				log.Printf("[fsnotify:error] %v", err)
				runtime.EventsEmit(ctx, "log", fmt.Sprintf("[fsnotify:error] %v", err))
			}

		case <-tmrC:
			// Debounce window ended; check stability and fire callback
			stopTimer()
			fireIfStable()
		}
	}
}

func findTopLevelALS(projectPath string) (string, error) {
	entries, err := filepath.Glob(filepath.Join(projectPath, "*.als"))
	if err != nil || len(entries) == 0 {
		return "", errors.New("no .als at project root")
	}
	// Prefer FolderName.als if present; else lexicographically smallest for determinism
	folder := filepath.Base(projectPath)
	var fallback string
	for _, p := range entries {
		base := filepath.Base(p)
		if strings.EqualFold(base, folder+".als") {
			return p, nil
		}
		if fallback == "" || strings.ToLower(base) < strings.ToLower(filepath.Base(fallback)) {
			fallback = p
		}
	}
	return fallback, nil
}

// waitFileStable waits until BOTH size and mtime stop changing for `attempts` cycles.
// It treats any stat/open error as "not stable yet" to handle transient locks (Windows).
func waitFileStable(p string, interval time.Duration, attempts int) error {
	var lastSize int64 = -1
	var lastMod time.Time
	for i := 0; i < attempts; i++ {
		fi, err := os.Stat(p)
		if err != nil {
			time.Sleep(interval)
			continue
		}
		size := fi.Size()
		mod := fi.ModTime()

		// Try to open read-only; if it fails, someone might still be writing/locking the file.
		f, err := os.Open(p)
		if err == nil {
			_ = f.Close()
		} else {
			time.Sleep(interval)
			continue
		}

		if size == lastSize && mod.Equal(lastMod) {
			return nil
		}
		lastSize = size
		lastMod = mod
		time.Sleep(interval)
	}
	return errors.New("file not stable")
}
