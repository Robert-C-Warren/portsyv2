package main

import (
	"context"
	"embed"
	"log"

	"Portsy/backend/uiapi"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

var (
	app *App
	api *uiapi.API
)

func main() {
	app = NewApp()
	api = &uiapi.API{} // <-- expose DetectChanges + event/log emitter

	err := wails.Run(&options.App{
		Title:  "Portsy",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 255},
		OnStartup: func(ctx context.Context) {
			// hand the Wails ctx to both layers
			app.Startup(ctx)
			api.SetContext(ctx)
		},
		// bind BOTH so the frontend gets ../wailsjs/go/uiapi/API bindings
		Bind: []interface{}{
			app,
			api,
		},
		LogLevel:           logger.DEBUG,
		LogLevelProduction: logger.ERROR,
	})
	if err != nil {
		log.Fatal(err)
	}
}
