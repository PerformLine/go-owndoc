package main

import (
	"encoding/json"
	"errors"
	"os"
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
	Metadata Metadata
	Package  *Package
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

	if pkg, err := LoadPackage(root); err == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent(``, `    `)

		return &Module{
			Metadata: Metadata{
				Title:            ``,
				URL:              `https://github.com/ghetzel/go-owndoc`,
				GeneratorVersion: Version,
			},
			Package: pkg,
		}, nil
	} else {
		return nil, err
	}
}
