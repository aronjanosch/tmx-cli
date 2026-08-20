package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aronjanosch/tmx-cli/internal/api"
)

type RecipeCmd struct {
	Show   RecipeShowCmd   `cmd:"" help:"Show recipe details."`
	Fetch  RecipeFetchCmd  `cmd:"" help:"Scrape a recipe website into import JSON (schema.org)."`
	Import RecipeImportCmd `cmd:"" help:"Import a recipe to your Cookidoo custom recipes."`
	Mine   RecipeMineCmd   `cmd:"" help:"List your custom (own) recipes."`
	Delete RecipeDeleteCmd `cmd:"" help:"Delete a custom recipe."`
	Copy   RecipeCopyCmd   `cmd:"" help:"Copy a Cookidoo recipe to your own recipes."`
}

type RecipeShowCmd struct {
	IDs         []string `arg:"" help:"Recipe ID(s), e.g. r130616. Multiple IDs fetch in one call."`
	Ingredients bool     `short:"i" help:"Include ingredients."`
	Steps       bool     `short:"s" help:"Include preparation steps."`
	Nutrition   bool     `short:"n" help:"Include nutrition per serving."`
	Full        bool     `short:"f" help:"Include all sections (ingredients, steps, nutrition)."`
	Raw         bool     `help:"Output the unmodified API response (single ID only)."`
}

func (r *RecipeShowCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	sections := recipeSections{
		Ingredients: r.Ingredients || r.Full,
		Steps:       r.Steps || r.Full,
		Nutrition:   r.Nutrition || r.Full,
	}

	if r.Raw {
		if len(r.IDs) != 1 {
			return fmt.Errorf("--raw supports exactly one recipe ID")
		}
		raw, err := cl.Get(fmt.Sprintf("/recipes/recipe/%s/%s", locale, ensurePrefix(r.IDs[0], "r")))
		if err != nil {
			return fmt.Errorf("fetching recipe: %w", err)
		}
		_, err = os.Stdout.Write(append(raw, '\n'))
		return err
	}

	details := make([]*api.RecipeDetail, 0, len(r.IDs))
	for _, id := range r.IDs {
		id = ensurePrefix(id, "r")
		raw, err := cl.Get(fmt.Sprintf("/recipes/recipe/%s/%s", locale, id))
		if err != nil {
			return fmt.Errorf("fetching recipe %s: %w", id, err)
		}
		detail, err := api.ParseRecipeDetail(raw, cookidooBase, locale)
		if err != nil {
			return fmt.Errorf("recipe %s: %w", id, err)
		}
		if !sections.Ingredients {
			detail.Ingredients = nil
		}
		if !sections.Steps {
			detail.Steps = nil
		}
		if !sections.Nutrition {
			detail.Nutrition = nil
		}
		details = append(details, detail)
	}

	if ctx.JSON {
		if len(details) == 1 {
			return ctx.PrintJSON(details[0])
		}
		return ctx.PrintJSON(map[string]any{"data": details, "count": len(details)})
	}

	for _, d := range details {
		printRecipe(d, sections)
	}
	return nil
}

type recipeSections struct {
	Ingredients bool
	Steps       bool
	Nutrition   bool
}

func printRecipe(d *api.RecipeDetail, show recipeSections) {
	width := max(44, len(d.Title)+4)
	fmt.Println()
	fmt.Println("+" + strings.Repeat("-", width) + "+")
	fmt.Printf("|  %-*s|\n", width-2, d.Title)
	fmt.Println("+" + strings.Repeat("-", width) + "+")
	fmt.Println()

	fmt.Printf("ID:          %s\n", d.ID)
	if d.TotalTimeMinutes > 0 {
		fmt.Printf("Time:        %s", formatTime(d.TotalTimeMinutes*60))
		if d.ActiveTimeMinutes > 0 {
			fmt.Printf(" (active: %s)", formatTime(d.ActiveTimeMinutes*60))
		}
		fmt.Println()
	}
	if d.Servings > 0 {
		fmt.Printf("Servings:    %d\n", d.Servings)
	}
	if d.Difficulty != "" {
		fmt.Printf("Difficulty:  %s\n", d.Difficulty)
	}
	if len(d.TMVersions) > 0 {
		fmt.Printf("Versions:    %s\n", strings.Join(d.TMVersions, ", "))
	}
	if len(d.Categories) > 0 {
		fmt.Printf("Categories:  %s\n", strings.Join(d.Categories, ", "))
	}

	if show.Ingredients && len(d.Ingredients) > 0 {
		fmt.Println()
		fmt.Println("INGREDIENTS")
		fmt.Println(strings.Repeat("-", 40))
		lastGroup := ""
		for _, ing := range d.Ingredients {
			if ing.Group != "" && ing.Group != lastGroup {
				fmt.Printf("\n%s:\n", ing.Group)
			}
			lastGroup = ing.Group

			line := "  "
			if ing.Quantity > 0 {
				if ing.Quantity == float64(int(ing.Quantity)) {
					line += fmt.Sprintf("%d ", int(ing.Quantity))
				} else {
					line += fmt.Sprintf("%.1f ", ing.Quantity)
				}
			}
			if ing.Unit != "" {
				line += ing.Unit + " "
			}
			line += ing.Name
			if ing.Preparation != "" {
				line += ", " + ing.Preparation
			}
			if ing.Optional {
				line += " (optional)"
			}
			fmt.Println(line)
		}
	}

	if show.Steps && len(d.Steps) > 0 {
		fmt.Println()
		fmt.Println("PREPARATION")
		fmt.Println(strings.Repeat("-", 40))
		lastGroup := ""
		for i, s := range d.Steps {
			if s.Group != "" && s.Group != lastGroup {
				fmt.Printf("\n%s:\n", s.Group)
			}
			lastGroup = s.Group
			fmt.Printf("  %d. %s\n", i+1, s.Text)
		}
	}

	if show.Nutrition && len(d.Nutrition) > 0 {
		fmt.Println()
		fmt.Println("NUTRITION (per serving)")
		fmt.Println(strings.Repeat("-", 40))
		order := []string{"kcal", "kJ", "protein", "carb", "carb2", "fat", "dietaryFibre"}
		printed := map[string]bool{}
		for _, k := range order {
			if v, ok := d.Nutrition[k]; ok {
				fmt.Printf("  %-14s %s\n", nutritionLabel(k)+":", v)
				printed[k] = true
			}
		}
		for k, v := range d.Nutrition {
			if !printed[k] {
				fmt.Printf("  %-14s %s\n", k+":", v)
			}
		}
	}

	fmt.Println()
}

func nutritionLabel(key string) string {
	labels := map[string]string{
		"kcal":         "Calories",
		"kJ":           "Energy (kJ)",
		"protein":      "Protein",
		"carb":         "Carbs",
		"carb2":        "Carbs",
		"fat":          "Fat",
		"dietaryFibre": "Fiber",
	}
	if l, ok := labels[key]; ok {
		return l
	}
	return key
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
