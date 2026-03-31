package api

type ShoppingItem struct {
	Name        string   `json:"name"`
	Quantity    float64  `json:"quantity"`
	Unit        string   `json:"unit"`
	Preparation string   `json:"preparation"`
	IsOwned     bool     `json:"is_owned"`
	Optional    bool     `json:"optional"`
	Recipes     []string `json:"recipes"`
}
