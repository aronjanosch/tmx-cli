# tmx-cli

Thermomix/Cookidoo CLI — meal plans, recipe search, and shopping lists from your terminal.

> Unofficial hobby project. Not affiliated with or endorsed by Vorwerk/Cookidoo®.

## Install

**Homebrew** (macOS/Linux):
```bash
brew tap aronjanosch/tap
brew install tmx-cli
```

**From source** (requires Go 1.21+):
```bash
git clone https://github.com/aronjanosch/tmx-cli
cd tmx-cli
go build -o tmx .
sudo mv tmx /usr/local/bin/
```

**Pre-built binaries** on the [Releases](https://github.com/aronjanosch/tmx-cli/releases) page (Linux, macOS, Windows — amd64/arm64).

## Setup

```bash
tmx login               # OAuth login with your Cookidoo account
tmx setup               # Configure TM version, diet preference, max cooking time
tmx status              # Check login and config
```

Config is stored at `~/.config/tmx/`.

## Commands

> `-j` / `--json` is a **root-level** flag: `tmx -j search pasta`, not `tmx search pasta -j`

### search
```bash
tmx search "pasta"                        # search recipes
tmx search "curry" -n 20                  # more results
tmx search "salad" -t 15                  # max 15 minutes
tmx search "" -c vegetarisch              # browse by category
tmx search "soup" --tm TM6               # filter by TM version
```

### recipe
```bash
tmx recipe show <id>                      # ingredients, steps, nutrition
```

### plan
```bash
tmx plan sync                             # sync from Cookidoo
tmx plan show                             # show current week (from cache)
tmx plan add <id> <day>                   # add recipe (mon/tue/wed/thu/fri/sat/sun)
tmx plan remove <id> <day>
tmx plan move <id> <from> <to>
tmx today                                 # today's recipes only
```

### shopping
```bash
tmx shopping show                         # current list
tmx shopping from-plan                    # generate from meal plan
tmx shopping add <recipe-id>              # add recipe ingredients
tmx shopping add-item "milk" "bread"      # add custom items
tmx shopping remove <recipe-id>
tmx shopping clear
tmx shopping export -f markdown           # export (text/markdown/json)
```

### favorites
```bash
tmx favorites show
tmx favorites add <id>
tmx favorites remove <id>
```

### collections
```bash
tmx collections search "pasta"            # search public collections
tmx collections list                      # your saved collections
tmx collections show <id>                 # collection details + recipes
```

### categories
```bash
tmx categories show                       # list categories
tmx categories sync                       # fetch from Cookidoo
```

### other
```bash
tmx status                                # login + config info
tmx cache clear                           # clear cached data
tmx setup [--tm TM6] [--diet vegetarisch] [--max-time 30]
```

## JSON output

Every command supports `-j` for clean, typed JSON — useful for scripting and AI agents:

```bash
tmx -j search "pasta" -n 5
tmx -j plan show
tmx -j shopping show
tmx -j recipe show r130616
```

Schema: `{"data": [...], "count": N}` for lists, typed object for single items.

## Releasing a new version

```bash
git tag v0.x.0
GITHUB_TOKEN=$(gh auth token) goreleaser release --clean
```

Builds binaries for all platforms, publishes a GitHub release, and updates the Homebrew formula in `aronjanosch/homebrew-tap` automatically.

## License

MIT © aronjanosch
