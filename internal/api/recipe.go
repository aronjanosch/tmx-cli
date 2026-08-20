package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// RecipeDetail is the compact, agent-friendly recipe schema emitted by
// `tmx recipe show`. Section fields are only populated when requested.
type RecipeDetail struct {
	ID                string             `json:"id"`
	Title             string             `json:"title"`
	URL               string             `json:"url"`
	Servings          int                `json:"servings,omitempty"`
	TotalTimeMinutes  int                `json:"totalTimeMinutes,omitempty"`
	ActiveTimeMinutes int                `json:"activeTimeMinutes,omitempty"`
	Difficulty        string             `json:"difficulty,omitempty"`
	TMVersions        []string           `json:"tmVersions,omitempty"`
	Categories        []string           `json:"categories,omitempty"`
	Ingredients       []RecipeIngredient `json:"ingredients,omitempty"`
	Steps             []RecipeStep       `json:"steps,omitempty"`
	Nutrition         map[string]string  `json:"nutrition,omitempty"`
}

type RecipeIngredient struct {
	Name        string  `json:"name"`
	Quantity    float64 `json:"quantity,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	Preparation string  `json:"preparation,omitempty"`
	Optional    bool    `json:"optional,omitempty"`
	Group       string  `json:"group,omitempty"`
}

type RecipeStep struct {
	Text  string `json:"text"`
	Group string `json:"group,omitempty"`
}

// rawRecipe mirrors the fields of GET /recipes/recipe/{locale}/{id} that we use.
type rawRecipe struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Difficulty        string   `json:"difficulty"`
	ThermomixVersions []string `json:"thermomixVersions"`
	Times             []struct {
		Type     string   `json:"type"`
		Quantity Quantity `json:"quantity"`
	} `json:"times"`
	ServingSize struct {
		Quantity Quantity `json:"quantity"`
	} `json:"servingSize"`
	Categories []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"categories"`
	RecipeIngredientGroups []struct {
		Title             string `json:"title"`
		RecipeIngredients []struct {
			IngredientNotation string   `json:"ingredientNotation"`
			UnitNotation       string   `json:"unitNotation"`
			Preparation        string   `json:"preparation"`
			Optional           bool     `json:"optional"`
			Quantity           Quantity `json:"quantity"`
		} `json:"recipeIngredients"`
	} `json:"recipeIngredientGroups"`
	RecipeStepGroups []struct {
		Title       string `json:"title"`
		RecipeSteps []struct {
			FormattedText string `json:"formattedText"`
		} `json:"recipeSteps"`
	} `json:"recipeStepGroups"`
	NutritionGroups []struct {
		RecipeNutritions []struct {
			Nutritions []struct {
				Type     string `json:"type"`
				Number   any    `json:"number"`   // string or float
				Unittype string `json:"unittype"` // both casings occur
				UnitType string `json:"unitType"`
			} `json:"nutritions"`
		} `json:"recipeNutritions"`
	} `json:"nutritionGroups"`
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// ParseRecipeDetail converts a raw recipe API response into the compact schema.
// Ingredients/steps/nutrition are always parsed; the caller drops unrequested
// sections before output.
func ParseRecipeDetail(data []byte, baseURL, locale string) (*RecipeDetail, error) {
	var raw rawRecipe
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing recipe: %w", err)
	}

	d := &RecipeDetail{
		ID:         raw.ID,
		Title:      raw.Title,
		URL:        fmt.Sprintf("%s/recipes/recipe/%s/%s", baseURL, locale, raw.ID),
		Servings:   int(raw.ServingSize.Quantity.Amount()),
		Difficulty: raw.Difficulty,
		TMVersions: raw.ThermomixVersions,
	}

	for _, t := range raw.Times {
		switch t.Type {
		case "activeTime":
			d.ActiveTimeMinutes = int(t.Quantity.Amount()) / 60
		case "totalTime":
			d.TotalTimeMinutes = int(t.Quantity.Amount()) / 60
		}
	}

	for _, c := range raw.Categories {
		if c.Title != "" {
			d.Categories = append(d.Categories, c.Title)
		}
	}

	for _, g := range raw.RecipeIngredientGroups {
		for _, ing := range g.RecipeIngredients {
			d.Ingredients = append(d.Ingredients, RecipeIngredient{
				Name:        ing.IngredientNotation,
				Quantity:    ing.Quantity.Amount(),
				Unit:        ing.UnitNotation,
				Preparation: ing.Preparation,
				Optional:    ing.Optional,
				Group:       g.Title,
			})
		}
	}

	for _, g := range raw.RecipeStepGroups {
		for _, s := range g.RecipeSteps {
			text := strings.TrimSpace(htmlTagRe.ReplaceAllString(s.FormattedText, ""))
			if text != "" {
				d.Steps = append(d.Steps, RecipeStep{Text: text, Group: g.Title})
			}
		}
	}

	nutrition := map[string]string{}
	for _, g := range raw.NutritionGroups {
		for _, rn := range g.RecipeNutritions {
			for _, n := range rn.Nutritions {
				num := parseNumber(n.Number)
				unit := n.Unittype
				if unit == "" {
					unit = n.UnitType
				}
				if n.Type != "" && num > 0 {
					nutrition[n.Type] = strings.TrimSpace(fmt.Sprintf("%.0f %s", num, strings.TrimSpace(unit)))
				}
			}
		}
	}
	if len(nutrition) > 0 {
		d.Nutrition = nutrition
	}

	return d, nil
}

func parseNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		var f float64
		fmt.Sscanf(x, "%f", &f)
		return f
	}
	return 0
}

// SearchResult is a recipe from Algolia search.
// Algolia only returns: id, title, rating, totalTime, image.
// TotalTime is in minutes.
type SearchResult struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	Image     string  `json:"image,omitempty"`
	TotalTime int     `json:"totalTimeMinutes"`
	Rating    float64 `json:"rating"`
}
