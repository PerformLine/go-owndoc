package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/httputil"
	"github.com/ghetzel/go-stockutil/maputil"
)

// Renders the provided module as a static website in the target directory.
func RenderHTML(targetDir string, module *Module) error {
	if module == nil || module.Package == nil {
		return fmt.Errorf("cannot render empty module")
	}

	if fileutil.DirExists(targetDir) {
		if err := os.RemoveAll(targetDir); err != nil {
			return err
		}
	} else if fileutil.FileExists(targetDir) {
		return fmt.Errorf("target path exists and is a file")
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	// start diecast rooted with FS() as its filesystem
	server := diecast.NewServer(nil)
	server.SetFileSystem(FS(false))

	// copy all FS assets to target dir
	for _, asset := range maputil.StringKeys(_escData) {
		var req = httptest.NewRequest(http.MethodGet, asset, nil)
		var w = httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if res := w.Result(); httputil.Is2xx(res.StatusCode) {
			defer res.Body.Close()
			var targetPath = filepath.Join(targetDir, asset)

			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err == nil {
				if _, err := fileutil.WriteFile(res.Body, targetPath); err == nil {
					res.Body.Close()
				} else {
					return fmt.Errorf("%s: %v", asset, err)
				}
			} else {
				return fmt.Errorf("%s: %v", asset, err)
			}
		} else {
			return fmt.Errorf("bad path: HTTP %v", res.Status)
		}
	}

	return nil
}
