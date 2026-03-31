package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/aronjanosch/tmx-cli/cmd"
	"github.com/aronjanosch/tmx-cli/internal/config"
)

var version = "dev"

func main() {
	cli := &cmd.CLI{}

	ctx := kong.Parse(cli,
		kong.Name("tmx"),
		kong.Description("Thermomix / Cookidoo CLI."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	cmdCtx := &cmd.Context{
		Config: cfg,
		JSON:   cli.JSON,
	}

	ctx.FatalIfErrorf(ctx.Run(cmdCtx))
}
