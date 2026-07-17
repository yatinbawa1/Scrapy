package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

 frontendFS, _ := fs.Sub(assets, "frontend/dist")

 mux := http.NewServeMux()

 mux.Handle("/", http.FileServerFS(frontendFS))

 mux.HandleFunc("/cache/", func(w http.ResponseWriter, r *http.Request) {
   path := strings.TrimPrefix(r.URL.Path, "/cache/")
   path = strings.ReplaceAll(path, "..", "")
   http.ServeFile(w, r, app.appDir+"/cache/"+path)
 })

 err := wails.Run(&options.App{
		Title:     "Wallpaper Chooser",
		Width:     1400,
		Height:    900,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: mux,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
