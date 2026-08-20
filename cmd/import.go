package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aronjanosch/tmx-cli/internal/api"
)

// ── Import input schema (agent-facing) ───────────────────────────────────────

// ImportRecipe is the input schema for `tmx recipe import`.
// Times are in minutes; step TTS parameters are explicit fields so the
// calling agent does the semantic conversion, not fragile keyword heuristics.
type ImportRecipe struct {
	Title            string       `json:"title"`
	Servings         int          `json:"servings"`
	PrepTimeMinutes  int          `json:"prepTimeMinutes"`
	TotalTimeMinutes int          `json:"totalTimeMinutes"`
	Tools            []string     `json:"tools,omitempty"`
	Ingredients      []string     `json:"ingredients"`
	Steps            []ImportStep `json:"steps"`
	Hints            string       `json:"hints,omitempty"`
	Source           string       `json:"source,omitempty"`
}

// ImportStep is one preparation step. TTS fields are optional; when set,
// the Thermomix notation (e.g. "3 Min./100°C/Stufe 2") is appended to the
// text and a structured TTS annotation is generated.
type ImportStep struct {
	Text        string `json:"text"`
	TimeSeconds int    `json:"timeSeconds,omitempty"`
	Temp        string `json:"temp,omitempty"`  // "37".."120" (°C) or "varoma"
	Speed       string `json:"speed,omitempty"` // "0.5".."10" or "turbo"
}

// Steps may be given as plain strings or objects.
func (s *ImportStep) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		s.Text = text
		return nil
	}
	type plain ImportStep
	return json.Unmarshal(data, (*plain)(s))
}

func (r *ImportRecipe) validate() error {
	if strings.TrimSpace(r.Title) == "" {
		return fmt.Errorf("recipe title is required")
	}
	if len(r.Ingredients) == 0 {
		return fmt.Errorf("at least one ingredient is required")
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	for i, st := range r.Steps {
		if strings.TrimSpace(st.Text) == "" {
			return fmt.Errorf("step %d has empty text", i+1)
		}
		if st.Temp != "" && !strings.EqualFold(st.Temp, "varoma") {
			t, err := strconv.Atoi(st.Temp)
			if err != nil || t < 37 || t > 120 {
				return fmt.Errorf("step %d: temp must be 37-120 or \"varoma\", got %q", i+1, st.Temp)
			}
		}
		if st.Speed != "" && !strings.EqualFold(st.Speed, "turbo") {
			v, err := strconv.ParseFloat(st.Speed, 64)
			if err != nil || v < 0.5 || v > 10 {
				return fmt.Errorf("step %d: speed must be \"0.5\"-\"10\" (dot decimal) or \"turbo\", got %q", i+1, st.Speed)
			}
		}
	}
	if r.Servings <= 0 {
		r.Servings = 4
	}
	if r.TotalTimeMinutes <= 0 {
		r.TotalTimeMinutes = 45
	}
	if r.PrepTimeMinutes <= 0 {
		r.PrepTimeMinutes = 15
	}
	return nil
}

// ── Cookidoo payload builder ─────────────────────────────────────────────────

var tmVersionRe = regexp.MustCompile(`(?i)^TM\d+$`)

// ttsText renders TTS parameters in German Cookidoo notation:
// "3 Min./100°C/Stufe 2", "30 Sek./Stufe 5", "10 Min./Varoma/Stufe 1".
func (s ImportStep) ttsText() string {
	var parts []string
	if s.TimeSeconds > 0 {
		mins, secs := s.TimeSeconds/60, s.TimeSeconds%60
		switch {
		case mins > 0 && secs > 0:
			parts = append(parts, fmt.Sprintf("%d Min. %d Sek.", mins, secs))
		case mins > 0:
			parts = append(parts, fmt.Sprintf("%d Min.", mins))
		default:
			parts = append(parts, fmt.Sprintf("%d Sek.", secs))
		}
	}
	if strings.EqualFold(s.Temp, "varoma") {
		parts = append(parts, "Varoma")
	} else if s.Temp != "" {
		parts = append(parts, s.Temp+"°C")
	}
	if strings.EqualFold(s.Speed, "turbo") {
		parts = append(parts, "Turbo")
	} else if s.Speed != "" {
		parts = append(parts, "Stufe "+s.Speed)
	}
	return strings.Join(parts, "/")
}

// buildPayload converts an ImportRecipe into the Cookidoo custom-recipe
// PATCH payload, including TTS annotations with character offsets.
func buildPayload(r *ImportRecipe, tmVersion string) map[string]any {
	// The API only accepts TM versions in "tools" (anything else → 400
	// validationError). Other equipment goes into the hints text.
	tools := []string{tmVersion}
	var extraTools []string
	for _, t := range r.Tools {
		if tmVersionRe.MatchString(t) {
			if !strings.EqualFold(t, tmVersion) {
				tools = append(tools, strings.ToUpper(t))
			}
		} else {
			extraTools = append(extraTools, t)
		}
	}

	ingredients := make([]map[string]any, len(r.Ingredients))
	for i, ing := range r.Ingredients {
		ingredients[i] = map[string]any{"type": "INGREDIENT", "text": ing}
	}

	instructions := make([]map[string]any, len(r.Steps))
	for i, st := range r.Steps {
		text := strings.TrimSpace(st.Text)
		annotations := []map[string]any{}

		if tts := st.ttsText(); tts != "" {
			// TTS string is appended after the description with one space;
			// offsets count unicode code points, not bytes.
			offset := utf8.RuneCountInString(text) + 1
			text = text + " " + tts

			// "Turbo" is not an allowed speed enum value; the API accepts a
			// turbo flag instead (and the step text carries the notation).
			data := map[string]any{}
			if strings.EqualFold(st.Speed, "turbo") {
				data["turbo"] = true
			} else if st.Speed != "" {
				data["speed"] = st.Speed
			}
			if st.TimeSeconds > 0 {
				data["time"] = st.TimeSeconds
			}
			if strings.EqualFold(st.Temp, "varoma") {
				data["temperature"] = map[string]any{"value": "varoma", "unit": "C"}
			} else if st.Temp != "" {
				data["temperature"] = map[string]any{"value": st.Temp, "unit": "C"}
			}

			annotations = append(annotations, map[string]any{
				"type":     "TTS",
				"position": map[string]any{"offset": offset, "length": utf8.RuneCountInString(tts)},
				"data":     data,
			})
		}

		instructions[i] = map[string]any{
			"type":         "STEP",
			"text":         text,
			"annotations":  annotations,
			"missedUsages": []any{},
		}
	}

	hints := r.Hints
	if len(extraTools) > 0 {
		if hints != "" {
			hints += "\n"
		}
		hints += "Zubehör: " + strings.Join(extraTools, ", ")
	}
	if r.Source != "" {
		if hints != "" {
			hints += "\n"
		}
		hints += "Original: " + r.Source
	}

	return map[string]any{
		"name":           r.Title,
		"image":          nil,
		"tools":          tools,
		"yield":          map[string]any{"value": r.Servings, "unitText": "portion"},
		"prepTime":       r.PrepTimeMinutes * 60,
		"cookTime":       0,
		"totalTime":      r.TotalTimeMinutes * 60,
		"ingredients":    ingredients,
		"instructions":   instructions,
		"hints":          hints,
		"workStatus":     "PRIVATE",
		"recipeMetadata": map[string]any{"requiresAnnotationsCheck": false},
	}
}

// ── Commands ─────────────────────────────────────────────────────────────────

type RecipeImportCmd struct {
	File   string `arg:"" optional:"" help:"Recipe JSON file (or - for stdin)."`
	URL    string `short:"u" help:"Import directly from a recipe website URL (schema.org)."`
	DryRun bool   `name:"dry-run" help:"Print the Cookidoo payload without uploading."`
}

func (c *RecipeImportCmd) Run(ctx *Context) error {
	var recipe *ImportRecipe
	var err error

	switch {
	case c.URL != "":
		recipe, err = scrapeRecipe(c.URL)
		if err != nil {
			return fmt.Errorf("scraping %s: %w", c.URL, err)
		}
	case c.File == "-" || c.File == "":
		if c.File == "" {
			return fmt.Errorf("provide a recipe JSON file, - for stdin, or --url")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		recipe = &ImportRecipe{}
		if err := json.Unmarshal(data, recipe); err != nil {
			return fmt.Errorf("parsing recipe JSON: %w", err)
		}
	default:
		data, err := os.ReadFile(c.File)
		if err != nil {
			return err
		}
		recipe = &ImportRecipe{}
		if err := json.Unmarshal(data, recipe); err != nil {
			return fmt.Errorf("parsing recipe JSON: %w", err)
		}
	}

	if err := recipe.validate(); err != nil {
		return err
	}

	tm := ctx.Config.TMVersion
	if tm == "" {
		tm = "TM6"
	}
	payload := buildPayload(recipe, tm)

	if c.DryRun {
		return ctx.PrintJSON(payload)
	}

	cl, err := ctx.Client()
	if err != nil {
		return err
	}

	// Step 1: create the recipe shell (name only) to obtain the ID.
	raw, err := cl.PostJSON(fmt.Sprintf("/created-recipes/%s", locale), map[string]string{
		"recipeName": recipe.Title,
	})
	if err != nil {
		return fmt.Errorf("creating recipe: %w", err)
	}
	var created struct {
		RecipeID string `json:"recipeId"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.RecipeID == "" {
		return fmt.Errorf("create response missing recipeId: %s", string(raw))
	}

	// Step 2: the backend needs a moment before the recipe accepts content.
	time.Sleep(3 * time.Second)

	if _, err := cl.Request("PATCH", fmt.Sprintf("/created-recipes/%s/%s", locale, created.RecipeID), payload, nil); err != nil {
		return fmt.Errorf("uploading recipe content (recipe shell %s was created): %w", created.RecipeID, err)
	}

	url := fmt.Sprintf("%s/recipes/custom-recipes/%s", cookidooBase, created.RecipeID)
	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{
			"status": "imported",
			"id":     created.RecipeID,
			"title":  recipe.Title,
			"url":    url,
		})
	}
	fmt.Printf("Imported %q as %s\n%s\n", recipe.Title, created.RecipeID, url)
	return nil
}

type RecipeFetchCmd struct {
	URL string `arg:"" help:"Recipe website URL."`
}

// Run scrapes a recipe site and prints the import-schema JSON so an agent
// (or human) can refine it — e.g. add TTS parameters — before importing.
func (c *RecipeFetchCmd) Run(ctx *Context) error {
	recipe, err := scrapeRecipe(c.URL)
	if err != nil {
		return fmt.Errorf("scraping %s: %w", c.URL, err)
	}
	return ctx.PrintJSON(recipe)
}

type RecipeMineCmd struct{}

func (c *RecipeMineCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	raw, err := cl.Request("GET", fmt.Sprintf("/created-recipes/%s", locale), nil,
		map[string]string{"Accept": "application/vnd.vorwerk.customer-recipe.full+json"})
	if err != nil {
		return fmt.Errorf("fetching own recipes: %w", err)
	}

	var list api.CustomRecipeList
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("parsing own recipes: %w", err)
	}

	type ownRecipe struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Modified string `json:"modified,omitempty"`
		URL      string `json:"url"`
	}
	out := make([]ownRecipe, 0, len(list.Items))
	for _, item := range list.Items {
		title := item.Content.Name
		if title == "" {
			title = item.Title
		}
		out = append(out, ownRecipe{
			ID:       item.RecipeID,
			Title:    title,
			Modified: item.ModifiedAt,
			URL:      fmt.Sprintf("%s/recipes/custom-recipes/%s", cookidooBase, item.RecipeID),
		})
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]any{"data": out, "count": len(out), "limit": list.Meta.RecipeLimit})
	}
	if len(out) == 0 {
		fmt.Println("No own recipes found.")
		return nil
	}
	fmt.Printf("%-28s  %s\n", "ID", "Title")
	fmt.Printf("%-28s  %s\n", "--", "-----")
	for _, r := range out {
		fmt.Printf("%-28s  %s\n", r.ID, r.Title)
	}
	fmt.Printf("\n%d of max %d recipes\n", len(out), list.Meta.RecipeLimit)
	return nil
}

type RecipeDeleteCmd struct {
	ID string `arg:"" help:"Custom recipe ID."`
}

func (c *RecipeDeleteCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	if _, err := cl.Request("DELETE", fmt.Sprintf("/created-recipes/%s/%s", locale, c.ID), nil, nil); err != nil {
		return fmt.Errorf("deleting recipe: %w", err)
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "deleted", "id": c.ID})
	}
	fmt.Printf("Deleted custom recipe %s.\n", c.ID)
	return nil
}

type RecipeCopyCmd struct {
	RecipeID string `arg:"" help:"Cookidoo recipe ID (rNNN) to copy to own recipes."`
	Servings int    `short:"s" help:"Rescale to this serving size."`
}

func (c *RecipeCopyCmd) Run(ctx *Context) error {
	cl, err := ctx.Client()
	if err != nil {
		return err
	}
	id := ensurePrefix(c.RecipeID, "r")
	payload := map[string]any{
		"recipeUrl": fmt.Sprintf("%s/recipes/recipe/%s/%s", cookidooBase, locale, id),
	}
	if c.Servings > 0 {
		payload["servingSize"] = c.Servings
	}

	raw, err := cl.PostJSON(fmt.Sprintf("/created-recipes/%s", locale), payload)
	if err != nil {
		return fmt.Errorf("copying recipe: %w", err)
	}
	var created struct {
		RecipeID string `json:"recipeId"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.RecipeID == "" {
		return fmt.Errorf("copy response missing recipeId: %s", string(raw))
	}

	if ctx.JSON {
		return ctx.PrintJSON(map[string]string{"status": "copied", "source": id, "id": created.RecipeID})
	}
	fmt.Printf("Copied %s to own recipes as %s.\n", id, created.RecipeID)
	return nil
}
