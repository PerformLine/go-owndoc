package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/mcuadros/go-defaults"
)

const Version = `0.0.5`

type ModuleWalkFunc func(pkg *Package) error

var stop error = errors.New(`quit`)

type ScanOptions struct {
	Version          string
	StartDir         string `default:"."`
	VersionConstName string `default:"Version"`
}

type Metadata struct {
	Title            string
	Version          string
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

func ScanDir(options *ScanOptions) (*Module, error) {
	if options == nil {
		options = new(ScanOptions)
	}

	defaults.SetDefaults(options)

	if pkg, err := LoadPackage(options.StartDir); err == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent(``, `    `)

		var mod = &Module{
			Metadata: Metadata{
				Title:            filepath.Base(pkg.URL),
				URL:              pkg.URL,
				GeneratorVersion: Version,
			},
			Package: pkg,
		}

		if options.Version == `` {
			for _, c := range pkg.Constants {
				if c.Name == options.VersionConstName {
					mod.Metadata.Version = c.Value
					break
				}
			}
		} else {
			mod.Metadata.Version = options.Version
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
