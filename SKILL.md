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
   - **Unattended**: `tmx login --save` stores credentials (0600) — expired sessions then re-login automatically. `tmx login --forget` removes them.
3. Configure (optional): `tmx setup` or `tmx setup --tm TM6 --diet vegetarisch --max-time 30`

## Critical Rules

1. **Confirm before destructive actions** (shopping clear, plan remove, recipe/collection delete).
2. **Use `-j`** when parsing output programmatically — always place right after `tmx`.
3. **Respect user preferences** — `tmx setup` config auto-applies to searches (check the `applied` object in search output; `--no-prefs` disables).

## Exit codes

- `0` success, `1` error (`{"error": ...}` on stdout in `-j` mode)
- `3` session expired → run `tmx login` (or enable `tmx login --save` once)

## CLI Usage

```
tmx [--json] <command> [subcommand] [args] [flags]
```

## Core Workflows

### Search Recipes
```bash
tmx -j search "Pasta"
tmx -j search "Kuchen" -n 20
tmx -j search "Suppe" -t 30                  # max 30 min
tmx -j search "Salat" -c vegetarisch         # by category
tmx -j search "" --tm TM6                    # by TM version
tmx -j search "" --min-rating 4              # well-rated only
tmx -j search "" -I Kürbis -I Zwiebel        # must use these ingredients
tmx -j search "Curry" -x Fleisch             # must NOT use this ingredient
```

Filters: `-t <minutes>`, `--tm TM5|TM6|TM7`, `-c <category>`, `--min-rating <0-5>`,
`-I/--ingredient` (repeatable), `-x/--exclude` (repeatable), `--no-prefs`.

Setup preferences (TM version, max time, diet) auto-apply; the JSON output echoes
an `applied` object with what was actually sent. `--no-prefs` disables this.
An unknown `-c` value errors and lists valid categories.

> **Note:** Search results contain `id, title, totalTimeMinutes, rating` only — no
> nutrition data. To filter by macros, batch-fetch: `tmx -j recipe show <id...> -n`.

Categories: vorspeisen, suppen, pasta, fleisch, fisch, vegetarisch, beilagen, desserts, herzhaft-backen, kuchen, brot, getraenke, grundrezepte, saucen, snacks (refresh: `tmx categories sync`)

### Recipe Details

Default output: meta only (id, title, times, servings, difficulty, categories). Add sections explicitly to control context size. **Multiple IDs are allowed** — one call compares candidates.

```bash
tmx -j recipe show <id>              # meta only — minimal context
tmx -j recipe show <id> -n           # + nutrition (kcal, protein, fat, carbs)
tmx -j recipe show <id> -i           # + ingredients (for shopping)
tmx -j recipe show <id> -s           # + preparation steps
tmx -j recipe show id1 id2 id3 -n    # batch: nutrition for several candidates at once
tmx -j recipe show <id> --full       # all sections
tmx -j recipe show <id> --raw        # unmodified Cookidoo API response (debugging)
```

> **Context efficiency:** Prefer `-n` when you only need macros. Avoid `--full` unless all sections are required.

## Recommendation Workflows

**"I have 600 kcal left today — what can I eat?"**
```bash
tmx -j search "Abendessen" -n 8 --min-rating 4        # candidates (respects prefs)
tmx -j recipe show r1 r2 r3 r4 -n                     # batch nutrition, one call
# pick recipes where nutrition.kcal <= 600, present 2-3 options with kcal/protein
```

**"I have these leftover ingredients — anything we can make?"**
```bash
tmx -j search "" -I Kürbis -I Kartoffel -x Sahne -n 8
tmx -j recipe show <top_ids> -i                       # check what else is needed
# present options; mention missing ingredients
```

**"Something vegetarian / fitness / low carb"**
```bash
tmx -j search "Low Carb" -c vegetarisch -n 8          # category + free-text
tmx -j recipe show <ids> -n                           # verify macros before recommending
```

After the user picks: `tmx plan add <id>` (today) and optionally `tmx shopping add <id>`.
Nothing beats checking macros — search text alone does not guarantee "low carb".

### Import Any Recipe (custom recipes)

Import recipes from any website or from structured JSON into Cookidoo "own recipes".

```bash
tmx -j recipe fetch <url>                  # scrape site (schema.org) → import JSON, no upload
tmx recipe import recipe.json              # upload structured recipe
tmx recipe import - < recipe.json          # from stdin
tmx recipe import --url <url>              # scrape + upload directly (steps without TTS)
tmx recipe import recipe.json --dry-run    # preview exact Cookidoo payload
tmx -j recipe mine                         # list own recipes (IDs are ULIDs)
tmx recipe delete <ulid>                   # confirm first!
tmx recipe copy r130616 -s 2               # copy Cookidoo recipe to own recipes, rescaled
```

**Recommended agent workflow:** `recipe fetch <url>` → refine the JSON (add Thermomix TTS
parameters per step, adjust ingredients) → `recipe import file.json`. You are the converter;
put time/temp/speed on every step that runs in the Thermomix.

Import JSON schema (times in minutes, step TTS optional):

```json
{
  "title": "Kürbissuppe",
  "servings": 4,
  "prepTimeMinutes": 10,
  "totalTimeMinutes": 35,
  "tools": ["Varoma"],
  "ingredients": ["500 g Kürbis, in Stücken", "1 Zwiebel"],
  "steps": [
    {"text": "Zwiebel zerkleinern", "timeSeconds": 5, "speed": "5"},
    {"text": "Kürbis zugeben, garen", "timeSeconds": 1200, "temp": "100", "speed": "1"},
    "Abschmecken und servieren."
  ],
  "hints": "Mit Kürbiskernöl servieren.",
  "source": "https://original.url"
}
```

TTS rules: `temp` is `"37"`–`"120"` (°C) or `"varoma"`; `speed` is `"0.5"`–`"10"`
(dot decimal) or `"turbo"`; `timeSeconds` is int seconds. The CLI appends German
notation ("20 Min./100°C/Stufe 1") to the step text and generates Cookidoo TTS
annotations. Steps may be plain strings (no Thermomix action). TM version is taken
from `tmx setup`; other `tools` entries (e.g. "Varoma") are moved into the hints text
(the API only accepts TM versions there). Quantities belong in `ingredients`, not in
step text (Cookidoo convention: "den Kürbis zugeben", not "500 g Kürbis zugeben").

### Meal Plan
```bash
tmx -j plan show                                          # current week plan (from cache) — includes syncedAt
tmx plan sync                                             # sync from Cookidoo first
tmx plan add <recipe_id> --date=YYYY-MM-DD                # default: today
tmx plan add <ulid> --date=YYYY-MM-DD --custom            # plan an own (custom) recipe
tmx plan remove <recipe_id> --date=YYYY-MM-DD
tmx plan move <recipe_id> --from=YYYY-MM-DD --to=YYYY-MM-DD
tmx -j today                                  # today's recipes only
```

### Shopping List
```bash
tmx -j shopping show                # structured list; every item has an id + isOwned
tmx shopping from-plan              # generate from current meal plan
tmx shopping add <recipe_id>        # add recipe ingredients (--custom for own recipes)
tmx shopping remove <recipe_id>     # accepts rNNN or shopping-list ULID
tmx shopping check <item_id>...     # mark owned (ingredient or custom item ids)
tmx shopping uncheck <item_id>...
tmx shopping add-item "Milk" "Eggs" # add custom items (returns their ids)
tmx shopping edit-item <id> "Oat milk"
tmx shopping remove-item <id>...
tmx shopping clear                  # confirm first!
tmx shopping export -f markdown     # export (text/markdown/json)
```

### Account
```bash
tmx -j whoami                       # username, id, subscription status
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
tmx -j collections list              # your saved public collections
tmx -j collections show <id>         # collection details + recipe list
tmx collections save col500561       # save a public collection to account
tmx collections unsave col500561
tmx -j collections mine              # your own recipe lists
tmx collections create "Weekend"     # create own list (returns id)
tmx collections add-recipe <list_id> r130616 r391733
tmx collections remove-recipe <list_id> r130616
tmx collections delete <list_id>     # confirm first!
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
{"data": [{date, dayName, isToday, recipes: [...], customRecipeIds: [...]}], "syncedAt": "2026-01-01T00:00:00Z"}

// shopping show — item ids are ULIDs, needed for check/uncheck
{"data": [{id, name, quantity, unit, isOwned, recipeId}], "recipes": [{id, title, ulid}], "additionalItems": [{id, name, isOwned}], "count": N}

// recipe mine
{"data": [{id, title, modified, url}], "count": N, "limit": 150}

// recipe import
{"status": "imported", "id": "<ulid>", "title": "...", "url": "https://cookidoo.de/recipes/custom-recipes/<ulid>"}

// search results — totalTimeMinutes is in minutes (not seconds)
{"data": [{id, title, url, totalTimeMinutes, rating}], "count": N, "total": N,
 "applied": {query, category, maxTimeMinutes, tm, diet, ingredients, ...}}

// recipe show — compact schema; sections only present when flag requested
// single ID → one object; multiple IDs → {"data": [...], "count": N}
{
  "id": "r391733", "title": "...", "url": "...",
  "servings": 4, "totalTimeMinutes": 40, "activeTimeMinutes": 15,
  "difficulty": "easy", "tmVersions": ["TM6"], "categories": ["..."],
  "ingredients": [{name, quantity, unit, preparation, optional, group}],   // -i
  "steps": [{text, group}],                                                // -s
  "nutrition": {"kcal": "350 kcal", "protein": "12 g", ...}                // -n
}

// mutations
{"status": "added|removed|moved", "id": "r123"}

// errors (stdout in -j mode)
{"error": "message"}
```

## Chaining Example

```bash
# Find a vegetarian pasta recipe under 30 min and add to Thursday
ID=$(tmx -j search "pasta" -t 30 -c vegetarisch -n 1 | jq -r '.data[0].id')
tmx plan add "$ID" --date=2026-08-27
tmx -j recipe show "$ID" -n   # confirm nutrition only
tmx -j recipe show "$ID" --full   # or get everything
```
