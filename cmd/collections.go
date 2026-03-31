package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aronjanosch/tmx-cli/internal/client"
)

type CollectionsCmd struct {
	Search CollectionsSearchCmd `cmd:"" help:"Search public collections."`
	List   CollectionsListCmd   `cmd:"" help:"List your saved collections."`
	Show   CollectionsShowCmd   `cmd:"" help:"Show a collection and its recipes."`
}

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
	token, err := getSearchToken(cl)
	if err != nil {
		return fmt.Errorf("could not get search token (are you logged in?): %w", err)
	}

	hits, total, err := algoliaCollections(token, c.Query, c.Limit)
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

func algoliaCollections(token, query string, limit int) ([]collectionHit, int, error) {
	params := map[string]any{
		"query":       query,
		"hitsPerPage": limit,
		"filters":     "countries:de",
	}
	body, _ := json.Marshal(params)
	url := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/collections-production-de/query", algoliaAppID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-Algolia-Application-Id", algoliaAppID)
	req.Header.Set("X-Algolia-API-Key", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result struct {
		Hits   []map[string]any `json:"hits"`
		NbHits int              `json:"nbHits"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, 0, err
	}

	hits := make([]collectionHit, 0, len(result.Hits))
	for _, h := range result.Hits {
		id, _ := h["id"].(string)
		title, _ := h["title"].(string)
		desc, _ := h["description"].(string)
		image, _ := h["image"].(string)
		published, _ := h["publishedAt"].(string)
		hits = append(hits, collectionHit{ID: id, Title: title, Description: desc, Image: image, PublishedAt: published})
	}
	return hits, result.NbHits, nil
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
	url := fmt.Sprintf("https://cookidoo.de/organize/%s/api/managed-list", locale)
	raw, err := cl.GetURL(url)
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
	url := fmt.Sprintf("https://cookidoo.de/organize/%s/api/managed-list/%s", locale, id)
	raw, err := cl.GetURL(url)
	if err != nil {
		return nil, fmt.Errorf("could not fetch collection (are you logged in?): %w", err)
	}
	var list managedList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return &list, nil
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
