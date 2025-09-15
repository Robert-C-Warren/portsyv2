package main

import (
	"context"
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"Portsy/backend/uiapi"
)

// dev_assets.go / prod_assets.go will define this:
var assets embed.FS

var (
	app *App
	api *uiapi.API
)

func main() {
	app = NewApp()
	api = &uiapi.API{} // exposes DetectChanges + event/log emitter

	err := wails.Run(&options.App{
		Title:  "Portsy",
		Width:  1120,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 255},
		OnStartup: func(ctx context.Context) {
			app.Startup(ctx)
			api.SetContext(ctx)

			if err := api.InitMetaStore(
				os.Getenv("FIREBASE_PROJECT_ID"),
				os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
			); err != nil {
			}
		},
		OnShutdown: func(ctx context.Context) {
			// allow graceful teardown of watchers, goroutines, etc
			if closer, ok := interface{}(api).(interface{ Close() error }); ok {
				_ = closer.Close()
			}
			if closer, ok := interface{}(app).(interface{ Shutdown() error }); ok {
				_ = closer.Shutdown()
			}
		},
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
