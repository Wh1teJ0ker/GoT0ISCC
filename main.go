package main

import (
	"embed"
	"log"

	"got0iscc/desktop/internal/bootstrap"
	desktopapi "got0iscc/desktop/internal/presentation/desktop"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	application, err := bootstrap.NewApplication()
	if err != nil {
		log.Fatal(err)
	}

	api := desktopapi.NewAPI(application)

	err = wails.Run(&options.App{
		Title:            "GoT0ISCC",
		Width:            1400,
		Height:           850,
		MinWidth:         1400,
		MinHeight:        850,
		MaxWidth:         1400,
		MaxHeight:        850,
		DisableResize:    true,
		WindowStartState: options.Normal,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 13, G: 18, B: 24, A: 1},
		OnStartup:        api.Startup,
		OnShutdown:       api.Shutdown,
		Bind: []interface{}{
			api,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
