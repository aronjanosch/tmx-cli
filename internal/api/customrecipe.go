package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CustomRecipeList is the response of GET /created-recipes/{locale}
// (Accept: application/vnd.vorwerk.customer-recipe.full+json).
type CustomRecipeList struct {
	Meta struct {
		RecipeLimit          int `json:"recipeLimit"`
		RecipeLimitThreshold int `json:"recipeLimitThreshold"`
	} `json:"meta"`
	Items []CustomRecipe `json:"items"`
}

type CustomRecipe struct {
	RecipeID   string              `json:"recipeId"`
	Title      string              `json:"title"`
	Status     string              `json:"status"`
	WorkStatus string              `json:"workStatus"`
	CreatedAt  string              `json:"createdAt"`
	ModifiedAt string              `json:"modifiedAt"`
	Content    CustomRecipeContent `json:"recipeContent"`
}

// CustomRecipeContent tolerates the API's alternate key spellings
// (recipeIngredient vs ingredients, tool vs tools, ...).
type CustomRecipeContent struct {
	Name         string       `json:"name"`
	TotalTime    FlexDuration `json:"totalTime"`
	PrepTime     FlexDuration `json:"prepTime"`
	Tool         []string     `json:"tool"`
	Tools        []string     `json:"tools"`
	RecipeYield  *YieldValue  `json:"recipeYield"`
	Yield        *YieldValue  `json:"yield"`
	RecipeIngred FlexTextList `json:"recipeIngredient"`
	Ingredients  FlexTextList `json:"ingredients"`
	RecipeInstr  FlexTextList `json:"recipeInstructions"`
	Instructions FlexTextList `json:"instructions"`
	Hints        string       `json:"hints"`
	Image        string       `json:"image"`
}

type YieldValue struct {
	Value    int    `json:"value"`
	UnitText string `json:"unitText"`
}

// FlexDuration parses either an ISO-8601 duration ("PT30M") or a number of seconds.
type FlexDuration int

var isoDurationRe = regexp.MustCompile(`^P(?:(\d+)D)?T?(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)

func (d *FlexDuration) UnmarshalJSON(data []byte) error {
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*d = FlexDuration(int(num))
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("duration: unsupported value %s", string(data))
	}
	s = strings.TrimSpace(s)
	if s == "" {
		*d = 0
		return nil
	}
	if m := isoDurationRe.FindStringSubmatch(s); m != nil {
		days, _ := strconv.Atoi(zeroIfEmpty(m[1]))
		hours, _ := strconv.Atoi(zeroIfEmpty(m[2]))
		mins, _ := strconv.Atoi(zeroIfEmpty(m[3]))
		secs, _ := strconv.Atoi(zeroIfEmpty(m[4]))
		*d = FlexDuration(days*86400 + hours*3600 + mins*60 + secs)
		return nil
	}
	// Plain numeric string, possibly with decimals ("1500.0").
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		*d = FlexDuration(int(f))
		return nil
	}
	return fmt.Errorf("duration: cannot parse %q", s)
}

func zeroIfEmpty(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// FlexTextList parses a list whose items are either plain strings or
// objects with a "text" field.
type FlexTextList []string

func (l *FlexTextList) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			out = append(out, s)
			continue
		}
		var obj struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(item, &obj); err == nil {
			out = append(out, obj.Text)
			continue
		}
		return fmt.Errorf("text list: unsupported item %s", string(item))
	}
	*l = out
	return nil
}
