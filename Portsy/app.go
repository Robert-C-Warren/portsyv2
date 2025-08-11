package main

import (
	"context"

	"Portsy/backend"
)

type App struct {
	ctx context.Context
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
