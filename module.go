package main

import (
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"sort"

	"github.com/ghetzel/go-stockutil/log"
)

const Version = `0.0.2`

type ModuleWalkFunc func(pkg *Package) error

var stop error = errors.New(`quit`)

type Metadata struct {
	Title            string
	GeneratorVersion string
	URL              string
}

type Module struct {
	Metadata    Metadata
	PackageList []PackageSummary
	Package     *Package
}

func (self *Module) Walk(fn ModuleWalkFunc) error {
	if fn != nil {
		return self.walkPackage(self.Package, fn)
	} else {
		return nil
	}
}

// depth-first recursive call-the-function-on-each-package traversing friend.
func (self *Module) walkPackage(current *Package, fn ModuleWalkFunc) error {
	for _, sub := range current.Packages {
		if err := self.walkPackage(sub, fn); err != nil {
			if err == stop {
				return nil
			} else {
				return err
			}
		}
	}

	if err := fn(current); err != nil {
		if err == stop {
			return nil
		} else {
			return err
		}
	}

	return nil
}

func ScanDir(root string) (*Module, error) {
	if root == `` {
		root = `.`
	}

	log.Infof("scanning: %s", root)
	log.Infof("  GOROOT: %s", runtime.GOROOT())

	if pkg, err := LoadPackage(root); err == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent(``, `    `)

		var mod = &Module{
			Metadata: Metadata{
				Title:            pkg.Name,
				URL:              pkg.URL,
				GeneratorVersion: Version,
			},
			Package: pkg,
		}

		if err := mod.Walk(func(pkg *Package) error {
			mod.PackageList = append(mod.PackageList, pkg.PackageSummary)

			sort.Slice(mod.PackageList, func(i int, j int) bool {
				return mod.PackageList[i].ImportPath < mod.PackageList[j].ImportPath
			})

			return nil
		}); err == nil {
			return mod, nil
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}
