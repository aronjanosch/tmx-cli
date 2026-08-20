package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aronjanosch/tmx-cli/internal/api"
	"github.com/aronjanosch/tmx-cli/internal/config"
)

type PlanCmd struct {
	Show   PlanShowCmd   `cmd:"" help:"Show cached meal plan."`
	Sync   PlanSyncCmd   `cmd:"" help:"Sync meal plan from Cookidoo."`
	Add    PlanAddCmd    `cmd:"" help:"Add a recipe to the plan."`
	Remove PlanRemoveCmd `cmd:"" help:"Remove a recipe from the plan."`
	Move   PlanMoveCmd   `cmd:"" help:"Move a recipe to another day."`
}

type PlanShowCmd struct{}

func (p *PlanShowCmd) Run(ctx *Context) error {
	days, err := loadPlanCache()
	if err != nil {
		return fmt.Errorf("no cached plan — run: tmx plan sync")
	}

	if ctx.JSON {
		out := map[string]any{"data": days, "count": len(days)}
		if info, err := os.Stat(filepath.Join(config.Dir(), "weekplan.json")); err == nil {
			out["syncedAt"] = info.ModTime().UTC().Format(time.RFC3339)
		}
		return ctx.PrintJSON(out)
	}

	printPlan(days)
	return nil
}

type PlanSyncCmd struct {
	Since string `short:"s" help:"Start date (YYYY-MM-DD, default: today)."`
	Days  int    `short:"d" default:"14" help:"Number of days to fetch."`
}

func (p *PlanSyncCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	since := p.Since
	if since == "" {
		since = time.Now().Format("2006-01-02")
	}
	sinceDate, err := time.Parse("2006-01-02", since)
	if err != nil {
		return fmt.Errorf("invalid date: %s", since)
	}

	// The API returns the calendar week (Mon–Sun) containing the given date,
	// so walk Monday anchors until the whole requested range is covered.
	monday := sinceDate.AddDate(0, 0, -(int(sinceDate.Weekday())+6)%7)
	endDate := sinceDate.AddDate(0, 0, p.Days-1)

	byDate := map[string]api.MyDay{}
	for anchor := monday; !anchor.After(endDate); anchor = anchor.AddDate(0, 0, 7) {
		weekDate := anchor.Format("2006-01-02")
		raw, err := cl.Get(fmt.Sprintf("/planning/%s/api/my-week/%s", locale, weekDate))
		if err != nil {
			return fmt.Errorf("fetching week %s: %w", weekDate, err)
		}
		var week api.MyWeek
		if err := json.Unmarshal(raw, &week); err != nil {
			return fmt.Errorf("parsing week %s: %w", weekDate, err)
		}
		for _, d := range week.MyDays {
			byDate[d.DayKey] = d
		}
	}

	// Build one entry per requested day; days absent from the API are empty.
	today := time.Now().Format("2006-01-02")
	days := make([]api.PlanDay, 0, p.Days)
	for i := 0; i < p.Days; i++ {
		date := sinceDate.AddDate(0, 0, i)
		key := date.Format("2006-01-02")

		day := api.PlanDay{
			Date:      key,
			DayName:   germanWeekday(date.Weekday()),
			DayNumber: fmt.Sprintf("%d", date.Day()),
			IsToday:   key == today,
			Recipes:   []api.PlanRecipe{},
		}
		if md, ok := byDate[key]; ok {
			for _, r := range md.Recipes {
				day.Recipes = append(day.Recipes, api.PlanRecipe{
					ID:    r.ID,
					Title: r.Title,
					URL:   fmt.Sprintf("%s/recipes/recipe/%s/%s", cookidooBase, locale, r.ID),
				})
			}
			day.CustomRecipeIDs = md.CustomerRecipeIDs
		}
		days = append(days, day)
	}

	if err := config.SaveCache("weekplan.json", days); err != nil {
		return fmt.Errorf("saving plan: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"data": days, "count": len(days)})
	}

	fmt.Printf("Synced %d days.\n", len(days))
	printPlan(days)
	return nil
}

func germanWeekday(d time.Weekday) string {
	names := map[time.Weekday]string{
		time.Monday: "Mo", time.Tuesday: "Di", time.Wednesday: "Mi",
		time.Thursday: "Do", time.Friday: "Fr", time.Saturday: "Sa", time.Sunday: "So",
	}
	return names[d]
}

type PlanAddCmd struct {
	RecipeID string `arg:"" help:"Recipe ID."`
	Date     string `short:"d" help:"Date (YYYY-MM-DD, default: today)."`
	Custom   bool   `short:"c" help:"Recipe is a custom (own) recipe."`
}

func (p *PlanAddCmd) Run(ctx *Context) error {
	if p.Date == "" {
		p.Date = time.Now().Format("2006-01-02")
	}
	id := p.RecipeID
	source := "VORWERK"
	if p.Custom {
		source = "CUSTOMER"
	} else {
		id = ensurePrefix(id, "r")
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	_, err = cl.PutJSON(fmt.Sprintf("/planning/%s/api/my-day", locale), map[string]any{
		"recipeSource": source,
		"recipeIds":    []string{id},
		"dayKey":       p.Date,
	})
	if err != nil {
		return fmt.Errorf("adding recipe: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "added", "id": id, "date": p.Date})
	}
	fmt.Printf("Added %s to %s.\n", id, p.Date)
	return nil
}

type PlanRemoveCmd struct {
	RecipeID string `arg:"" help:"Recipe ID."`
	Date     string `short:"d" required:"" help:"Date (YYYY-MM-DD)."`
	Custom   bool   `short:"c" help:"Recipe is a custom (own) recipe."`
}

func (p *PlanRemoveCmd) Run(ctx *Context) error {
	id := p.RecipeID
	source := "VORWERK"
	if p.Custom {
		source = "CUSTOMER"
	} else {
		id = ensurePrefix(id, "r")
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	err = cl.Delete(fmt.Sprintf("/planning/%s/api/my-day/%s/recipes/%s?recipeSource=%s", locale, p.Date, id, source))
	if err != nil {
		return fmt.Errorf("removing recipe: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "removed", "id": id, "date": p.Date})
	}
	fmt.Printf("Removed %s from %s.\n", id, p.Date)
	return nil
}

type PlanMoveCmd struct {
	RecipeID string `arg:"" help:"Recipe ID."`
	From     string `short:"f" required:"" help:"Source date (YYYY-MM-DD)."`
	To       string `short:"t" required:"" help:"Target date (YYYY-MM-DD)."`
	Custom   bool   `short:"c" help:"Recipe is a custom (own) recipe."`
}

func (p *PlanMoveCmd) Run(ctx *Context) error {
	id := p.RecipeID
	source := "VORWERK"
	if p.Custom {
		source = "CUSTOMER"
	} else {
		id = ensurePrefix(id, "r")
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	if err := cl.Delete(fmt.Sprintf("/planning/%s/api/my-day/%s/recipes/%s?recipeSource=%s", locale, p.From, id, source)); err != nil {
		return fmt.Errorf("removing from %s: %w", p.From, err)
	}

	if _, err := cl.PutJSON(fmt.Sprintf("/planning/%s/api/my-day", locale), map[string]any{
		"recipeSource": source,
		"recipeIds":    []string{id},
		"dayKey":       p.To,
	}); err != nil {
		return fmt.Errorf("adding to %s: %w", p.To, err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "moved", "id": id, "from": p.From, "to": p.To})
	}
	fmt.Printf("Moved %s from %s to %s.\n", id, p.From, p.To)
	return nil
}

func printPlan(days []api.PlanDay) {
	for _, d := range days {
		marker := ""
		if d.IsToday {
			marker = " [today]"
		}
		fmt.Printf("\n%s %s %s%s\n", d.DayName, d.DayNumber, d.Date, marker)
		if len(d.Recipes) == 0 && len(d.CustomRecipeIDs) == 0 {
			fmt.Println("  (no recipes)")
		}
		for _, r := range d.Recipes {
			fmt.Printf("  %-12s  %s\n", r.ID, r.Title)
		}
		for _, id := range d.CustomRecipeIDs {
			fmt.Printf("  %-12s  (custom recipe)\n", id)
		}
	}
	fmt.Println()
}

func ensurePrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s
	}
	return prefix + s
}

// loadPlanCache loads the cached plan and recomputes IsToday from the current date.
func loadPlanCache() ([]api.PlanDay, error) {
	var days []api.PlanDay
	if err := config.LoadCache("weekplan.json", &days); err != nil {
		return nil, err
	}
	today := time.Now().Format("2006-01-02")
	for i := range days {
		days[i].IsToday = days[i].Date == today
	}
	return days, nil
}
