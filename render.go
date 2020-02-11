package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/httputil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
	"github.com/mcuadros/go-defaults"
)

var SkipAssets = []string{
	`/_layouts`,
	`/_includes`,
	`/pkg.html`,
	`/package.json`,
}

type RenderOptions struct {
	TargetDir  string `default:"docs"`
	Properties map[string]interface{}
}

// Renders the provided module as a static website in the target directory.
func RenderHTML(module *Module, options *RenderOptions) error {
	if options == nil {
		options = new(RenderOptions)
	}

	defaults.SetDefaults(options)

	if module == nil || module.Package == nil {
		return fmt.Errorf("cannot render empty module")
	}

	if fileutil.DirExists(options.TargetDir) {
		if err := os.RemoveAll(options.TargetDir); err != nil {
			return err
		}
	} else if fileutil.FileExists(options.TargetDir) {
		return fmt.Errorf("target path exists and is a file")
	}

	if err := os.MkdirAll(options.TargetDir, 0755); err != nil {
		return err
	}

	// start diecast rooted with FS() as its filesystem
	server := diecast.NewServer(nil)
	server.Address = `localhost:0` // use ephemeral local port
	server.VerifyFile = `/_layouts/default.html`
	server.OverridePageObject = options.Properties

	if ui := os.Getenv(`OWNDOC_UI`); fileutil.DirExists(ui) {
		server.RootPath = ui
	} else {
		server.SetFileSystem(FS(false))
	}

	server.Get(`/module.json`, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set(`Content-Type`, `application/json`)
		enc := json.NewEncoder(w)
		enc.SetIndent(``, `    `)
		enc.Encode(module)
	})

	server.Get(`/package.json`, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set(`Content-Type`, `application/json`)

		if pkg := httputil.Q(req, `package`); pkg != `` {
			var found *Package

			if err := module.Walk(func(p *Package) error {
				if p.ImportPath == pkg || p.Name == pkg {
					found = p
					return stop
				} else {
					return nil
				}
			}); err == nil && found != nil {
				enc := json.NewEncoder(w)
				enc.SetIndent(``, `    `)
				enc.Encode(found)
			} else if found == nil {
				http.Error(w, fmt.Sprintf("package %q not found", pkg), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusNotFound)
			}
		} else {
			http.Error(w, `Must provide ?package parameter`, http.StatusBadRequest)
		}
	})

	return server.Serve(func(s *diecast.Server) error {
		s.BindingPrefix = `http://` + s.Address

		// copy all FS assets to target dir
		for _, asset := range maputil.StringKeys(_escData) {
			if _escData[asset].isDir {
				continue
			} else if stringutil.HasAnyPrefix(asset, SkipAssets...) {
				continue
			}

			if err := renderRequestAndWriteFile(options.TargetDir, asset, s, ``); err != nil {
				return err
			}
		}

		if err := renderRequestAndWriteFile(options.TargetDir, `/module.json`, s, ``); err != nil {
			return err
		}

		return module.Walk(func(pkg *Package) error {
			log.Infof("package: %s", pkg.Name)

			if err := renderRequestAndWriteFile(
				options.TargetDir,
				`/package.json?package=`+pkg.Name,
				s,
				filepath.Join(`pkg`, pkg.ImportPath+`.json`),
			); err != nil {
				return err
			}

			return renderRequestAndWriteFile(
				options.TargetDir,
				`/pkg?package=`+pkg.Name,
				s,
				filepath.Join(`pkg`, pkg.ImportPath),
			)
		})
	})
}

func renderRequestAndWriteFile(targetDir string, path string, server *diecast.Server, targetName string) error {
	var req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", server.BindingPrefix, path), nil)
	var w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if res := w.Result(); httputil.Is2xx(res.StatusCode) {
		defer res.Body.Close()
		var targetPath = filepath.Join(targetDir, path)

		if targetName != `` {
			targetPath = filepath.Join(targetDir, targetName)
		}

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
		log.Error(typeutil.String(res.Body))
		return fmt.Errorf("bad path %q: HTTP %v", path, res.Status)
	}
}
