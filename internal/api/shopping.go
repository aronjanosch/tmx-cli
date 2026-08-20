package api

// ShoppingList is the response of GET /shopping/{locale}.
type ShoppingList struct {
	Recipes         []ShoppingRecipe `json:"recipes"`
	CustomerRecipes []ShoppingRecipe `json:"customerRecipes"`
	AdditionalItems []AdditionalItem `json:"additionalItems"`
}

// AllRecipes returns Vorwerk and customer recipes as one fresh slice.
func (l *ShoppingList) AllRecipes() []ShoppingRecipe {
	out := make([]ShoppingRecipe, 0, len(l.Recipes)+len(l.CustomerRecipes))
	out = append(out, l.Recipes...)
	out = append(out, l.CustomerRecipes...)
	return out
}

type ShoppingRecipe struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	ULID             string `json:"ulid"`
	IsCustomerRecipe bool   `json:"isCustomerRecipe"`
	// Despite the name, this is a flat list of ingredient items.
	Ingredients []ShoppingIngredient `json:"recipeIngredientGroups"`
}

type ShoppingIngredient struct {
	ID          string   `json:"id"`
	Name        string   `json:"ingredientNotation"`
	Unit        string   `json:"unitNotation"`
	Quantity    Quantity `json:"quantity"`
	Preparation string   `json:"preparation"`
	Optional    bool     `json:"optional"`
	IsOwned     bool     `json:"isOwned"`
}

// Quantity is either {"value": N} or a range {"from": N, "to": M}.
type Quantity struct {
	Value float64  `json:"value,omitempty"`
	From  *float64 `json:"from,omitempty"`
	To    *float64 `json:"to,omitempty"`
}

func (q Quantity) Amount() float64 {
	if q.From != nil {
		return *q.From
	}
	return q.Value
}

type AdditionalItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsOwned bool   `json:"isOwned"`
}
