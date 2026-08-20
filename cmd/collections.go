package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aronjanosch/tmx-cli/internal/client"
)

type CollectionsCmd struct {
	Search       CollectionsSearchCmd       `cmd:"" help:"Search public collections."`
	List         CollectionsListCmd         `cmd:"" help:"List your saved collections."`
	Show         CollectionsShowCmd         `cmd:"" help:"Show a collection and its recipes."`
	Mine         CollectionsMineCmd         `cmd:"" help:"List your own recipe lists."`
	Create       CollectionsCreateCmd       `cmd:"" help:"Create an own recipe list."`
	Delete       CollectionsDeleteCmd       `cmd:"" help:"Delete an own recipe list."`
	AddRecipe    CollectionsAddRecipeCmd    `cmd:"add-recipe" help:"Add recipes to an own list."`
	RemoveRecipe CollectionsRemoveRecipeCmd `cmd:"remove-recipe" help:"Remove a recipe from an own list."`
	Save         CollectionsSaveCmd         `cmd:"" help:"Save a public collection to your account."`
	Unsave       CollectionsUnsaveCmd       `cmd:"" help:"Remove a saved public collection."`
}

const (
	customListAccept  = "application/vnd.vorwerk.organize.custom-list.mobile+json"
	managedListAccept = "application/vnd.vorwerk.organize.managed-list.mobile+json"
)

// ── Search ────────────────────────────────────────────────────────────────────

type CollectionsSearchCmd struct {
	Query string `arg:"" optional:"" help:"Search query."`
	Limit int    `short:"n" default:"10" help:"Max results."`
}

type collectionHit struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
}

func (c *CollectionsSearchCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	hits, total, err := algoliaCollections(cl, c.Query, c.Limit)
	if err != nil {
		return err
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"data": hits, "count": len(hits), "total": total})
	}
	if len(hits) == 0 {
		fmt.Println("No collections found.")
		return nil
	}
	fmt.Printf("%-14s  %s\n", "ID", "Title")
	fmt.Printf("%-14s  %s\n", "--", "-----")
	for _, h := range hits {
		title := h.Title
		if len(title) > 58 {
			title = title[:55] + "..."
		}
		fmt.Printf("%-14s  %s\n", h.ID, title)
	}
	fmt.Printf("\n%d of %d results\n", len(hits), total)
	return nil
}

func algoliaCollections(cl *client.Client, query string, limit int) ([]collectionHit, int, error) {
	params := map[string]any{
		"query":       query,
		"hitsPerPage": limit,
		"filters":     "countries:de",
	}
	var result struct {
		Hits   []collectionHit `json:"hits"`
		NbHits int             `json:"nbHits"`
	}
	if err := algoliaQueryAuto(cl, "collections-production-de", params, &result); err != nil {
		return nil, 0, err
	}
	return result.Hits, result.NbHits, nil
}

// ── Managed list helpers ──────────────────────────────────────────────────────

type managedRecipe struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	TotalTime string `json:"totalTime"`
}

type managedChapter struct {
	Title   string          `json:"title"`
	Recipes []managedRecipe `json:"recipes"`
}

type managedList struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Author      string           `json:"author,omitempty"`
	Chapters    []managedChapter `json:"chapters"`
	Modified    string           `json:"modified"`
}

func fetchManagedLists(cl *client.Client) ([]managedList, error) {
	raw, err := cl.Get(fmt.Sprintf("/organize/%s/api/managed-list", locale))
	if err != nil {
		return nil, fmt.Errorf("could not fetch managed lists (are you logged in?): %w", err)
	}
	var data struct {
		Lists []managedList `json:"managedlists"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data.Lists, nil
}

func fetchManagedList(cl *client.Client, id string) (*managedList, error) {
	raw, err := cl.Get(fmt.Sprintf("/organize/%s/api/managed-list/%s", locale, id))
	if err != nil {
		return nil, fmt.Errorf("could not fetch collection (are you logged in?): %w", err)
	}
	var list managedList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// ── Own (custom) lists ────────────────────────────────────────────────────────

type CollectionsMineCmd struct{}

func (c *CollectionsMineCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	raw, err := cl.Request("GET", fmt.Sprintf("/organize/%s/api/custom-list", locale), nil,
		map[string]string{"Accept": customListAccept})
	if err != nil {
		return fmt.Errorf("fetching own lists (are you logged in?): %w", err)
	}
	var data struct {
		Lists []managedList `json:"customlists"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"data": data.Lists, "count": len(data.Lists)})
	}
	if len(data.Lists) == 0 {
		fmt.Println("No own lists found.")
		return nil
	}
	fmt.Printf("%-28s  %-40s  %s\n", "ID", "Title", "Recipes")
	fmt.Printf("%-28s  %-40s  %s\n", "--", "-----", "-------")
	for _, l := range data.Lists {
		n := 0
		for _, ch := range l.Chapters {
			n += len(ch.Recipes)
		}
		fmt.Printf("%-28s  %-40s  %d\n", l.ID, l.Title, n)
	}
	return nil
}

type CollectionsCreateCmd struct {
	Title string `arg:"" help:"List title."`
}

func (c *CollectionsCreateCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	raw, err := cl.Request("POST", fmt.Sprintf("/organize/%s/api/custom-list", locale),
		map[string]string{"title": c.Title},
		map[string]string{"Accept": customListAccept})
	if err != nil {
		return fmt.Errorf("creating list: %w", err)
	}
	var resp struct {
		Content managedList `json:"content"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Content.ID == "" {
		return fmt.Errorf("list created but response had no id: %s", truncateStr(string(raw), 200))
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "created", "id": resp.Content.ID, "title": c.Title})
	}
	fmt.Printf("Created list %q (%s).\n", c.Title, resp.Content.ID)
	return nil
}

type CollectionsDeleteCmd struct {
	ID string `arg:"" help:"List ID."`
}

func (c *CollectionsDeleteCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.Request("DELETE", fmt.Sprintf("/organize/%s/api/custom-list/%s", locale, c.ID), nil,
		map[string]string{"Accept": customListAccept}); err != nil {
		return fmt.Errorf("deleting list: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "deleted", "id": c.ID})
	}
	fmt.Printf("Deleted list %s.\n", c.ID)
	return nil
}

type CollectionsAddRecipeCmd struct {
	ListID    string   `arg:"" help:"List ID."`
	RecipeIDs []string `arg:"" help:"Recipe IDs to add."`
}

func (c *CollectionsAddRecipeCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	ids := make([]string, len(c.RecipeIDs))
	for i, id := range c.RecipeIDs {
		ids[i] = ensurePrefix(id, "r")
	}
	if _, err := cl.Request("PUT", fmt.Sprintf("/organize/%s/api/custom-list/%s", locale, c.ListID),
		map[string]any{"recipeIds": ids},
		map[string]string{"Accept": customListAccept}); err != nil {
		return fmt.Errorf("adding recipes: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"status": "added", "listId": c.ListID, "ids": ids})
	}
	fmt.Printf("Added %d recipe(s) to %s.\n", len(ids), c.ListID)
	return nil
}

type CollectionsRemoveRecipeCmd struct {
	ListID   string `arg:"" help:"List ID."`
	RecipeID string `arg:"" help:"Recipe ID to remove."`
}

func (c *CollectionsRemoveRecipeCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	id := ensurePrefix(c.RecipeID, "r")
	if _, err := cl.Request("DELETE",
		fmt.Sprintf("/organize/%s/api/custom-list/%s/recipes/%s", locale, c.ListID, id), nil,
		map[string]string{"Accept": customListAccept}); err != nil {
		return fmt.Errorf("removing recipe: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "removed", "listId": c.ListID, "id": id})
	}
	fmt.Printf("Removed %s from %s.\n", id, c.ListID)
	return nil
}

type CollectionsSaveCmd struct {
	ID string `arg:"" help:"Public collection ID (e.g. col500561)."`
}

func (c *CollectionsSaveCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.Request("POST", fmt.Sprintf("/organize/%s/api/managed-list", locale),
		map[string]string{"collectionId": c.ID},
		map[string]string{"Accept": managedListAccept}); err != nil {
		return fmt.Errorf("saving collection: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "saved", "id": c.ID})
	}
	fmt.Printf("Saved collection %s.\n", c.ID)
	return nil
}

type CollectionsUnsaveCmd struct {
	ID string `arg:"" help:"Saved collection ID."`
}

func (c *CollectionsUnsaveCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.Request("DELETE", fmt.Sprintf("/organize/%s/api/managed-list/%s", locale, c.ID), nil,
		map[string]string{"Accept": managedListAccept}); err != nil {
		return fmt.Errorf("removing saved collection: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "unsaved", "id": c.ID})
	}
	fmt.Printf("Removed saved collection %s.\n", c.ID)
	return nil
}

// ── List ──────────────────────────────────────────────────────────────────────

type CollectionsListCmd struct{}

func (c *CollectionsListCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	lists, err := fetchManagedLists(cl)
	if err != nil {
		return err
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"data": lists, "count": len(lists)})
	}
	if len(lists) == 0 {
		fmt.Println("No collections found.")
		return nil
	}
	fmt.Printf("%-14s  %-40s  %s\n", "ID", "Title", "Author")
	fmt.Printf("%-14s  %-40s  %s\n", "--", "-----", "------")
	for _, l := range lists {
		title := l.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf("%-14s  %-40s  %s\n", l.ID, title, l.Author)
	}
	return nil
}

// ── Show ──────────────────────────────────────────────────────────────────────

type CollectionsShowCmd struct {
	ID string `arg:"" help:"Collection ID (e.g. col500561)."`
}

func (c *CollectionsShowCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	found, err := fetchManagedList(cl, c.ID)
	if err != nil {
		return err
	}

	if ctx.JSON {
		return ctx.PrintJSON(found)
	}

	width := max(44, len(found.Title)+4)
	fmt.Println()
	fmt.Println("+" + strings.Repeat("-", width) + "+")
	fmt.Printf("|  %-*s|\n", width-2, found.Title)
	fmt.Println("+" + strings.Repeat("-", width) + "+")
	if found.Author != "" {
		fmt.Printf("Author:  %s\n", found.Author)
	}
	if found.Description != "" {
		fmt.Println()
		fmt.Println(wordWrap(found.Description, width+2))
	}

	for _, ch := range found.Chapters {
		fmt.Println()
		if ch.Title != "" {
			fmt.Printf("── %s\n", ch.Title)
		}
		fmt.Printf("%-12s  %-48s  %s\n", "ID", "Title", "Time")
		fmt.Printf("%-12s  %-48s  %s\n", "----", "-----", "----")
		for _, r := range ch.Recipes {
			title := r.Title
			if len(title) > 48 {
				title = title[:45] + "..."
			}
			var secs float64
			fmt.Sscanf(r.TotalTime, "%f", &secs)
			fmt.Printf("%-12s  %-48s  %s\n", r.ID, title, formatTime(int(secs)))
		}
	}
	fmt.Println()
	return nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func wordWrap(s string, width int) string {
	words := strings.Fields(s)
	var lines []string
	line := ""
	for _, w := range words {
		if len(line)+len(w)+1 > width {
			if line != "" {
				lines = append(lines, line)
			}
			line = w
		} else {
			if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
