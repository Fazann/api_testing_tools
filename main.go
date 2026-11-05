package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"api-test-kits/internal/appdata"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

//go:embed all:frontend/dist
var assets embed.FS

// appIcon is the window/taskbar icon on Linux. Windows uses build/windows/icon.ico,
// which wails build embeds in the executable.
//
//go:embed build/appicon.png
var appIcon []byte

func main() {
	path, err := appdata.DatabasePath()
	if err != nil {
		log.Fatal(err)
	}
	store, err := storage.Open(path)
	if err != nil {
		log.Fatalf("open local database: %v", err)
	}
	defer store.Close()

	svc, err := service.New(store)
	if err != nil {
		log.Fatalf("start service: %v", err)
	}
	app := NewApp(svc)

	err = wails.Run(&options.App{
		Title:     service.AppName,
		Width:     1280,
		Height:    820,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind:      []any{app},
		// Only warnings and errors are logged; request data is never logged.
		LogLevel:           logger.WARNING,
		LogLevelProduction: logger.ERROR,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		Linux: &linux.Options{
			Icon:                appIcon,
			ProgramName:         appdata.DirName,
			WebviewGpuPolicy:    linux.WebviewGpuPolicyOnDemand,
			WindowIsTranslucent: false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
