package api

type Recipe struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Image       string   `json:"image"`
	TotalTime   int      `json:"totalTime"`
	Rating      float64  `json:"rating"`
	Difficulty  string   `json:"difficulty"`
	Versions    []string `json:"thermomixVersions"`
	Servings    int      `json:"servings"`
	ActiveTime  int      `json:"activeTime"`
	Description string   `json:"description"`

	IngredientGroups []IngredientGroup `json:"recipeIngredientGroups"`
	NutritionGroups  []NutritionGroup  `json:"nutritionGroups"`
}

type IngredientGroup struct {
	Title       string       `json:"title"`
	Ingredients []Ingredient `json:"recipeIngredients"`
}

type Ingredient struct {
	Name        string  `json:"ingredientNotation"`
	Quantity    float64 `json:"-"`
	Unit        string  `json:"unitNotation"`
	Preparation string  `json:"preparation"`
	Optional    bool    `json:"optional"`
}

type NutritionGroup struct {
	Items []NutritionSet `json:"recipeNutritions"`
}

type NutritionSet struct {
	Nutritions []Nutrition `json:"nutritions"`
}

type Nutrition struct {
	Type     string  `json:"type"`
	Number   float64 `json:"number"`
	UnitType string  `json:"unittype"`
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
