package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aronjanosch/tmx-cli/internal/api"
	"github.com/aronjanosch/tmx-cli/internal/config"
)

const (
	algoliaAppID = "3TA8NT85XJ"
	algoliaIndex = "recipes-production-de"
	cookidooBase = "https://cookidoo.de"
	locale       = "de-DE"
)

// Hardcoded category fallback — matches the Python source.
var categoryFallback = map[string]string{
	"vorspeisen":      "VrkNavCategory-RPF-001",
	"suppen":          "VrkNavCategory-RPF-002",
	"pasta":           "VrkNavCategory-RPF-003",
	"fleisch":         "VrkNavCategory-RPF-004",
	"fisch":           "VrkNavCategory-RPF-005",
	"vegetarisch":     "VrkNavCategory-RPF-006",
	"beilagen":        "VrkNavCategory-RPF-008",
	"desserts":        "VrkNavCategory-RPF-011",
	"herzhaft-backen": "VrkNavCategory-RPF-012",
	"kuchen":          "VrkNavCategory-RPF-013",
	"brot":            "VrkNavCategory-RPF-014",
	"getraenke":       "VrkNavCategory-RPF-015",
	"grundrezepte":    "VrkNavCategory-RPF-016",
	"saucen":          "VrkNavCategory-RPF-018",
	"snacks":          "VrkNavCategory-RPF-020",
}

type SearchCmd struct {
	Query      string `arg:"" optional:"" help:"Search query."`
	Limit      int    `short:"n" default:"10" help:"Max results."`
	Time       int    `short:"t" help:"Max preparation time in minutes."`
	Difficulty string `short:"d" help:"Difficulty: easy|medium|advanced."`
	TM         string `name:"tm" help:"Thermomix version: TM5|TM6|TM7."`
	Category   string `short:"c" help:"Category filter (e.g. pasta, vegetarisch)."`
}

func (s *SearchCmd) Run(ctx *Context) error {
	// Apply config defaults when flags not set
	if s.TM == "" {
		s.TM = ctx.Config.TMVersion
	}
	if s.Time == 0 && ctx.Config.MaxTime > 0 {
		s.Time = ctx.Config.MaxTime
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	token, err := getSearchToken(cl)
	if err != nil {
		return fmt.Errorf("could not get search token (are you logged in?): %w", err)
	}

	results, total, err := searchRecipes(token, s.Query, s.Limit, s.Time, s.Difficulty, s.TM, s.Category)
	if err != nil {
		return err
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{
			"data":  results,
			"count": len(results),
			"total": total,
		})
	}

	if len(results) == 0 {
		fmt.Println("No recipes found.")
		return nil
	}

	fmt.Printf("%-12s  %-48s  %8s  %s\n", "ID", "Title", "Time", "Rating")
	fmt.Printf("%-12s  %-48s  %8s  %s\n", "----", "-----", "----", "------")
	for _, r := range results {
		title := r.Title
		if len(title) > 48 {
			title = title[:45] + "..."
		}
		fmt.Printf("%-12s  %-48s  %8s  %.1f\n",
			r.ID, title, formatTime(r.TotalTime), r.Rating)
	}
	fmt.Printf("\n%d of %d results\n", len(results), total)
	return nil
}

type searchTokenCache struct {
	APIKey     string  `json:"apiKey"`
	ValidUntil float64 `json:"validUntil"`
}

func getSearchToken(cl interface{ Get(string) ([]byte, error) }) (string, error) {
	// Check cache
	var cached searchTokenCache
	if err := config.LoadCache("search_token.json", &cached); err == nil {
		if cached.ValidUntil > float64(time.Now().Unix()+300) {
			return cached.APIKey, nil
		}
	}

	raw, err := cl.Get("/search/api/subscription/token")
	if err != nil {
		return "", err
	}

	var data searchTokenCache
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("parsing search token: %w", err)
	}

	_ = config.SaveCache("search_token.json", data)
	return data.APIKey, nil
}

func searchRecipes(token, query string, limit, maxTimeMins int, difficulty, tmVersion, category string) ([]api.SearchResult, int, error) {
	filters := []string{}
	if maxTimeMins > 0 {
		filters = append(filters, fmt.Sprintf("totalTime <= %d", maxTimeMins*60))
	}
	if difficulty != "" {
		filters = append(filters, fmt.Sprintf("difficulty:%s", difficulty))
	}
	if tmVersion != "" {
		filters = append(filters, fmt.Sprintf("tmversion:%s", tmVersion))
	}
	if category != "" {
		catID := categoryFallback[strings.ToLower(category)]
		if catID != "" {
			filters = append(filters, fmt.Sprintf("categories.id:%s", catID))
		}
	}

	params := map[string]any{
		"query":       query,
		"hitsPerPage": limit,
	}
	if len(filters) > 0 {
		params["filters"] = strings.Join(filters, " AND ")
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, 0, err
	}

	algoliaURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", algoliaAppID, algoliaIndex)
	req, err := http.NewRequest("POST", algoliaURL, bytes.NewReader(body))
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
		return nil, 0, fmt.Errorf("parsing search results: %w", err)
	}

	recipes := make([]api.SearchResult, 0, len(result.Hits))
	for _, hit := range result.Hits {
		id, _ := hit["id"].(string)
		title, _ := hit["title"].(string)
		rating, _ := hit["rating"].(float64)
		totalTime, _ := hit["totalTime"].(float64)
		image, _ := hit["image"].(string)

		recipes = append(recipes, api.SearchResult{
			ID:        id,
			Title:     title,
			URL:       fmt.Sprintf("%s/recipes/recipe/%s/%s", cookidooBase, locale, id),
			Image:     image,
			TotalTime: int(totalTime) / 60,
			Rating:    rating,
		})
	}

	return recipes, result.NbHits, nil
}

func formatTime(seconds int) string {
	if seconds == 0 {
		return ""
	}
	mins := seconds / 60
	if mins < 60 {
		return fmt.Sprintf("%d min", mins)
	}
	h := mins / 60
	m := mins % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
