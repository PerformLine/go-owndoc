package main

//go:generate esc -o static.go -pkg main -modtime 1500000000 -prefix assets assets

import (
	"encoding/json"
	"os"

	"github.com/ghetzel/cli"
	"github.com/ghetzel/go-stockutil/log"
)

func main() {
	app := cli.NewApp()
	app.Name = `owndoc`
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

	app.Commands = []cli.Command{
		{
			Name:  `generate`,
			Usage: `Generate a JSON manifest decribing the current package and all subpackages.`,
			Flags: []cli.Flag{},
			Action: func(c *cli.Context) {
				root := c.Args().First()
				if root == `` {
					root = `.`
				}

				if pkg, err := LoadPackage(root); err == nil {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent(``, `    `)

					enc.Encode(&Module{
						Metadata: Metadata{
							Title:            ``,
							URL:              `https://github.com/ghetzel/go-owndoc`,
							GeneratorVersion: app.Version,
						},
						Package: pkg,
					})
				} else {
					log.Fatal(err)
				}
			},
		},
	}

	app.Run(os.Args)
}
