package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aronjanosch/tmx-cli/internal/client"
	"github.com/aronjanosch/tmx-cli/internal/config"
)

type CategoriesCmd struct {
	Show CategoriesShowCmd `cmd:"" help:"List available categories."`
	Sync CategoriesSyncCmd `cmd:"" help:"Sync categories from Cookidoo (makes many API calls)."`
}

type CategoriesShowCmd struct{}

func (c *CategoriesShowCmd) Run(ctx *Context) error {
	cats, fromCache := loadCategoriesCache()

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{
			"categories": cats,
			"from_cache": fromCache,
			"count":      len(cats),
		})
	}

	keys := make([]string, 0, len(cats))
	for k := range cats {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	source := "hardcoded"
	if fromCache {
		source = "cached"
	}
	fmt.Printf("%-24s  %s  (%s)\n", "Category", "ID", source)
	fmt.Printf("%-24s  %s\n", "--------", "--")
	for _, k := range keys {
		fmt.Printf("%-24s  %s\n", k, cats[k])
	}
	return nil
}

type categoryCacheFile struct {
	Timestamp  string            `json:"timestamp"`
	Categories map[string]string `json:"categories"`
}

func loadCategoriesCache() (map[string]string, bool) {
	var f categoryCacheFile
	if err := config.LoadCache("categories.json", &f); err == nil && len(f.Categories) > 0 {
		return f.Categories, true
	}
	return categoryFallback, false
}

type CategoriesSyncCmd struct{}

func (c *CategoriesSyncCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	if !ctx.JSON {
		fmt.Println("Fetching category IDs from Algolia...")
	}

	categoryIDs, err := fetchCategoryFacets(cl)
	if err != nil {
		return fmt.Errorf("fetching category IDs: %w", err)
	}

	if !ctx.JSON {
		fmt.Printf("Found %d categories — fetching names...\n", len(categoryIDs))
	}

	categories := map[string]string{}
	var errors []string

	for i, catID := range categoryIDs {
		if !ctx.JSON {
			fmt.Printf("  [%d/%d] %s\r", i+1, len(categoryIDs), catID)
		}

		recipeID, err := searchOneByCategoryID(cl, catID)
		if err != nil || recipeID == "" {
			errors = append(errors, fmt.Sprintf("%s: no recipe found", catID))
			continue
		}

		raw, err := cl.Get(fmt.Sprintf("/recipes/recipe/%s/%s", locale, recipeID))
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: recipe fetch failed", catID))
			continue
		}

		var recipe map[string]any
		if err := json.Unmarshal(raw, &recipe); err != nil {
			errors = append(errors, fmt.Sprintf("%s: parse failed", catID))
			continue
		}

		catName := extractCategoryName(recipe, catID)
		if catName == "" {
			errors = append(errors, fmt.Sprintf("%s: name not found", catID))
			continue
		}

		key := slugify(catName)
		categories[key] = catID
	}

	if !ctx.JSON {
		fmt.Println() // clear \r line
	}

	if len(categories) > 0 {
		cacheData := categoryCacheFile{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Categories: categories,
		}
		_ = config.SaveCache("categories.json", cacheData)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{
			"status":     "synced",
			"count":      len(categories),
			"errors":     errors,
			"categories": categories,
		})
	}

	fmt.Printf("Synced %d categories.\n", len(categories))
	if len(errors) > 0 {
		fmt.Printf("%d errors:\n", len(errors))
		for _, e := range errors {
			fmt.Println(" ", e)
		}
	}
	return nil
}

func fetchCategoryFacets(cl *client.Client) ([]string, error) {
	params := map[string]any{
		"query":       "",
		"hitsPerPage": 0,
		"facets":      []string{"categories.id"},
	}
	var result struct {
		Facets map[string]map[string]int `json:"facets"`
	}
	if err := algoliaQueryAuto(cl, algoliaIndex, params, &result); err != nil {
		return nil, err
	}

	facetMap := result.Facets["categories.id"]
	ids := make([]string, 0, len(facetMap))
	for id := range facetMap {
		ids = append(ids, id)
	}
	return ids, nil
}

func searchOneByCategoryID(cl *client.Client, catID string) (string, error) {
	params := map[string]any{
		"query":       "",
		"hitsPerPage": 1,
		"filters":     fmt.Sprintf("categories.id:%s", catID),
	}
	var result struct {
		Hits []struct {
			ID string `json:"id"`
		} `json:"hits"`
	}
	if err := algoliaQueryAuto(cl, algoliaIndex, params, &result); err != nil {
		return "", err
	}
	if len(result.Hits) == 0 {
		return "", nil
	}
	return result.Hits[0].ID, nil
}

func extractCategoryName(recipe map[string]any, catID string) string {
	cats, _ := recipe["categories"].([]any)
	for _, c := range cats {
		cm, _ := c.(map[string]any)
		if id, _ := cm["id"].(string); id == catID {
			title, _ := cm["title"].(string)
			return title
		}
	}
	return ""
}

func slugify(s string) string {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", " ", "-")
	s = replacer.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
