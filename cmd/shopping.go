package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aronjanosch/tmx-cli/internal/api"
	"github.com/aronjanosch/tmx-cli/internal/client"
)

type ShoppingCmd struct {
	Show       ShoppingShowCmd       `cmd:"" help:"Show shopping list."`
	Add        ShoppingAddCmd        `cmd:"" help:"Add recipes to shopping list."`
	Remove     ShoppingRemoveCmd     `cmd:"" help:"Remove a recipe from shopping list."`
	AddItem    ShoppingAddItemCmd    `cmd:"add-item" help:"Add custom items to shopping list."`
	EditItem   ShoppingEditItemCmd   `cmd:"edit-item" help:"Rename a custom item."`
	RemoveItem ShoppingRemoveItemCmd `cmd:"remove-item" help:"Remove custom items."`
	Check      ShoppingCheckCmd      `cmd:"" help:"Check off items (mark as owned)."`
	Uncheck    ShoppingUncheckCmd    `cmd:"" help:"Uncheck items (mark as not owned)."`
	FromPlan   ShoppingFromPlanCmd   `cmd:"from-plan" help:"Generate shopping list from meal plan."`
	Clear      ShoppingClearCmd      `cmd:"" help:"Clear the entire shopping list."`
	Export     ShoppingExportCmd     `cmd:"" help:"Export shopping list."`
}

// recipeIDRe matches Vorwerk recipe ids (r130616 or bare 130616), as opposed
// to the 26-char ULIDs used for shopping-list entries and custom recipes.
var recipeIDRe = regexp.MustCompile(`^r?\d+$`)

func fetchShoppingList(cl *client.Client) (*api.ShoppingList, error) {
	raw, err := cl.Get(fmt.Sprintf("/shopping/%s", locale))
	if err != nil {
		return nil, fmt.Errorf("fetching shopping list: %w", err)
	}
	var list api.ShoppingList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parsing shopping list: %w", err)
	}
	return &list, nil
}

// ── Show ─────────────────────────────────────────────────────────────────────

type ShoppingShowCmd struct {
	ByRecipe bool `short:"r" name:"by-recipe" help:"Group by recipe."`
}

// shoppingItemOut is one flat shopping list entry. ID is needed for check/uncheck.
type shoppingItemOut struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Quantity    float64 `json:"quantity,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	Preparation string  `json:"preparation,omitempty"`
	Optional    bool    `json:"optional,omitempty"`
	IsOwned     bool    `json:"isOwned"`
	RecipeID    string  `json:"recipeId"`
}

type shoppingRecipeOut struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	ULID   string `json:"ulid"`
	Custom bool   `json:"custom,omitempty"`
}

type shoppingShowOut struct {
	Items           []shoppingItemOut    `json:"data"`
	Recipes         []shoppingRecipeOut  `json:"recipes"`
	AdditionalItems []api.AdditionalItem `json:"additionalItems"`
	Count           int                  `json:"count"`
}

func buildShoppingOut(list *api.ShoppingList) shoppingShowOut {
	out := shoppingShowOut{Items: []shoppingItemOut{}, Recipes: []shoppingRecipeOut{}, AdditionalItems: list.AdditionalItems}
	if out.AdditionalItems == nil {
		out.AdditionalItems = []api.AdditionalItem{}
	}
	all := list.AllRecipes()
	for _, r := range all {
		out.Recipes = append(out.Recipes, shoppingRecipeOut{ID: r.ID, Title: r.Title, ULID: r.ULID, Custom: r.IsCustomerRecipe})
		for _, ing := range r.Ingredients {
			out.Items = append(out.Items, shoppingItemOut{
				ID:          ing.ID,
				Name:        ing.Name,
				Quantity:    ing.Quantity.Amount(),
				Unit:        ing.Unit,
				Preparation: ing.Preparation,
				Optional:    ing.Optional,
				IsOwned:     ing.IsOwned,
				RecipeID:    r.ID,
			})
		}
	}
	out.Count = len(out.Items) + len(out.AdditionalItems)
	return out
}

func (s *ShoppingShowCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	list, err := fetchShoppingList(cl)
	if err != nil {
		return err
	}

	if ctx.JSON {
		return ctx.PrintJSON(buildShoppingOut(list))
	}

	printShoppingList(list, s.ByRecipe)
	return nil
}

func checkbox(owned bool) string {
	if owned {
		return "[x]"
	}
	return "[ ]"
}

func printShoppingList(list *api.ShoppingList, byRecipe bool) {
	all := list.AllRecipes()
	if byRecipe {
		for _, r := range all {
			fmt.Printf("\n%s (%s)\n", r.Title, r.ID)
			fmt.Println(strings.Repeat("-", len(r.Title)+len(r.ID)+3))
			for _, ing := range r.Ingredients {
				fmt.Printf("  %s %-28s %s\n", checkbox(ing.IsOwned), ing.ID, ingredientLine(ing))
			}
		}
	} else {
		for _, r := range all {
			for _, ing := range r.Ingredients {
				fmt.Printf("  %s %-28s %s\n", checkbox(ing.IsOwned), ing.ID, ingredientLine(ing))
			}
		}
	}
	if len(list.AdditionalItems) > 0 {
		fmt.Println("\nCustom items:")
		for _, item := range list.AdditionalItems {
			fmt.Printf("  %s %-28s %s\n", checkbox(item.IsOwned), item.ID, item.Name)
		}
	}
	fmt.Println()
}

func ingredientLine(ing api.ShoppingIngredient) string {
	parts := []string{}
	if q := ing.Quantity.Amount(); q > 0 {
		if q == float64(int(q)) {
			parts = append(parts, fmt.Sprintf("%d", int(q)))
		} else {
			parts = append(parts, fmt.Sprintf("%.1f", q))
		}
	}
	if ing.Unit != "" {
		parts = append(parts, ing.Unit)
	}
	parts = append(parts, ing.Name)
	if ing.Preparation != "" {
		parts = append(parts, "("+ing.Preparation+")")
	}
	return strings.Join(parts, " ")
}

// ── Add / Remove recipes ─────────────────────────────────────────────────────

type ShoppingAddCmd struct {
	RecipeIDs []string `arg:"" help:"Recipe IDs to add."`
	Custom    bool     `short:"c" help:"IDs are custom (own) recipe IDs."`
}

func (s *ShoppingAddCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	var payload map[string]any
	if s.Custom {
		entries := make([]map[string]string, len(s.RecipeIDs))
		for i, id := range s.RecipeIDs {
			entries[i] = map[string]string{"id": id, "source": "CUSTOMER"}
		}
		payload = map[string]any{"recipeIDs": entries}
	} else {
		ids := make([]string, len(s.RecipeIDs))
		for i, id := range s.RecipeIDs {
			ids[i] = ensurePrefix(id, "r")
		}
		payload = map[string]any{"recipeIDs": ids}
	}

	if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/recipes/add", locale), payload); err != nil {
		return fmt.Errorf("adding recipes: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "added", "ids": s.RecipeIDs})
	}
	fmt.Printf("Added %d recipe(s) to shopping list.\n", len(s.RecipeIDs))
	return nil
}

type ShoppingRemoveCmd struct {
	RecipeID string `arg:"" help:"Recipe ID (rNNN) or shopping-list ULID."`
}

func (s *ShoppingRemoveCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	// The remove endpoint wants the shopping-list entry ULID, not the recipe ID.
	// Resolve rNNN (or bare numeric) ids by looking them up in the current list.
	ulid := s.RecipeID
	if recipeIDRe.MatchString(s.RecipeID) {
		id := ensurePrefix(s.RecipeID, "r")
		list, err := fetchShoppingList(cl)
		if err != nil {
			return err
		}
		ulid = ""
		for _, r := range list.AllRecipes() {
			if r.ID == id {
				ulid = r.ULID
				break
			}
		}
		if ulid == "" {
			return fmt.Errorf("recipe %s not found on shopping list", id)
		}
	}

	if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/recipes/remove", locale), map[string]any{
		"recipeIDs": []string{ulid},
	}); err != nil {
		return fmt.Errorf("removing recipe: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "removed", "id": s.RecipeID})
	}
	fmt.Printf("Removed %s from shopping list.\n", s.RecipeID)
	return nil
}

// ── Custom items ─────────────────────────────────────────────────────────────

type ShoppingAddItemCmd struct {
	Items []string `arg:"" help:"Custom items to add."`
}

func (s *ShoppingAddItemCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	raw, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/additional-items/add", locale), map[string]any{
		"itemsValue": s.Items,
	})
	if err != nil {
		return fmt.Errorf("adding items: %w", err)
	}

	var resp struct {
		Data []api.AdditionalItem `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing add-item response: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "added", "data": resp.Data, "count": len(resp.Data)})
	}
	fmt.Printf("Added %d item(s).\n", len(resp.Data))
	for _, item := range resp.Data {
		fmt.Printf("  %s  %s\n", item.ID, item.Name)
	}
	return nil
}

type ShoppingEditItemCmd struct {
	ID   string `arg:"" help:"Item ID."`
	Name string `arg:"" help:"New name."`
}

func (s *ShoppingEditItemCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/additional-items/edit", locale), map[string]any{
		"additionalItems": []map[string]string{{"id": s.ID, "name": s.Name}},
	}); err != nil {
		return fmt.Errorf("editing item: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "edited", "id": s.ID, "name": s.Name})
	}
	fmt.Printf("Renamed item to %q.\n", s.Name)
	return nil
}

type ShoppingRemoveItemCmd struct {
	IDs []string `arg:"" help:"Item IDs to remove."`
}

func (s *ShoppingRemoveItemCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/additional-items/remove", locale), map[string]any{
		"additionalItemIDs": s.IDs,
	}); err != nil {
		return fmt.Errorf("removing items: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "removed", "ids": s.IDs})
	}
	fmt.Printf("Removed %d item(s).\n", len(s.IDs))
	return nil
}

// ── Check / Uncheck ──────────────────────────────────────────────────────────

type ShoppingCheckCmd struct {
	IDs []string `arg:"" help:"Ingredient or custom item IDs."`
}

func (s *ShoppingCheckCmd) Run(ctx *Context) error {
	return setOwnership(ctx, s.IDs, true)
}

type ShoppingUncheckCmd struct {
	IDs []string `arg:"" help:"Ingredient or custom item IDs."`
}

func (s *ShoppingUncheckCmd) Run(ctx *Context) error {
	return setOwnership(ctx, s.IDs, false)
}

// setOwnership classifies each ID as recipe ingredient or additional item and
// updates ownership via the matching endpoint.
func setOwnership(ctx *Context, ids []string, owned bool) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	list, err := fetchShoppingList(cl)
	if err != nil {
		return err
	}

	isAdditional := map[string]bool{}
	for _, item := range list.AdditionalItems {
		isAdditional[item.ID] = true
	}
	isIngredient := map[string]bool{}
	for _, r := range list.AllRecipes() {
		for _, ing := range r.Ingredients {
			isIngredient[ing.ID] = true
		}
	}

	ts := time.Now().Unix()
	var ingredients, additional []map[string]any
	for _, id := range ids {
		entry := map[string]any{"id": id, "isOwned": owned, "ownedTimestamp": ts}
		switch {
		case isIngredient[id]:
			ingredients = append(ingredients, entry)
		case isAdditional[id]:
			additional = append(additional, entry)
		default:
			return fmt.Errorf("id %s not found on shopping list", id)
		}
	}

	if len(ingredients) > 0 {
		if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/owned-ingredients/ownership/edit", locale), map[string]any{
			"ingredients": ingredients,
		}); err != nil {
			return fmt.Errorf("updating ingredients: %w", err)
		}
	}
	if len(additional) > 0 {
		if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/additional-items/ownership/edit", locale), map[string]any{
			"additionalItems": additional,
		}); err != nil {
			return fmt.Errorf("updating items: %w", err)
		}
	}

	status, label := "unchecked", "Unchecked"
	if owned {
		status, label = "checked", "Checked"
	}
	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": status, "ids": ids})
	}
	fmt.Printf("%s %d item(s).\n", label, len(ids))
	return nil
}

// ── From plan / Clear / Export ───────────────────────────────────────────────

type ShoppingFromPlanCmd struct {
	Days int `short:"d" default:"7" help:"Days from today to include."`
}

func (s *ShoppingFromPlanCmd) Run(ctx *Context) error {
	days, err := loadPlanCache()
	if err != nil {
		return fmt.Errorf("no cached plan — run: tmx plan sync")
	}

	// Only consider today and the next N-1 days, not whatever the cache starts with.
	today := time.Now().Format("2006-01-02")
	until := time.Now().AddDate(0, 0, s.Days-1).Format("2006-01-02")

	var recipeIDs, customIDs []string
	seen := map[string]bool{}
	for _, d := range days {
		if d.Date < today || d.Date > until {
			continue
		}
		for _, r := range d.Recipes {
			if !seen[r.ID] {
				seen[r.ID] = true
				recipeIDs = append(recipeIDs, r.ID)
			}
		}
		for _, id := range d.CustomRecipeIDs {
			if !seen[id] {
				seen[id] = true
				customIDs = append(customIDs, id)
			}
		}
	}

	if len(recipeIDs) == 0 && len(customIDs) == 0 {
		if ctx.JSON {
			return ctx.PrintJSON(map[string]any{"status": "empty", "ids": []string{}})
		}
		fmt.Println("No recipes found in plan for the next", s.Days, "days.")
		return nil
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if len(recipeIDs) > 0 {
		if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/recipes/add", locale), map[string]any{
			"recipeIDs": recipeIDs,
		}); err != nil {
			return fmt.Errorf("adding recipes: %w", err)
		}
	}
	if len(customIDs) > 0 {
		entries := make([]map[string]string, len(customIDs))
		for i, id := range customIDs {
			entries[i] = map[string]string{"id": id, "source": "CUSTOMER"}
		}
		if _, err := cl.PostJSON(fmt.Sprintf("/shopping/%s/recipes/add", locale), map[string]any{
			"recipeIDs": entries,
		}); err != nil {
			return fmt.Errorf("adding custom recipes: %w", err)
		}
	}

	all := append(recipeIDs, customIDs...)
	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "added", "ids": all, "count": len(all)})
	}
	fmt.Printf("Added %d recipe(s) from plan to shopping list.\n", len(all))
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
	list, err := fetchShoppingList(cl)
	if err != nil {
		return err
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

	if s.Format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(buildShoppingOut(list))
	}

	markdown := s.Format == "markdown"
	writeLine := func(owned bool, text string) {
		if markdown {
			mark := " "
			if owned {
				mark = "x"
			}
			fmt.Fprintf(out, "- [%s] %s\n", mark, text)
		} else {
			fmt.Fprintln(out, text)
		}
	}

	if markdown {
		fmt.Fprintln(out, "# Shopping List")
		fmt.Fprintln(out)
	}

	if s.ByRecipe {
		for _, r := range list.AllRecipes() {
			if markdown {
				fmt.Fprintf(out, "## %s\n\n", r.Title)
			} else {
				fmt.Fprintf(out, "%s\n%s\n", r.Title, strings.Repeat("-", len(r.Title)))
			}
			for _, ing := range r.Ingredients {
				writeLine(ing.IsOwned, ingredientLine(ing))
			}
			fmt.Fprintln(out)
		}
	} else {
		// Deduplicate identical lines across recipes (shared staples).
		seen := map[string]bool{}
		for _, r := range list.AllRecipes() {
			for _, ing := range r.Ingredients {
				line := ingredientLine(ing)
				if !seen[line] {
					seen[line] = true
					writeLine(ing.IsOwned, line)
				}
			}
		}
	}

	if len(list.AdditionalItems) > 0 {
		if s.ByRecipe {
			if markdown {
				fmt.Fprintf(out, "## Custom items\n\n")
			} else {
				fmt.Fprintln(out, "Custom items\n------------")
			}
		}
		for _, item := range list.AdditionalItems {
			writeLine(item.IsOwned, item.Name)
		}
	}
	return nil
}
