package main

import (
	"log"
	"net/http"
	"os"

	"github.com/urfave/cli"
	gp "github.com/xiilei/gproxy"
)

var logger = log.New(os.Stdout, "[gproxy] ", log.Ltime)

func main() {
	app := cli.NewApp()
	app.Name = "gproxy"
	app.HelpName = "gproxy"
	app.Usage = "a simple http/https proxy"
	app.Version = "0.1.0"
	app.Action = run
	app.Flags = []cli.Flag{
		cli.StringFlag{Name: "addr", Usage: "listen port", Value: ":8080"},
		cli.StringSliceFlag{Name: "host", Usage: "https host"},
		cli.StringFlag{Name: "cert", Usage: "cert file for https host"},
		cli.StringFlag{Name: "key", Usage: "key file for https host"},
	}
	app.Commands = []cli.Command{
		certCmd,
		pureCmd,
	}
	err := app.Run(os.Args)
	if err != nil {
		logger.Fatal(err)
	}
}

func run(ctx *cli.Context) error {
	proxy := gp.NewProxyHandler()
	if ctx.IsSet("cert") && ctx.IsSet("key") {
		err := proxy.SetCert(ctx.StringSlice("host"),
			ctx.String("cert"),
			ctx.String("key"))
		if err != nil {
			return err
		}
	}
	logger.Printf("listen at %s\n", ctx.String("addr"))
	return http.ListenAndServe(ctx.String("addr"), proxy)
}
