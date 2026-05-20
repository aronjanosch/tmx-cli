package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)

type RecipeCmd struct {
	Show RecipeShowCmd `cmd:"" help:"Show recipe details."`
}

type RecipeShowCmd struct {
	ID          string `arg:"" help:"Recipe ID (e.g. r130616)."`
	Ingredients bool   `short:"i" help:"Include ingredients."`
	Steps       bool   `short:"s" help:"Include preparation steps."`
	Nutrition   bool   `short:"n" help:"Include nutrition per serving."`
	Full        bool   `short:"f" help:"Include all sections (ingredients, steps, nutrition)."`
}

func (r *RecipeShowCmd) Run(ctx *Context) error {
	id := r.ID
	if !strings.HasPrefix(id, "r") {
		id = "r" + id
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	raw, err := cl.Get(fmt.Sprintf("/recipes/recipe/%s/%s", locale, id))
	if err != nil {
		return fmt.Errorf("fetching recipe: %w", err)
	}

	if ctx.JSON {
		fmt.Println(string(raw))
		return nil
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing recipe: %w", err)
	}

	sections := recipeSections{
		Ingredients: r.Ingredients || r.Full,
		Steps:       r.Steps || r.Full,
		Nutrition:   r.Nutrition || r.Full,
	}
	printRecipe(data, sections)
	return nil
}

type recipeSections struct {
	Ingredients bool
	Steps       bool
	Nutrition   bool
}

func printRecipe(data map[string]any, show recipeSections) {
	title, _ := data["title"].(string)
	difficulty, _ := data["difficulty"].(string)

	// TM versions
	var versions []string
	if vs, ok := data["thermomixVersions"].([]any); ok {
		for _, v := range vs {
			if s, ok := v.(string); ok {
				versions = append(versions, s)
			}
		}
	}

	// Times
	var activeTime, totalTime int
	if times, ok := data["times"].([]any); ok {
		for _, t := range times {
			m, _ := t.(map[string]any)
			typ, _ := m["type"].(string)
			qty, _ := m["quantity"].(map[string]any)
			val, _ := qty["value"].(float64)
			switch typ {
			case "activeTime":
				activeTime = int(val) / 60
			case "totalTime":
				totalTime = int(val) / 60
			}
		}
	}

	// Servings
	var servings int
	if ss, ok := data["servingSize"].(map[string]any); ok {
		if qty, ok := ss["quantity"].(map[string]any); ok {
			if v, ok := qty["value"].(float64); ok {
				servings = int(v)
			}
		}
	}

	// Header + meta (always shown)
	width := max(44, len(title)+4)
	fmt.Println()
	fmt.Println("+" + strings.Repeat("-", width) + "+")
	fmt.Printf("|  %-*s|\n", width-2, title)
	fmt.Println("+" + strings.Repeat("-", width) + "+")
	fmt.Println()

	if totalTime > 0 {
		fmt.Printf("Time:        %s", formatTime(totalTime*60))
		if activeTime > 0 {
			fmt.Printf(" (active: %s)", formatTime(activeTime*60))
		}
		fmt.Println()
	}
	if servings > 0 {
		fmt.Printf("Servings:    %d\n", servings)
	}
	if difficulty != "" {
		fmt.Printf("Difficulty:  %s\n", difficulty)
	}
	if len(versions) > 0 {
		fmt.Printf("Versions:    %s\n", strings.Join(versions, ", "))
	}

	// Ingredients
	if show.Ingredients {
		if groups, ok := data["recipeIngredientGroups"].([]any); ok && len(groups) > 0 {
			fmt.Println()
			fmt.Println("INGREDIENTS")
			fmt.Println(strings.Repeat("-", 40))
			for _, g := range groups {
				group, _ := g.(map[string]any)
				groupTitle, _ := group["title"].(string)
				if groupTitle != "" {
					fmt.Printf("\n%s:\n", groupTitle)
				}
				ingredients, _ := group["recipeIngredients"].([]any)
				for _, ing := range ingredients {
					item, _ := ing.(map[string]any)
					name, _ := item["ingredientNotation"].(string)
					unit, _ := item["unitNotation"].(string)
					prep, _ := item["preparation"].(string)

					var qty float64
					if qMap, ok := item["quantity"].(map[string]any); ok {
						qty, _ = qMap["value"].(float64)
					}

					line := "  "
					if qty > 0 {
						if qty == float64(int(qty)) {
							line += fmt.Sprintf("%d ", int(qty))
						} else {
							line += fmt.Sprintf("%.1f ", qty)
						}
					}
					if unit != "" {
						line += unit + " "
					}
					line += name
					if prep != "" {
						line += ", " + prep
					}
					fmt.Println(line)
				}
			}
		}
	}

	// Steps
	if show.Steps {
		if steps, ok := data["recipeSteps"].([]any); ok && len(steps) > 0 {
			fmt.Println()
			fmt.Println("PREPARATION")
			fmt.Println(strings.Repeat("-", 40))
			for i, s := range steps {
				sm, _ := s.(map[string]any)
				desc, _ := sm["description"].(string)
				if desc == "" {
					desc, _ = sm["text"].(string)
				}
				if desc != "" {
					fmt.Printf("  %d. %s\n", i+1, desc)
				}
			}
		}
	}

	// Nutrition
	if show.Nutrition {
		nutrition := parseNutrition(data)
		if len(nutrition) > 0 {
			fmt.Println()
			fmt.Println("NUTRITION (per serving)")
			fmt.Println(strings.Repeat("-", 40))
			order := []string{"kcal", "kJ", "protein", "carb", "carb2", "fat", "dietaryFibre"}
			printed := map[string]bool{}
			for _, k := range order {
				if v, ok := nutrition[k]; ok {
					label := nutritionLabel(k)
					fmt.Printf("  %-14s %s\n", label+":", v)
					printed[k] = true
				}
			}
			for k, v := range nutrition {
				if !printed[k] {
					fmt.Printf("  %-14s %s\n", k+":", v)
				}
			}
		}
	}

	fmt.Println()
}

func parseNutrition(data map[string]any) map[string]string {
	result := map[string]string{}
	groups, ok := data["nutritionGroups"].([]any)
	if !ok {
		return result
	}
	for _, g := range groups {
		group, _ := g.(map[string]any)
		for _, rn := range asList(group["recipeNutritions"]) {
			rnMap, _ := rn.(map[string]any)
			for _, n := range asList(rnMap["nutritions"]) {
				nMap, _ := n.(map[string]any)
				typ, _ := nMap["type"].(string)
				num := parseNumber(nMap["number"])
				unit := firstString(nMap["unittype"], nMap["unitType"])
				if typ != "" && num > 0 {
					result[typ] = fmt.Sprintf("%.0f %s", num, strings.TrimSpace(unit))
				}
			}
		}
	}
	return result
}

func parseNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		var f float64
		fmt.Sscanf(x, "%f", &f)
		return f
	}
	return 0
}

func firstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func nutritionLabel(key string) string {
	labels := map[string]string{
		"kcal":         "Calories",
		"kJ":           "Energy (kJ)",
		"protein":      "Protein",
		"carb":         "Carbs",
		"carb2":        "Carbs",
		"fat":          "Fat",
		"dietaryFibre": "Fiber",
	}
	if l, ok := labels[key]; ok {
		return l
	}
	return key
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
