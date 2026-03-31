package api

type PlanDay struct {
	Date      string        `json:"date"`
	DayName   string        `json:"dayName"`
	DayNumber string        `json:"dayNumber"`
	IsToday   bool          `json:"isToday"`
	Recipes   []PlanRecipe  `json:"recipes"`
}

type PlanRecipe struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Image string `json:"image"`
}
