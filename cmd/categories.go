package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

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

	token, err := getSearchToken(cl)
	if err != nil {
		return fmt.Errorf("could not get search token (are you logged in?): %w", err)
	}

	if !ctx.JSON {
		fmt.Println("Fetching category IDs from Algolia...")
	}

	categoryIDs, err := fetchCategoryFacets(token)
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

		recipeID, err := searchOneByCategoryID(token, catID)
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

func fetchCategoryFacets(token string) ([]string, error) {
	params := map[string]any{
		"query":       "",
		"hitsPerPage": 0,
		"facets":      []string{"categories.id"},
	}
	body, _ := json.Marshal(params)
	algoliaURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", algoliaAppID, algoliaIndex)
	req, err := http.NewRequest("POST", algoliaURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Algolia-Application-Id", algoliaAppID)
	req.Header.Set("X-Algolia-API-Key", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result struct {
		Facets map[string]map[string]int `json:"facets"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}

	facetMap := result.Facets["categories.id"]
	ids := make([]string, 0, len(facetMap))
	for id := range facetMap {
		ids = append(ids, id)
	}
	return ids, nil
}

func searchOneByCategoryID(token, catID string) (string, error) {
	params := map[string]any{
		"query":       "",
		"hitsPerPage": 1,
		"filters":     fmt.Sprintf("categories.id:%s", catID),
	}
	body, _ := json.Marshal(params)
	algoliaURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", algoliaAppID, algoliaIndex)
	req, err := http.NewRequest("POST", algoliaURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Algolia-Application-Id", algoliaAppID)
	req.Header.Set("X-Algolia-API-Key", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result struct {
		Hits []map[string]any `json:"hits"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if len(result.Hits) == 0 {
		return "", nil
	}
	id, _ := result.Hits[0]["id"].(string)
	return id, nil
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
