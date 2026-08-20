package cmd

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aronjanosch/tmx-cli/internal/api"
	"github.com/aronjanosch/tmx-cli/internal/auth"
)

// scrapeRecipe fetches a recipe page and extracts the schema.org Recipe
// (JSON-LD) into the import schema. TTS parameters stay empty — semantic
// Thermomix conversion is the calling agent's job.
func scrapeRecipe(url string) (*ImportRecipe, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", auth.BrowserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	node := findRecipeNode(string(body))
	if node == nil {
		return nil, fmt.Errorf("no schema.org Recipe found on page — build the recipe JSON manually (see SKILL.md import schema)")
	}
	return parseRecipeNode(node, url)
}

var (
	ldJSONRe  = regexp.MustCompile(`(?is)<script[^>]+application/ld\+json[^>]*>(.*?)</script>`)
	htmlTagRe = regexp.MustCompile(`<[^>]+>`)
)

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// findRecipeNode locates the JSON-LD object whose @type is (or contains) "Recipe".
func findRecipeNode(htmlBody string) map[string]any {
	for _, m := range ldJSONRe.FindAllStringSubmatch(htmlBody, -1) {
		var doc any
		if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &doc); err != nil {
			continue
		}
		if node := searchRecipe(doc, 0); node != nil {
			return node
		}
	}
	return nil
}

func searchRecipe(v any, depth int) map[string]any {
	if depth > 4 {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		if isRecipeType(x["@type"]) {
			return x
		}
		if g, ok := x["@graph"]; ok {
			if node := searchRecipe(g, depth+1); node != nil {
				return node
			}
		}
	case []any:
		for _, item := range x {
			if node := searchRecipe(item, depth+1); node != nil {
				return node
			}
		}
	}
	return nil
}

func isRecipeType(t any) bool {
	switch x := t.(type) {
	case string:
		return x == "Recipe"
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok && s == "Recipe" {
				return true
			}
		}
	}
	return false
}

func parseRecipeNode(node map[string]any, sourceURL string) (*ImportRecipe, error) {
	recipe := &ImportRecipe{Source: sourceURL}

	recipe.Title = cleanText(stringOf(node["name"]))
	if recipe.Title == "" {
		return nil, fmt.Errorf("recipe has no name")
	}

	for _, ing := range asList(node["recipeIngredient"]) {
		if s := cleanText(stringOf(ing)); s != "" {
			recipe.Ingredients = append(recipe.Ingredients, s)
		}
	}

	recipe.Steps = parseInstructions(node["recipeInstructions"])

	recipe.PrepTimeMinutes = durationMinutes(node["prepTime"])
	recipe.TotalTimeMinutes = durationMinutes(node["totalTime"])
	if recipe.TotalTimeMinutes == 0 {
		recipe.TotalTimeMinutes = recipe.PrepTimeMinutes + durationMinutes(node["cookTime"])
	}

	recipe.Servings = parseYield(node["recipeYield"])

	if len(recipe.Ingredients) == 0 && len(recipe.Steps) == 0 {
		return nil, fmt.Errorf("recipe %q has neither ingredients nor instructions", recipe.Title)
	}
	return recipe, nil
}

func parseInstructions(v any) []ImportStep {
	var steps []ImportStep
	switch x := v.(type) {
	case string:
		for _, line := range strings.Split(x, "\n") {
			if s := cleanText(line); s != "" {
				steps = append(steps, ImportStep{Text: s})
			}
		}
	case []any:
		for _, item := range x {
			switch node := item.(type) {
			case string:
				if s := cleanText(node); s != "" {
					steps = append(steps, ImportStep{Text: s})
				}
			case map[string]any:
				typ, _ := node["@type"].(string)
				if typ == "HowToSection" {
					steps = append(steps, parseInstructions(node["itemListElement"])...)
					continue
				}
				// Some sites put the instruction in "name" instead of "text".
				text := cleanText(stringOf(node["text"]))
				if text == "" {
					text = cleanText(stringOf(node["name"]))
				}
				if text != "" {
					steps = append(steps, ImportStep{Text: text})
				}
			}
		}
	}
	return steps
}

func durationMinutes(v any) int {
	s := stringOf(v)
	if s == "" {
		return 0
	}
	var d api.FlexDuration
	raw, _ := json.Marshal(s)
	if err := json.Unmarshal(raw, &d); err != nil {
		return 0
	}
	return int(d) / 60
}

var firstIntRe = regexp.MustCompile(`\d+`)

func parseYield(v any) int {
	var s string
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		s = x
	case []any:
		for _, item := range x {
			if n := parseYield(item); n > 0 {
				return n
			}
		}
		return 0
	}
	if m := firstIntRe.FindString(s); m != "" {
		n, _ := strconv.Atoi(m)
		return n
	}
	return 0
}

func stringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func cleanText(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRe.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}
