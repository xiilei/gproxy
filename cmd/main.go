package main

import (
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli"
)

var logger = log.New(os.Stdout, "gproxy ", log.Lshortfile)

func main() {
	app := cli.NewApp()
	app.Name = "gproxy"
	app.HelpName = "gproxy"
	app.Usage = "a simple http/https proxy"
	app.Version = "0.1.0"
	app.Action = func(ctx *cli.Context) error {
		fmt.Println("hello gproxy")
		return nil
	}
	app.Commands = []cli.Command{
		{
			Name:  "cert",
			Usage: "generate local-sign rsa cert",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "cacert", Usage: "specify ca cert file"},
				cli.StringFlag{Name: "cakey", Usage: "specify ca key file"},
				cli.StringSliceFlag{Name: "hosts", Usage: "specify hosts"},
			},
			Action: func(ctx *cli.Context) error {
				name := ctx.Args().First()
				if name == "" {
					logger.Fatalln("must specify a regular name")
				}
				return generateCert(
					ctx.String("cacert"),
					ctx.String("cakey"),
					name, ctx.StringSlice("host"))
			},
			ArgsUsage: "filename",
		},
	}
	app.Run(os.Args)
}
