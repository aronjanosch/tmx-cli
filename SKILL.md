---
name: cookidoo
description: Manage Thermomix/Cookidoo meal planning via tmx-cli. Use for recipe search, weekly meal plan management, shopping list generation, favorites, and recipe details. Trigger when the user mentions Cookidoo, Thermomix, Wochenplan, meal plan, Rezept, recipe, or Einkaufsliste for cooking.
---

# Cookidoo / tmx-cli Skill

Manage Cookidoo® (Thermomix) meal plans, recipes, and shopping lists using `tmx` — a Go CLI.

## Setup

1. Install: `brew install aronjanosch/tap/tmx-cli` (or build from source)
2. Login:
   - Interactive: `tmx login`
   - Non-interactive (CI/agent): `TMX_EMAIL=x TMX_PASSWORD=y tmx login`
   - Pipe (safer for secrets): `echo "$PASS" | tmx login -e user@example.com --password-stdin`
3. Configure (optional): `tmx setup` or `tmx setup --tm TM6 --diet vegetarisch --max-time 30`

## Critical Rules

1. **Confirm before destructive actions** (shopping clear, plan remove).
2. **Use `-j`** when parsing output programmatically — always place right after `tmx`.
3. **Respect user preferences** — `tmx setup` config auto-applies to searches.

## CLI Usage

```
tmx [--json] <command> [subcommand] [args] [flags]
```

## Core Workflows

### Search Recipes
```bash
tmx -j search "Pasta"
tmx -j search "Kuchen" -n 20
tmx -j search "Suppe" -t 30           # max 30 min
tmx -j search "Salat" -c vegetarisch  # by category
tmx -j search "" --tm TM6             # by TM version
```

Filters: `-t <minutes>`, `--tm TM5|TM6|TM7`, `-c <category>`

> **Note:** Search results contain `id, title, totalTimeMinutes, rating` only — no nutrition data. The Algolia index does not include calories/protein. To filter by nutrition, fetch `recipe show <id>` for each candidate.

Categories: vorspeisen, suppen, pasta, fleisch, fisch, vegetarisch, beilagen, desserts, herzhaft-backen, kuchen, brot, getraenke, grundrezepte, saucen, snacks

### Recipe Details

Default output: meta only (title, time, servings, difficulty). Add sections explicitly to control context size.

```bash
tmx -j recipe show <id>          # meta only — minimal context
tmx -j recipe show <id> -n       # + nutrition (kcal, protein, fat, carbs)
tmx -j recipe show <id> -i       # + ingredients (for shopping)
tmx -j recipe show <id> -s       # + preparation steps
tmx -j recipe show <id> -n -i    # nutrition + ingredients
tmx -j recipe show <id> --full   # all sections (ingredients, steps, nutrition)
```

> **Context efficiency:** Prefer `-n` when you only need macros. Avoid `--full` unless all sections are required — steps alone can be 2–3k tokens.

### Meal Plan
```bash
tmx -j plan show                                          # current week plan (from cache) — includes syncedAt
tmx plan sync                                             # sync from Cookidoo first
tmx plan add <recipe_id> --date=YYYY-MM-DD                # default: today
tmx plan remove <recipe_id> --date=YYYY-MM-DD
tmx plan move <recipe_id> --from=YYYY-MM-DD --to=YYYY-MM-DD
tmx -j today                                  # today's recipes only
```

### Shopping List
```bash
tmx -j shopping show                # structured list with ingredients + recipes
tmx shopping from-plan              # generate from current meal plan
tmx shopping add <recipe_id>        # add recipe ingredients
tmx shopping add-item "Milk" "Eggs" # add custom items
tmx shopping remove <recipe_id>
tmx shopping clear                  # confirm first!
tmx shopping export -f markdown     # export (text/markdown/json)
```

### Favorites
```bash
tmx -j favorites show
tmx favorites add <recipe_id>
tmx favorites remove <recipe_id>
```

### Collections
```bash
tmx -j collections search "pasta"    # search public collections
tmx -j collections list              # your saved collections
tmx -j collections show <id>         # collection details + recipe list
```

### Categories
```bash
tmx -j categories show              # list all categories with IDs
tmx categories sync                 # fetch current from Cookidoo
```

## JSON Schema

```json
// list results
{"data": [...], "count": N, "total": N}

// plan show (cache)
{"data": [{date, dayName, isToday, recipes: [...]}], "syncedAt": "2026-01-01T00:00:00Z"}

// shopping show
{"data": [{name, quantity, unit, preparation, recipes:[...]}], "recipes": [...], "additional_items": [...], "count": N}

// search results — totalTimeMinutes is in minutes (not seconds)
{"data": [{id, title, url, totalTimeMinutes, rating}], "count": N, "total": N}

// recipe show — heavy fields only present when flag requested
// no flags:   {id, title, difficulty, times, servingSize, thermomixVersions, ...meta}
// -i flag:    + recipeIngredientGroups
// -s flag:    + recipeStepGroups
// -n flag:    + nutritionGroups
// --full:     all three included

// mutations
{"status": "added|removed|moved", "id": "r123"}

// errors (stdout in -j mode)
{"error": "message"}
```

## Chaining Example

```bash
# Find a vegetarian pasta recipe under 30 min and add to Thursday
ID=$(tmx -j search "pasta" -t 30 -c vegetarisch -n 1 | jq -r '.data[0].id')
tmx plan add "$ID" thu
tmx -j recipe show "$ID" -n   # confirm nutrition only
tmx -j recipe show "$ID" --full   # or get everything
```
