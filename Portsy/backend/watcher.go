package backend

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type ProjectWatcher struct {
	watcher *fsnotify.Watcher
}

func NewProjectWatcher() (*ProjectWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &ProjectWatcher{watcher: w}, nil
}

func (pw *ProjectWatcher) WatchProject(project AbletonProject) error {
	err := pw.watcher.Add(filepath.Dir(project.AlsFile))
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-pw.watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					if filepath.Ext(event.Name) == ".als" {
						fmt.Printf("[%s] ALS file changed at %s\n", project.Name, time.Now())
						// TODO Trigger frontend to prompt to push
					}
				}
			case err, ok := <-pw.watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("Watcher error:", err)
			}
		}
	}()
	return nil
}

func (pw *ProjectWatcher) Close() {
	pw.watcher.Close()
}
