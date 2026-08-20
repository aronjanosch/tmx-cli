package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/alecthomas/kong"
	"github.com/aronjanosch/tmx-cli/internal/auth"
	"github.com/aronjanosch/tmx-cli/internal/client"
	"github.com/aronjanosch/tmx-cli/internal/config"
)

type Context struct {
	Config *config.Config
	JSON   bool
	client *client.Client
}

// Client returns a lazy-initialized HTTP client with stored cookies.
// If credentials are stored (tmx login --save or TMX_EMAIL/TMX_PASSWORD),
// the client re-logs-in automatically once when the session has expired.
func (c *Context) Client() (*client.Client, error) {
	if c.client != nil {
		return c.client, nil
	}
	cookies, err := config.LoadCookies()
	if err != nil {
		return nil, fmt.Errorf("loading cookies: %w", err)
	}
	cl, err := client.New(cookies)
	if err != nil {
		return nil, err
	}
	if creds := config.LoadCredentials(); creds != nil {
		cl.Relogin = func() ([]*http.Cookie, error) {
			fresh, err := auth.Login(creds.Email, creds.Password)
			if err != nil {
				return nil, err
			}
			_ = config.SaveCookies(fresh)
			_ = config.ClearCache("search_token.json")
			return fresh, nil
		}
	}
	c.client = cl
	return cl, nil
}

// PrintJSON marshals v and writes it to stdout.
func (c *Context) PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// PrintError writes a JSON error object to stdout (for agent parsing) or plain text to stderr.
func (c *Context) PrintError(msg string) {
	if c.JSON {
		_ = c.PrintJSON(map[string]string{"error": msg})
	} else {
		fmt.Fprintln(os.Stderr, "error:", msg)
	}
}

type CLI struct {
	Login       LoginCmd       `cmd:"" help:"Log in to Cookidoo."`
	Setup       SetupCmd       `cmd:"" help:"Configure tmx preferences."`
	Search      SearchCmd      `cmd:"" help:"Search recipes."`
	Recipe      RecipeCmd      `cmd:"" help:"Show recipe details."`
	Plan        PlanCmd        `cmd:"" help:"Manage your meal plan."`
	Shopping    ShoppingCmd    `cmd:"" help:"Manage your shopping list."`
	Favorites   FavoritesCmd   `cmd:"" help:"Manage favorite recipes."`
	Collections CollectionsCmd `cmd:"" help:"Browse and search collections."`
	Categories  CategoriesCmd  `cmd:"" help:"Manage recipe categories."`
	Today       TodayCmd       `cmd:"" help:"Show today's planned recipes."`
	Whoami      WhoamiCmd      `cmd:"" help:"Show account and subscription info."`
	Status      StatusCmd      `cmd:"" help:"Show login and config status."`
	Cache       CacheCmd       `cmd:"" help:"Manage local cache."`

	Version kong.VersionFlag `short:"v" name:"version" help:"Print version and exit."`
	JSON    bool             `short:"j" name:"json" help:"Output JSON."`
}
