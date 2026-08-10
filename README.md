# agentpick

Launch coding agents with **bang-for-buck** defaults — one CLI, versioned model registry, Headroom-aware when available.

```bash
agentpick              # interactive picker
agentpick list         # show optimal defaults
agentpick claude       # Opus 5 · 1M · effort high
agentpick codex        # GPT-5.6 Sol · reasoning high
agentpick grok         # grok-4.5 · effort high
agentpick copilot      # gpt-5.6-terra · subscription
agentpick agy          # gemini-3.6-flash-high · effort high
```

## Why

Each agent CLI has different model flags and effort knobs. The “good” default changes as labs ship new models. `agentpick` keeps those choices in one YAML file so you can bump a model in a PR instead of rewriting shell aliases.

When [Headroom](https://github.com/headroomlabs-ai/headroom) is on your `PATH`, eligible providers run through `headroom wrap …` (context compression). If Headroom is missing, `agentpick` falls back to the native CLI with the same model/effort flags.

When [tokensave](https://github.com/aovestdipaperino/tokensave) is on your `PATH`, `agentpick` runs `tokensave sync` on **every** indexed project (`tokensave list -a`) before launch so the code graph is ready. Missing tokensave or a sync failure is non-fatal — the agent still starts. Use `--no-tokensave` to skip.

## Install

Requires Go 1.22+.

```bash
git clone https://github.com/marko-durasic/agentpick.git
cd agentpick
./install.sh
```

Installs `~/.local/bin/agentpick` plus short aliases: `hclaude`, `hopus`, `hcodex`, `hgrok`, `hcopilot`, `hagy`, `hagents` (→ `list`). Existing non-symlink scripts are backed up as `*.pre-agentpick`.

## Usage

```bash
agentpick --help
agentpick list
agentpick --dry-run claude          # print resolved argv (+ planned tokensave syncs)
agentpick --no-headroom codex       # force native CLI
agentpick --no-tokensave grok       # skip graph sync preflight
agentpick claude --resume <id>      # passthrough args
hclaude                             # same as agentpick claude
```

Global flags go **before** the provider name: `--dry-run`, `--no-headroom`, `--no-tokensave`.

## Bumping defaults

Edit [`defaults.yaml`](defaults.yaml) (symlink to [`internal/defaults/defaults.yaml`](internal/defaults/defaults.yaml)), rebuild:

```bash
./install.sh
# or: go build -o ~/.local/bin/agentpick ./cmd/agentpick
```

Each provider entry has a one-line `why` — keep that honest when you change models.

## Providers (v1)

| Provider | Default | Headroom |
|----------|---------|----------|
| `claude` | Opus 5 · `--1m` · effort high | `wrap claude` |
| `codex` | GPT-5.6 Sol · reasoning high | `wrap codex` |
| `grok` | grok-4.5 · effort high | `wrap grok` |
| `copilot` | gpt-5.6-luna · `--subscription` | `wrap copilot` |
| `agy` | gemini-3.6-flash-high · effort high | native only (Google harness) |

## License

MIT
