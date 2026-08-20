package cmd

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/aronjanosch/tmx-cli/internal/api"
	"github.com/aronjanosch/tmx-cli/internal/client"
)

const (
	algoliaIndex = "recipes-production-de"
	cookidooBase = "https://cookidoo.de"
	locale       = "de-DE"
)

// Hardcoded category fallback — used until `tmx categories sync` builds the full cache.
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
	Query      string   `arg:"" optional:"" help:"Search query."`
	Limit      int      `short:"n" default:"10" help:"Max results."`
	Time       int      `short:"t" help:"Max preparation time in minutes."`
	Difficulty string   `short:"d" help:"Difficulty: easy|medium|advanced."`
	TM         string   `name:"tm" help:"Thermomix version: TM5|TM6|TM7."`
	Category   string   `short:"c" help:"Category filter (e.g. pasta, vegetarisch)."`
	MinRating  float64  `name:"min-rating" help:"Minimum rating (0-5)."`
	Diet       string   `help:"Diet: vegetarian|vegan|pescetarian|low-carb|keto."`
	Goal       string   `help:"Nutrition goal: high-protein|low-calories|low-fat|high-fibre|low-sodium|low-histamine."`
	FreeOf     []string `name:"free-of" help:"Free of: gluten|lactose|nut|sugar|meat|seafood|alcohol|caffeine (repeatable)."`
	Ingredient []string `short:"I" help:"Must use this ingredient (repeatable, verified against the recipe)."`
	Exclude    []string `short:"x" help:"Must NOT use this ingredient (repeatable, verified against the recipe)."`
	Images     bool     `help:"Include image URLs in JSON output."`
	NoPrefs    bool     `name:"no-prefs" help:"Ignore preferences from tmx setup."`
}

// Facet enum mappings (flag value → Algolia facet token).
var dietFacets = map[string]string{
	"vegetarian": "vegetarian", "vegetarisch": "vegetarian",
	"vegan": "vegan", "pescetarian": "pescetarian",
	"low-carb": "low_carb", "low_carb": "low_carb", "keto": "keto",
}

var goalFacets = map[string]string{
	"high-protein": "high_protein", "low-calories": "low_calories",
	"low-fat": "low_fat", "high-fibre": "high_fibre", "high-fiber": "high_fibre",
	"low-sodium": "low_sodium", "low-histamine": "low_histamine",
}

var freeOfFacets = map[string]string{
	"gluten": "gluten_free", "lactose": "lactose_free", "nut": "nut_free",
	"sugar": "sugar_free", "meat": "without_meat", "seafood": "without_seafood",
	"alcohol": "alcohol_free", "caffeine": "caffeine_free",
}

func facetKeys(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// appliedFilters echoes what was actually sent, so agents see which
// setup defaults kicked in.
type appliedFilters struct {
	Query          string   `json:"query,omitempty"`
	Category       string   `json:"category,omitempty"`
	MaxTimeMinutes int      `json:"maxTimeMinutes,omitempty"`
	Difficulty     string   `json:"difficulty,omitempty"`
	TM             string   `json:"tm,omitempty"`
	MinRating      float64  `json:"minRating,omitempty"`
	Diet           string   `json:"diet,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	FreeOf         []string `json:"freeOf,omitempty"`
	Ingredients    []string `json:"ingredients,omitempty"`
	Exclude        []string `json:"excludeIngredients,omitempty"`
	Backend        string   `json:"backend,omitempty"`
}

func (s *SearchCmd) Run(ctx *Context) error {
	applied := appliedFilters{}

	// Apply config defaults when flags not set
	if !s.NoPrefs {
		if s.TM == "" {
			s.TM = ctx.Config.TMVersion
		}
		if s.Time == 0 && ctx.Config.MaxTime > 0 {
			s.Time = ctx.Config.MaxTime
		}
		if s.Diet == "" {
			s.Diet = strings.ToLower(ctx.Config.Diet)
		}
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	// Recipes of all markets share one index — pin to our language.
	lang := strings.SplitN(locale, "-", 2)[0]
	filters := []string{fmt.Sprintf("language:%s", lang)}
	if s.Time > 0 {
		filters = append(filters, fmt.Sprintf("totalTime <= %d", s.Time*60))
	}
	if s.Difficulty != "" {
		filters = append(filters, fmt.Sprintf("difficulty:%s", s.Difficulty))
	}
	if s.TM != "" {
		filters = append(filters, fmt.Sprintf("tmversion:%s", s.TM))
	}
	if s.MinRating > 0 {
		filters = append(filters, fmt.Sprintf("rating >= %g", s.MinRating))
	}
	if s.Diet != "" {
		facet, ok := dietFacets[strings.ToLower(s.Diet)]
		if !ok {
			return fmt.Errorf("unknown diet %q — valid: %s", s.Diet, facetKeys(dietFacets))
		}
		filters = append(filters, fmt.Sprintf("dietary:%s", facet))
	}
	if s.Goal != "" {
		facet, ok := goalFacets[strings.ToLower(s.Goal)]
		if !ok {
			return fmt.Errorf("unknown goal %q — valid: %s", s.Goal, facetKeys(goalFacets))
		}
		filters = append(filters, fmt.Sprintf("nutritionGoal:%s", facet))
	}
	for _, f := range s.FreeOf {
		facet, ok := freeOfFacets[strings.ToLower(f)]
		if !ok {
			return fmt.Errorf("unknown --free-of %q — valid: %s", f, facetKeys(freeOfFacets))
		}
		filters = append(filters, fmt.Sprintf("freeOfIngredient:%s", facet))
	}
	if s.Category != "" {
		catID, err := resolveCategory(s.Category)
		if err != nil {
			return err
		}
		filters = append(filters, fmt.Sprintf("categories.id:%s", catID))
	}

	applied.Query = s.Query
	applied.Category = s.Category
	applied.MaxTimeMinutes = s.Time
	applied.Difficulty = s.Difficulty
	applied.TM = s.TM
	applied.MinRating = s.MinRating
	applied.Diet = s.Diet
	applied.Goal = s.Goal
	applied.FreeOf = s.FreeOf

	// Ingredient constraints: fold -I terms into the query for ranking, then
	// verify against each candidate's real ingredient list. Free-text facets
	// on the API side are unreliable; checking the recipe is authoritative.
	verify := len(s.Ingredient) > 0 || len(s.Exclude) > 0
	query := s.Query
	hits := s.Limit
	if verify {
		query = strings.TrimSpace(query + " " + strings.Join(s.Ingredient, " "))
		hits = min(20, s.Limit*3)
		applied.Query = query
		applied.Ingredients = s.Ingredient
		applied.Exclude = s.Exclude
		applied.Backend = "algolia+ingredient-verify"
	}

	params := map[string]any{
		"query":       query,
		"hitsPerPage": hits,
	}
	if len(filters) > 0 {
		params["filters"] = strings.Join(filters, " AND ")
	}

	var result struct {
		Hits []struct {
			ID        string  `json:"id"`
			Title     string  `json:"title"`
			Rating    float64 `json:"rating"`
			TotalTime float64 `json:"totalTime"`
			Image     string  `json:"image"`
		} `json:"hits"`
		NbHits int `json:"nbHits"`
	}
	if err := algoliaQueryAuto(cl, algoliaIndex, params, &result); err != nil {
		return err
	}

	results := make([]api.SearchResult, 0, len(result.Hits))
	for _, hit := range result.Hits {
		r := api.SearchResult{
			ID:        hit.ID,
			Title:     hit.Title,
			URL:       fmt.Sprintf("%s/recipes/recipe/%s/%s", cookidooBase, locale, hit.ID),
			TotalTime: int(hit.TotalTime) / 60,
			// Full float precision is noise for agents.
			Rating: math.Round(hit.Rating*10) / 10,
		}
		if s.Images {
			r.Image = resolveImageURL(hit.Image)
		}
		results = append(results, r)
	}

	if verify {
		results, err = filterByIngredients(cl, results, s.Ingredient, s.Exclude, s.Limit)
		if err != nil {
			return err
		}
		return outputSearchResults(ctx, results, len(results), applied)
	}

	return outputSearchResults(ctx, results, result.NbHits, applied)
}

// filterByIngredients keeps only recipes whose real ingredient list contains
// every wanted term and none of the excluded terms (case-insensitive substring).
func filterByIngredients(cl *client.Client, candidates []api.SearchResult, want, exclude []string, limit int) ([]api.SearchResult, error) {
	out := []api.SearchResult{}
	for _, cand := range candidates {
		if len(out) >= limit {
			break
		}
		raw, err := cl.Get(fmt.Sprintf("/recipes/recipe/%s/%s", locale, cand.ID))
		if err != nil {
			return nil, fmt.Errorf("checking ingredients of %s: %w", cand.ID, err)
		}
		detail, err := api.ParseRecipeDetail(raw, cookidooBase, locale)
		if err != nil {
			return nil, fmt.Errorf("checking ingredients of %s: %w", cand.ID, err)
		}

		var names []string
		for _, ing := range detail.Ingredients {
			names = append(names, strings.ToLower(ing.Name))
		}
		all := strings.Join(names, "\n")

		ok := true
		for _, w := range want {
			if !strings.Contains(all, strings.ToLower(w)) {
				ok = false
				break
			}
		}
		for _, x := range exclude {
			if strings.Contains(all, strings.ToLower(x)) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, cand)
		}
	}
	return out, nil
}

func outputSearchResults(ctx *Context, results []api.SearchResult, total int, applied appliedFilters) error {
	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{
			"data":    results,
			"count":   len(results),
			"total":   total,
			"applied": applied,
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
			r.ID, title, formatTime(r.TotalTime*60), r.Rating)
	}
	fmt.Printf("\n%d of %d results\n", len(results), total)
	return nil
}

// resolveCategory maps a category slug to its Cookidoo ID, erroring with the
// valid options instead of silently dropping an unknown filter.
func resolveCategory(name string) (string, error) {
	cats, _ := loadCategoriesCache()
	if id, ok := cats[strings.ToLower(name)]; ok {
		return id, nil
	}
	known := make([]string, 0, len(cats))
	for k := range cats {
		known = append(known, k)
	}
	sort.Strings(known)
	return "", fmt.Errorf("unknown category %q — valid: %s (refresh with: tmx categories sync)",
		name, strings.Join(known, ", "))
}

// resolveImageURL fills the {assethost}/{transformation} placeholders the API
// leaves in image URLs, so agents get a directly usable link.
func resolveImageURL(u string) string {
	u = strings.ReplaceAll(u, "{assethost}", "assets.tmecosys.com")
	u = strings.ReplaceAll(u, "{transformation}", "t_web_shared_recipe_221x240")
	return u
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
