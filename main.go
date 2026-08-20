package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/aronjanosch/tmx-cli/cmd"
	"github.com/aronjanosch/tmx-cli/internal/client"
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

	if err := ctx.Run(cmdCtx); err != nil {
		cmdCtx.PrintError(err.Error())
		if errors.Is(err, client.ErrUnauthorized) {
			os.Exit(3)
		}
		os.Exit(1)
	}
}
