package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/httputil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
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
	server.Address = `localhost:33333`
	server.VerifyFile = `/_layouts/default.html`
	server.SetFileSystem(FS(false))

	server.Get(`/project.json`, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set(`Content-Type`, `application/json`)

		if pkg := httputil.Q(req, `package`); pkg == `` {
			json.NewEncoder(w).Encode(module)
		} else {
			var found *Package

			if err := module.Walk(func(p *Package) error {
				if p.ImportPath == pkg || p.Name == pkg {
					found = p
					return stop
				} else {
					return nil
				}
			}); err == nil && found != nil {
				json.NewEncoder(w).Encode(found)
			} else if found == nil {
				http.Error(w, fmt.Sprintf("package %q not found", pkg), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusNotFound)
			}
		}
	})

	return server.Serve(func(s *diecast.Server) error {
		s.BindingPrefix = `http://` + s.Address

		log.Infof(s.Address)
		time.Sleep(100000 * time.Minute)

		// copy all FS assets to target dir
		for _, asset := range maputil.StringKeys(_escData) {
			if _escData[asset].isDir {
				continue
			} else if stringutil.HasAnyPrefix(asset, `/_layouts`, `/_includes`) {
				continue
			}

			if err := renderRequestAndWriteFile(targetDir, asset, s); err != nil {
				return err
			}
		}

		return module.Walk(func(pkg *Package) error {
			log.Infof("package: %s", pkg.Name)
			return renderRequestAndWriteFile(targetDir, `/pkg?package=`+pkg.Name, s)
		})
	})
}

func renderRequestAndWriteFile(targetDir string, path string, server *diecast.Server) error {
	var req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", server.BindingPrefix, path), nil)
	var w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if res := w.Result(); httputil.Is2xx(res.StatusCode) {
		defer res.Body.Close()
		var targetPath = filepath.Join(targetDir, path)

		if filepath.Ext(targetPath) == `` {
			targetPath += `.html`
		}

		log.Infof("Writing file %s (%d bytes)", targetPath, res.ContentLength)

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err == nil {
			if _, err := fileutil.WriteFile(res.Body, targetPath); err == nil {
				return res.Body.Close()
			} else {
				return fmt.Errorf("%s: %v", path, err)
			}
		} else {
			return fmt.Errorf("%s: %v", path, err)
		}
	} else {
		log.Warningf("bad path %q: HTTP %v: %v", path, res.Status, typeutil.String(res.Body))
		return nil
	}
}
