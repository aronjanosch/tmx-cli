package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type SetupCmd struct {
	Reset   bool   `help:"Clear all configuration."`
	TM      string `name:"tm" help:"Thermomix version (TM5|TM6|TM7). Skips interactive prompt."`
	Diet    string `name:"diet" help:"Diet preference (vegetarisch|vegan|none). Skips interactive prompt."`
	MaxTime int    `name:"max-time" help:"Max cooking time in minutes (0 = no limit). Skips interactive prompt."`
}

func (s *SetupCmd) Run(ctx *Context) error {
	if s.Reset {
		if err := ctx.Config.Reset(); err != nil {
			return err
		}
		if ctx.JSON {
			return ctx.PrintJSON(map[string]string{"status": "cleared"})
		}
		fmt.Println("Configuration cleared.")
		return nil
	}

	// Non-interactive mode: any flag was explicitly provided
	if s.TM != "" || s.Diet != "" || s.MaxTime != 0 {
		if s.TM != "" {
			ctx.Config.TMVersion = s.TM
		}
		if s.Diet != "" {
			if s.Diet == "none" {
				ctx.Config.Diet = ""
			} else {
				ctx.Config.Diet = s.Diet
			}
		}
		if s.MaxTime != 0 {
			ctx.Config.MaxTime = s.MaxTime
		}
		if err := ctx.Config.Save(); err != nil {
			return err
		}
		if ctx.JSON {
			return ctx.PrintJSON(map[string]any{
				"status":   "saved",
				"tm":       ctx.Config.TMVersion,
				"diet":     ctx.Config.Diet,
				"max_time": ctx.Config.MaxTime,
			})
		}
		fmt.Println("Configuration saved.")
		return nil
	}

	scanner := bufio.NewScanner(os.Stdin)

	ctx.Config.TMVersion = prompt(scanner, "Thermomix version [TM5/TM6/TM7]", ctx.Config.TMVersion)
	ctx.Config.Diet = prompt(scanner, "Diet preference [vegetarisch/vegan/none]", ctx.Config.Diet)

	maxTimeStr := ""
	if ctx.Config.MaxTime > 0 {
		maxTimeStr = strconv.Itoa(ctx.Config.MaxTime)
	}
	raw := prompt(scanner, "Max cooking time in minutes [15/30/45/60/none]", maxTimeStr)
	if n, err := strconv.Atoi(raw); err == nil {
		ctx.Config.MaxTime = n
	} else {
		ctx.Config.MaxTime = 0
	}

	if err := ctx.Config.Save(); err != nil {
		return err
	}

	fmt.Println("Configuration saved.")
	return nil
}

func prompt(scanner *bufio.Scanner, label, current string) string {
	if current != "" {
		fmt.Printf("%s (current: %s): ", label, current)
	} else {
		fmt.Printf("%s: ", label)
	}
	scanner.Scan()
	raw := strings.TrimSpace(scanner.Text())
	if raw == "" || raw == "none" {
		return ""
	}
	return raw
}
