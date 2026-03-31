package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ShoppingCmd struct {
	Show     ShoppingShowCmd     `cmd:"" help:"Show shopping list."`
	Add      ShoppingAddCmd      `cmd:"" help:"Add recipes to shopping list."`
	AddItem  ShoppingAddItemCmd  `cmd:"add-item" help:"Add a custom item to shopping list."`
	FromPlan ShoppingFromPlanCmd `cmd:"from-plan" help:"Generate shopping list from meal plan."`
	Remove   ShoppingRemoveCmd   `cmd:"" help:"Remove a recipe from shopping list."`
	Clear    ShoppingClearCmd    `cmd:"" help:"Clear the entire shopping list."`
	Export   ShoppingExportCmd   `cmd:"" help:"Export shopping list."`
}

type ShoppingShowCmd struct {
	ByRecipe bool `short:"r" name:"by-recipe" help:"Group by recipe."`
}

func (s *ShoppingShowCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	raw, err := cl.Get(fmt.Sprintf("/shopping/%s", locale))
	if err != nil {
		return fmt.Errorf("fetching shopping list: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing shopping list: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(buildShoppingJSON(data))
	}

	printShoppingList(data, s.ByRecipe)
	return nil
}

type ShoppingAddCmd struct {
	RecipeIDs []string `arg:"" help:"Recipe IDs to add."`
}

func (s *ShoppingAddCmd) Run(ctx *Context) error {
	ids := make([]string, len(s.RecipeIDs))
	for i, id := range s.RecipeIDs {
		ids[i] = ensurePrefix(id, "r")
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	_, err = cl.PostJSON(fmt.Sprintf("/shopping/%s/add-recipes", locale), map[string]any{
		"recipeIDs": ids,
	})
	if err != nil {
		return fmt.Errorf("adding recipes: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "added", "ids": ids})
	}
	fmt.Printf("Added %d recipe(s) to shopping list.\n", len(ids))
	return nil
}

type ShoppingAddItemCmd struct {
	Items []string `arg:"" help:"Custom items to add."`
}

func (s *ShoppingAddItemCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	for _, item := range s.Items {
		if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/additional-item", locale), map[string]string{
			"itemValue": item,
		}); err != nil {
			return fmt.Errorf("adding item %q: %w", item, err)
		}
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "added", "items": s.Items})
	}
	fmt.Printf("Added %d item(s).\n", len(s.Items))
	return nil
}

type ShoppingFromPlanCmd struct {
	Days int `short:"d" default:"7" help:"Days from today to include."`
}

func (s *ShoppingFromPlanCmd) Run(ctx *Context) error {
	days, err := loadPlanCache()
	if err != nil {
		return fmt.Errorf("no cached plan — run: tmx plan sync")
	}

	var recipeIDs []string
	seen := map[string]bool{}
	count := 0
	for _, d := range days {
		if count >= s.Days {
			break
		}
		for _, r := range d.Recipes {
			if !seen[r.ID] {
				seen[r.ID] = true
				recipeIDs = append(recipeIDs, r.ID)
			}
		}
		count++
	}

	if len(recipeIDs) == 0 {
		fmt.Println("No recipes found in plan for the next", s.Days, "days.")
		return nil
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/add-recipes", locale), map[string]any{
		"recipeIDs": recipeIDs,
	}); err != nil {
		return fmt.Errorf("adding recipes: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "added", "ids": recipeIDs})
	}
	fmt.Printf("Added %d recipe(s) from plan to shopping list.\n", len(recipeIDs))
	return nil
}

type ShoppingRemoveCmd struct {
	RecipeID string `arg:"" help:"Recipe ID to remove."`
}

func (s *ShoppingRemoveCmd) Run(ctx *Context) error {
	id := ensurePrefix(s.RecipeID, "r")
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/recipe/%s/remove", locale, id), nil); err != nil {
		return fmt.Errorf("removing recipe: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "removed", "id": id})
	}
	fmt.Printf("Removed %s from shopping list.\n", id)
	return nil
}

type ShoppingClearCmd struct{}

func (s *ShoppingClearCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if err := cl.Delete(fmt.Sprintf("/shopping/%s", locale)); err != nil {
		return fmt.Errorf("clearing shopping list: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "cleared"})
	}
	fmt.Println("Shopping list cleared.")
	return nil
}

type ShoppingExportCmd struct {
	Format   string `short:"f" default:"text" help:"Format: text|markdown|json."`
	ByRecipe bool   `short:"r" name:"by-recipe" help:"Group by recipe."`
	Output   string `short:"o" help:"Output file (default: stdout)."`
}

func (s *ShoppingExportCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	raw, err := cl.Get(fmt.Sprintf("/shopping/%s", locale))
	if err != nil {
		return fmt.Errorf("fetching shopping list: %w", err)
	}

	out := os.Stdout
	if s.Output != "" {
		f, err := os.Create(s.Output)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing shopping list: %w", err)
	}

	switch s.Format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(buildShoppingJSON(data))
	case "markdown":
		fmt.Fprintln(out, "# Shopping List")
		fmt.Fprintln(out)
		for _, item := range flatIngredients(data) {
			fmt.Fprintf(out, "- [ ] %s\n", item)
		}
	default:
		for _, item := range flatIngredients(data) {
			fmt.Fprintln(out, item)
		}
	}
	return nil
}

type shoppingItemJSON struct {
	Name        string   `json:"name"`
	Quantity    float64  `json:"quantity,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	Preparation string   `json:"preparation,omitempty"`
	Recipes     []string `json:"recipes,omitempty"`
}

type shoppingRecipeJSON struct {
	ID    string              `json:"id"`
	Title string              `json:"title"`
	Items []shoppingItemJSON  `json:"items"`
}

type shoppingListJSON struct {
	Items           []shoppingItemJSON   `json:"data"`
	Recipes         []shoppingRecipeJSON `json:"recipes"`
	AdditionalItems []string             `json:"additional_items,omitempty"`
	Count           int                  `json:"count"`
}

func buildShoppingJSON(data map[string]any) shoppingListJSON {
	result := shoppingListJSON{}

	// Per-recipe grouping
	seenItem := map[string]int{} // name -> index in Items

	for _, r := range asList(data["recipes"]) {
		rm, _ := r.(map[string]any)
		id, _ := rm["id"].(string)
		title, _ := rm["title"].(string)

		recipeEntry := shoppingRecipeJSON{ID: id, Title: title}

		for _, ing := range asList(rm["recipeIngredientGroups"]) {
			im, _ := ing.(map[string]any)
			name, _ := im["ingredientNotation"].(string)
			unit, _ := im["unitNotation"].(string)
			prep, _ := im["preparation"].(string)
			var qty float64
			if qMap, ok := im["quantity"].(map[string]any); ok {
				qty, _ = qMap["value"].(float64)
			}
			item := shoppingItemJSON{Name: name, Quantity: qty, Unit: unit, Preparation: prep, Recipes: []string{id}}
			recipeEntry.Items = append(recipeEntry.Items, item)

			// Merge into flat list
			if idx, ok := seenItem[name]; ok {
				result.Items[idx].Quantity += qty
				result.Items[idx].Recipes = append(result.Items[idx].Recipes, id)
			} else {
				seenItem[name] = len(result.Items)
				flat := shoppingItemJSON{Name: name, Quantity: qty, Unit: unit, Preparation: prep, Recipes: []string{id}}
				result.Items = append(result.Items, flat)
			}
		}

		result.Recipes = append(result.Recipes, recipeEntry)
	}

	// Additional custom items
	for _, a := range asList(data["additionalItems"]) {
		am, _ := a.(map[string]any)
		if name, _ := am["itemValue"].(string); name != "" {
			result.AdditionalItems = append(result.AdditionalItems, name)
		}
	}

	result.Count = len(result.Items) + len(result.AdditionalItems)
	return result
}

func printShoppingList(data map[string]any, byRecipe bool) {
	if byRecipe {
		for _, r := range asList(data["recipes"]) {
			rm, _ := r.(map[string]any)
			title, _ := rm["title"].(string)
			fmt.Printf("\n%s\n", title)
			fmt.Println(strings.Repeat("-", len(title)))
			for _, ing := range asList(rm["recipeIngredientGroups"]) {
				im, _ := ing.(map[string]any)
				fmt.Printf("  %s\n", ingredientLine(im))
			}
		}
	} else {
		for _, item := range flatIngredients(data) {
			fmt.Println(" ", item)
		}
	}
	fmt.Println()
}

func flatIngredients(data map[string]any) []string {
	var items []string
	seen := map[string]bool{}
	recipes, _ := data["recipes"].([]any)
	for _, r := range recipes {
		rm, _ := r.(map[string]any)
		// Shopping list uses recipeIngredientGroups (flat per-ingredient list)
		for _, ing := range asList(rm["recipeIngredientGroups"]) {
			im, _ := ing.(map[string]any)
			line := ingredientLine(im)
			if line != "" && !seen[line] {
				seen[line] = true
				items = append(items, line)
			}
		}
	}
	// Additional custom items
	for _, a := range asList(data["additionalItems"]) {
		am, _ := a.(map[string]any)
		name, _ := am["itemValue"].(string)
		if name != "" && !seen[name] {
			seen[name] = true
			items = append(items, name)
		}
	}
	return items
}

func ingredientLine(m map[string]any) string {
	name, _ := m["ingredientNotation"].(string)
	unit, _ := m["unitNotation"].(string)
	prep, _ := m["preparation"].(string)

	qty := ""
	if qMap, ok := m["quantity"].(map[string]any); ok {
		if v, ok := qMap["value"].(float64); ok && v > 0 {
			if v == float64(int(v)) {
				qty = fmt.Sprintf("%d", int(v))
			} else {
				qty = fmt.Sprintf("%.1f", v)
			}
		}
	}

	parts := []string{}
	if qty != "" {
		parts = append(parts, qty)
	}
	if unit != "" {
		parts = append(parts, unit)
	}
	parts = append(parts, name)
	if prep != "" {
		parts = append(parts, "("+prep+")")
	}
	return strings.Join(parts, " ")
}
