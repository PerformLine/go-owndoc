package main

//go:generate esc -o static.go -pkg main -modtime 1500000000 -prefix assets assets

import (
	"encoding/json"
	"os"

	"github.com/ghetzel/cli"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
)

func main() {
	app := cli.NewApp()
	app.Name = `owndoc`
	app.Usage = `Generate a static site documenting a Golang package and all subpackages`
	app.Version = Version

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   `log-level, L`,
			Usage:  `Level of log output verbosity`,
			Value:  `debug`,
			EnvVar: `LOGLEVEL`,
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
				if mod, err := ScanDir(c.Args().First()); err == nil {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent(``, `    `)
					enc.Encode(mod)
				} else {
					log.Fatal(err)
				}
			},
		}, {
			Name:  `render`,
			Usage: `Render a module's documentation as a standalone static site.`,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   `output-dir, o`,
					Usage:  `The output directory where generated files will be placed.`,
					Value:  `docs`,
					EnvVar: `OWNDOC_DIR`,
				},
				cli.StringSliceFlag{
					Name:  `property, p`,
					Usage: `A key=value pair to expose to all page generation templates.`,
				},
			},
			Action: func(c *cli.Context) {
				if mod, err := ScanDir(c.Args().First()); err == nil {
					var props = maputil.M(nil)

					for _, pair := range c.StringSlice(`property`) {
						k, v := stringutil.SplitPair(pair, `=`)
						props.Set(k, typeutil.Auto(v))
					}

					log.FatalIf(
						RenderHTML(mod, &RenderOptions{
							TargetDir:  c.String(`output-dir`),
							Properties: props.MapNative(),
						}),
					)
				} else {
					log.Fatal(err)
				}
			},
		},
	}

	app.Run(os.Args)
}
