package cmd

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/aronjanosch/tmx-cli/internal/api"
)

type FavoritesCmd struct {
	Show   FavoritesShowCmd   `cmd:"" help:"List favorite recipes."`
	Add    FavoritesAddCmd    `cmd:"" help:"Add a recipe to favorites."`
	Remove FavoritesRemoveCmd `cmd:"" help:"Remove a recipe from favorites."`
}

type FavoritesShowCmd struct{}

func (f *FavoritesShowCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	raw, err := cl.Get(fmt.Sprintf("/organize/%s/my-recipes", locale))
	if err != nil {
		return fmt.Errorf("fetching favorites: %w", err)
	}

	recipes := parseFavoritesHTML(string(raw))

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"data": recipes, "count": len(recipes)})
	}

	if len(recipes) == 0 {
		fmt.Println("No favorites found.")
		return nil
	}

	fmt.Printf("%-12s  %s\n", "ID", "Title")
	fmt.Printf("%-12s  %s\n", "----", "-----")
	for _, r := range recipes {
		fmt.Printf("%-12s  %s\n", r.ID, r.Title)
	}
	return nil
}

type FavoritesAddCmd struct {
	RecipeID string `arg:"" help:"Recipe ID."`
}

func (f *FavoritesAddCmd) Run(ctx *Context) error {
	id := ensurePrefix(f.RecipeID, "r")
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	vals := url.Values{
		"_method":  {"put"},
		"recipeId": {id},
	}
	if _, err := cl.PostForm(fmt.Sprintf("/organize/%s/api/bookmark", locale), vals); err != nil {
		return fmt.Errorf("adding favorite: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "added", "id": id})
	}
	fmt.Printf("Added %s to favorites.\n", id)
	return nil
}

type FavoritesRemoveCmd struct {
	RecipeID string `arg:"" help:"Recipe ID."`
}

func (f *FavoritesRemoveCmd) Run(ctx *Context) error {
	id := ensurePrefix(f.RecipeID, "r")
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	vals := url.Values{
		"_method":  {"delete"},
		"recipeId": {id},
	}
	if _, err := cl.PostForm(fmt.Sprintf("/organize/%s/api/bookmark", locale), vals); err != nil {
		return fmt.Errorf("removing favorite: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "removed", "id": id})
	}
	fmt.Printf("Removed %s from favorites.\n", id)
	return nil
}

var favTileRe = regexp.MustCompile(`(?s)<core-tile\s+[^>]*data-recipe-id="([^"]+)"[^>]*>(.*?)</core-tile>`)

func parseFavoritesHTML(html string) []api.PlanRecipe {
	var recipes []api.PlanRecipe
	for _, m := range favTileRe.FindAllStringSubmatch(html, -1) {
		recipeID := m[1]
		block := m[2]
		title := ""
		if tn := tileTitleRe.FindStringSubmatch(block); len(tn) > 1 {
			title = strings.TrimSpace(tn[1])
		}
		recipes = append(recipes, api.PlanRecipe{
			ID:    recipeID,
			Title: title,
			URL:   fmt.Sprintf("%s/recipes/recipe/%s/%s", cookidooBase, locale, recipeID),
		})
	}
	return recipes
}
