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
	}
	app.Commands = []cli.Command{
		certCmd,
	}
	app.Run(os.Args)
}

func run(ctx *cli.Context) error {
	proxy := gp.NewProxyHandler()
	logger.Printf("listen at %s\n", ctx.String("addr"))
	return http.ListenAndServe(ctx.String("addr"), proxy)
}
