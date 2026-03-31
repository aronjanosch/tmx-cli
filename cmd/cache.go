package cmd

import (
	"fmt"

	"github.com/aronjanosch/tmx-cli/internal/config"
)

type CacheCmd struct {
	Clear CacheClearCmd `cmd:"" help:"Clear local cache."`
}

type CacheClearCmd struct {
	All bool `short:"a" help:"Also clear session cookies (requires re-login)."`
}

func (c *CacheClearCmd) Run(ctx *Context) error {
	files := []string{"weekplan.json", "search_token.json", "categories.json"}
	for _, f := range files {
		_ = config.ClearCache(f)
	}

	if c.All {
		if err := config.ClearCookies(); err != nil {
			return fmt.Errorf("clearing cookies: %w", err)
		}
		fmt.Println("Cache and session cleared.")
		return nil
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "cleared"})
	}
	fmt.Println("Cache cleared.")
	return nil
}
