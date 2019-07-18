package main

import (
	"github.com/urfave/cli"
	gp "github.com/xiilei/gproxy"
)

var pureCmd = cli.Command{
	Name:  "pure",
	Usage: "pure http proxy",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "addr", Usage: "listen port", Value: ":8080"},
	},
	Action: func(ctx *cli.Context) error {
		return pureRun(ctx.String("addr"))
	},
	ArgsUsage: "",
}

func pureRun(addr string) error {
	p := gp.PureProxy{}
	logger.Printf("listen at %s\n", addr)
	return p.ListenAndServe(addr)
}
