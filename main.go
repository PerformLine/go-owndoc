package main

//go:generate esc -o static.go -pkg main -modtime 1500000000 -prefix assets assets

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ghetzel/cli"
	"github.com/ghetzel/go-stockutil/log"
)

func main() {
	app := cli.NewApp()
	app.Name = `godocfriend`
	app.Usage = `Generate a static site documenting a Golang package and all subpackages`
	app.Version = `0.0.1`

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   `log-level, L`,
			Usage:  `Level of log output verbosity`,
			Value:  `debug`,
			EnvVar: `LOGLEVEL`,
		},
		cli.StringFlag{
			Name:   `output-dir, o`,
			Usage:  `The output directory where generated files will be placed.`,
			Value:  `docs`,
			EnvVar: `GODOCGEN_DIR`,
		},
	}

	app.Before = func(c *cli.Context) error {
		log.SetLevelString(c.String(`log-level`))
		return nil
	}

	app.Action = func(c *cli.Context) {
		root := c.Args().First()
		packages := make([]*Package, 0)

		if root == `` {
			root = `.`
		}

		if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				if p, err := LoadPackage(path); err == nil {
					if p != nil {
						packages = append(packages, p)
					}
				} else {
					return err
				}
			}

			return nil
		}); err == nil {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent(``, `    `)
			enc.Encode(packages)
			// fmt.Println(typeutil.Dump(packages))
		} else {
			log.Fatal(err)
		}
	}

	app.Run(os.Args)
}
