package cmd

import (
	"fmt"
	"time"
)

type TodayCmd struct{}

func (t *TodayCmd) Run(ctx *Context) error {
	days, err := loadPlanCache()
	if err != nil {
		return fmt.Errorf("no cached plan — run: tmx plan sync")
	}

	today := time.Now().Format("2006-01-02")
	for _, d := range days {
		if d.Date == today {
			if ctx.JSON {
				return ctx.PrintJSON(d)
			}
			if len(d.Recipes) == 0 && len(d.CustomRecipeIDs) == 0 {
				fmt.Println("No recipes planned for today.")
				return nil
			}
			fmt.Printf("Today (%s):\n", today)
			for _, r := range d.Recipes {
				fmt.Printf("  %-12s  %s\n", r.ID, r.Title)
			}
			for _, id := range d.CustomRecipeIDs {
				fmt.Printf("  %-12s  (custom recipe)\n", id)
			}
			return nil
		}
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"date": today, "recipes": []any{}})
	}
	fmt.Println("No recipes planned for today.")
	return nil
}
