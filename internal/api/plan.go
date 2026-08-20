package api

type PlanDay struct {
	Date            string       `json:"date"`
	DayName         string       `json:"dayName"`
	DayNumber       string       `json:"dayNumber"`
	IsToday         bool         `json:"isToday"`
	Recipes         []PlanRecipe `json:"recipes"`
	CustomRecipeIDs []string     `json:"customRecipeIds,omitempty"`
}

type PlanRecipe struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Image string `json:"image,omitempty"`
}

// MyWeek is the response of GET /planning/{locale}/api/my-week/{date}.
type MyWeek struct {
	MyDays []MyDay `json:"myDays"`
}

type MyDay struct {
	DayKey            string         `json:"dayKey"`
	Title             string         `json:"title"`
	Recipes           []MyWeekRecipe `json:"recipes"`
	CustomerRecipeIDs []string       `json:"customerRecipeIds"`
}

type MyWeekRecipe struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	TotalTime string `json:"totalTime"` // decimal string of seconds, e.g. "1500.0"
	Locale    string `json:"locale"`
}
