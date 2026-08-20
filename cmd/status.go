package cmd

import (
	"fmt"
	"os"

	"github.com/aronjanosch/tmx-cli/internal/config"
)

type StatusCmd struct{}

func (s *StatusCmd) Run(ctx *Context) error {
	cookies, _ := config.LoadCookies()
	loggedIn := len(cookies) > 0

	cfg := ctx.Config
	configDir := config.Dir()

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{
			"logged_in":          loggedIn,
			"auto_relogin":       config.LoadCredentials() != nil,
			"credentials_stored": config.HasCredentialsFile(),
			"config_dir":         configDir,
			"tm_version":         cfg.TMVersion,
			"diet":               cfg.Diet,
			"max_time":           cfg.MaxTime,
		})
	}

	fmt.Println("Status")
	fmt.Println("------")
	if loggedIn {
		fmt.Println("Login:       logged in")
	} else {
		fmt.Println("Login:       not logged in (run: tmx login)")
	}
	if config.LoadCredentials() != nil {
		fmt.Println("Auto-login:  enabled")
	} else {
		fmt.Println("Auto-login:  off (enable: tmx login --save)")
	}
	fmt.Printf("Config dir:  %s\n", configDir)
	if cfg.TMVersion != "" {
		fmt.Printf("TM version:  %s\n", cfg.TMVersion)
	}
	if cfg.Diet != "" {
		fmt.Printf("Diet:        %s\n", cfg.Diet)
	}
	if cfg.MaxTime > 0 {
		fmt.Printf("Max time:    %d min\n", cfg.MaxTime)
	}

	// Plan cache info
	days, err := loadPlanCache()
	if err == nil {
		fmt.Printf("Plan cache:  %d days cached\n", len(days))
	} else {
		fmt.Println("Plan cache:  none")
	}

	// Check for search token cache
	if _, err := os.Stat(config.Dir() + "/search_token.json"); err == nil {
		fmt.Println("Search token: cached")
	} else {
		fmt.Println("Search token: none")
	}

	return nil
}
