package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
		out := map[string]any{"data": days}
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

	today := time.Now().Format("2006-01-02")
	sinceDate, err := time.Parse("2006-01-02", since)
	if err != nil {
		return fmt.Errorf("invalid date: %s", since)
	}

	var allDays []api.PlanDay
	weeks := (p.Days + 6) / 7
	for i := 0; i < weeks; i++ {
		weekDate := sinceDate.AddDate(0, 0, i*7).Format("2006-01-02")
		raw, err := cl.Get(fmt.Sprintf("/planning/%s/calendar/week?date=%s&today=%s", locale, weekDate, today))
		if err != nil {
			fmt.Printf("warning: could not fetch week %s: %v\n", weekDate, err)
			continue
		}
		days := parseWeekplanHTML(string(raw))
		allDays = append(allDays, days...)
	}

	// Deduplicate and limit to requested days
	seen := map[string]bool{}
	var unique []api.PlanDay
	for _, d := range allDays {
		if !seen[d.Date] {
			seen[d.Date] = true
			unique = append(unique, d)
		}
	}

	if err := config.SaveCache("weekplan.json", unique); err != nil {
		return fmt.Errorf("saving plan: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"data": unique, "synced": len(unique)})
	}

	fmt.Printf("Synced %d days.\n", len(unique))
	printPlan(unique)
	return nil
}

type PlanAddCmd struct {
	RecipeID string `arg:"" help:"Recipe ID."`
	Date     string `short:"d" help:"Date (YYYY-MM-DD, default: today)."`
}

func (p *PlanAddCmd) Run(ctx *Context) error {
	if p.Date == "" {
		p.Date = time.Now().Format("2006-01-02")
	}
	id := ensurePrefix(p.RecipeID, "r")

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	_, err = cl.PutJSON(fmt.Sprintf("/planning/%s/api/my-day", locale), map[string]any{
		"recipeSource": "VORWERK",
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
}

func (p *PlanRemoveCmd) Run(ctx *Context) error {
	id := ensurePrefix(p.RecipeID, "r")

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	err = cl.Delete(fmt.Sprintf("/planning/%s/api/my-day/%s/recipes/%s?recipeSource=VORWERK", locale, p.Date, id))
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
}

func (p *PlanMoveCmd) Run(ctx *Context) error {
	id := ensurePrefix(p.RecipeID, "r")

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	if err := cl.Delete(fmt.Sprintf("/planning/%s/api/my-day/%s/recipes/%s?recipeSource=VORWERK", locale, p.From, id)); err != nil {
		return fmt.Errorf("removing from %s: %w", p.From, err)
	}

	if _, err := cl.PutJSON(fmt.Sprintf("/planning/%s/api/my-day", locale), map[string]any{
		"recipeSource": "VORWERK",
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
		if len(d.Recipes) == 0 {
			fmt.Println("  (no recipes)")
		}
		for _, r := range d.Recipes {
			fmt.Printf("  %-12s  %s\n", r.ID, r.Title)
		}
	}
	fmt.Println()
}

var (
	dayBlockRe    = regexp.MustCompile(`(?s)<plan-week-day[^>]*date="([^"]+)"[^>]*>(.*?)</plan-week-day>`)
	dayShortRe    = regexp.MustCompile(`class="my-week__day-short">([^<]+)<`)
	dayNumberRe   = regexp.MustCompile(`class="my-week__day-number">([^<]+)<`)
	coreTileRe    = regexp.MustCompile(`(?s)<core-tile\s+data-recipe-id="([^"]+)"[^>]*>(.*?)</core-tile>`)
	tileTitleRe   = regexp.MustCompile(`class="core-tile__description-text">([^<]+)<`)
)

func parseWeekplanHTML(html string) []api.PlanDay {
	var days []api.PlanDay
	for _, m := range dayBlockRe.FindAllStringSubmatch(html, -1) {
		date := m[1]
		block := m[2]

		dayName := ""
		if mn := dayShortRe.FindStringSubmatch(block); len(mn) > 1 {
			dayName = strings.TrimSpace(mn[1])
		}
		dayNumber := ""
		if mn := dayNumberRe.FindStringSubmatch(block); len(mn) > 1 {
			dayNumber = strings.TrimSpace(mn[1])
		}
		isToday := strings.Contains(block, "my-week__today") || strings.Contains(block, ">Heute<")

		recipes := []api.PlanRecipe{}
		for _, tm := range coreTileRe.FindAllStringSubmatch(block, -1) {
			recipeID := tm[1]
			recipeBlock := tm[2]
			title := ""
			if tn := tileTitleRe.FindStringSubmatch(recipeBlock); len(tn) > 1 {
				title = strings.TrimSpace(tn[1])
			}
			if title != "" {
				recipes = append(recipes, api.PlanRecipe{
					ID:    recipeID,
					Title: title,
					URL:   fmt.Sprintf("%s/recipes/recipe/%s/%s", cookidooBase, locale, recipeID),
				})
			}
		}

		days = append(days, api.PlanDay{
			Date:      date,
			DayName:   dayName,
			DayNumber: dayNumber,
			IsToday:   isToday,
			Recipes:   recipes,
		})
	}
	return days
}

func ensurePrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
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

// marshalPlan is used by status.go
func planCacheInfo() string {
	var days []api.PlanDay
	if err := config.LoadCache("weekplan.json", &days); err != nil {
		return "no cache"
	}
	b, _ := json.Marshal(map[string]int{"days": len(days)})
	return string(b)
}
