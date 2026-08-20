# AGENTS.md

Guidelines for coding agents working on tmx-cli.

## Rules

**Make it simple. Make it easy. Make it work first.**

- Small iteration steps — review every step
- Simplest solution first; edge cases later
- Easy-to-read code; avoid bloat
- Comment only when necessary
- Simple code over clever one-liners
- Rebuild clean; don't carry over complexity

## Project

Go CLI for Thermomix/Cookidoo (meal plans, recipe search, shopping lists).
Designed for both human use and AI agents (OpenClaw).

- Binary: `tmx`
- Module: `github.com/aronjanosch/tmx-cli`
- Framework: [Kong](https://github.com/alecthomas/kong) (declarative CLI via struct tags)
- Config/cache: `~/.config/tmx/`

## Output

Two modes, controlled by `-j` / `--json` flag:

| Mode | Default | Format |
|---|---|---|
| Human | yes | Formatted tables, plain text |
| JSON | `-j` flag | Typed structs, consistent schema |

JSON success: `{"data": [...], "count": N}`
JSON error: `{"error": "message"}`

Errors go to stderr. JSON errors go to stdout (so agents can parse them).
Exit codes: 0 ok, 1 error, 3 session expired (`client.ErrUnauthorized`).
Auto re-login on 401 when credentials are stored (`tmx login --save`).

## Code conventions

- Use typed Go structs (`internal/api/`) — no `map[string]any` for API responses
- All commands implement `Run(ctx *Context) error`
- `Context.Client()` lazy-inits the HTTP client with stored cookies
- `Context.PrintJSON(v)` for all JSON output
- HTTP client lives in `internal/client/`, config in `internal/config/`

## API field names

Never assume field names match intuition. Always verify against a live response before writing a parser.

```bash
go run . -j recipe show r391733 --full | python3 -c "import json,sys; [print(k) for k in sorted(json.load(sys.stdin))]"
```

Known surprises:
- Steps: `recipeStepGroups[].recipeSteps[].formattedText` (not `recipeSteps[].description`)
- Nutrition number: may be string or float — parse both
- Nutrition unit: `unittype` or `unitType` — check both casings
- Shopping list `recipeIngredientGroups` is a **flat item list**, not groups
- Shopping remove-recipe wants the list-entry **ULID**, not the rNNN id (`shopping/{lang}/recipes/remove`)
- Quantities: `{"value": N}` or range `{"from": N, "to": M}`
- `totalTime` in planning/collection/shopping responses is a decimal **string** ("1500.0"); in custom recipes ISO-8601 ("PT30M") or seconds — `api.FlexDuration` parses both
- Custom recipe content: keys vary (`ingredients`/`recipeIngredient`, `tools`/`tool`, `yield`/`recipeYield`); items are strings or `{"text": ...}` — `api.FlexTextList`
- Organize + created-recipes endpoints need vendor `Accept` headers (see cmd/collections.go, cmd/import.go)
- Custom recipe creation is two-step: POST name → wait ~3s → PATCH content (backend propagation)
- TTS annotation offsets count unicode code points, not bytes
- Custom recipe `tools` only accepts TM versions (TM5/TM6/TM7); other equipment → 400 validationError
- TTS `speed` enum has no "Turbo"; send `{"turbo": true}` instead (backend drops it, but the step text keeps the notation)
- Custom recipe GET: plain `application/json` view omits `hints`/annotations — use the vendor Accept for full content
- Search image URLs contain literal `{assethost}`/`{transformation}` placeholders — substitute `assets.tmecosys.com` / `t_web_shared_recipe_221x240`
- Native `/search/{lang}` `ingredients`/`excludeIngredients` params only match a curated facet vocabulary ("Kürbis" works, "Kartoffel" doesn't) — tmx verifies ingredients client-side instead
- `/community/profile` + shopping/planning writes accept any of the three session cookies being stale as long as `_oauth2_proxy` refreshes; a full 401 triggers tmx auto-relogin (~1s) when credentials are stored
- Endpoint/payload reference: [miaucl/cookidoo-api](https://github.com/miaucl/cookidoo-api) `const.py` + `docs/raw-api-requests/`

## Context efficiency

Commands that return potentially large data must support section flags in **both** human and JSON modes.

Pattern: `-i/--ingredients`, `-s/--steps`, `-n/--nutrition`, `--full`. Default = meta only.

JSON mode must filter the response object — not just skip printing. An agent receiving unrequested steps burns 2–3k tokens it didn't ask for.

## Text fields

Cookidoo API text fields (e.g. `formattedText`) contain HTML tags (`<NOBR>`, `<nobr>`, etc.). Strip before displaying:

```go
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)
text = strings.TrimSpace(htmlTagRe.ReplaceAllString(text, ""))
```

## Testing

After implementing any feature:

1. **End-to-end test** — run the actual command against the live API, not just `go build`
2. **Verify field names** — print all keys from a live response before writing any parser
3. **Agent usefulness rating** — critically ask: *"If I were an AI assistant needing this data, how useful is this output?"*
   - Is the JSON schema clean and predictable?
   - Are IDs included so I can chain commands?
   - Are errors clear and actionable?
   - Would I need to scrape or guess anything?

A feature that builds but returns unparseable or incomplete data is not done.

4. **Update SKILL.md** — after any change to commands, flags, output format, or JSON schema, check if SKILL.md needs updating. Agents using the skill get stale docs otherwise.

## Commit Style

- Short, imperative, one line; no period
- No `Co-Authored-By` trailers
- Example: `feat: add plan sync command`

## What not to do

- No TTY detection — tmx only calls APIs, no interactive pickers
- No silent failures — errors always surface
- No `map[string]any` for structured data (breaks agent parsing)
- No truncating output without the user knowing
