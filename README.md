# agentpick

Launch coding agents with **bang-for-buck** defaults — one CLI, versioned model registry, Headroom-aware when available.

```bash
agentpick              # interactive picker (shows remaining quota when known)
agentpick list         # show optimal defaults + quota
agentpick cursor       # Auto · quota-smart default
agentpick claude       # Opus 5 · 1M · effort high
agentpick codex        # GPT-5.6 Sol · reasoning high
agentpick grok         # grok-4.5 · effort high
agentpick copilot      # gpt-5.6-luna · subscription
agentpick agy          # gemini-3.6-flash-high · effort high
```

## Why

Each agent CLI has different model flags and effort knobs. The “good” default changes as labs ship new models. `agentpick` keeps those choices in one YAML file so you can bump a model in a PR instead of rewriting shell aliases.

When [Headroom](https://github.com/headroomlabs-ai/headroom) is on your `PATH`, eligible providers run through `headroom wrap …` (context compression). If Headroom is missing, `agentpick` falls back to the native CLI with the same model/effort flags.

When [tokensave](https://github.com/aovestdipaperino/tokensave) is on your `PATH`, `agentpick` runs `tokensave sync` on **every** indexed project (`tokensave list -a`) before launch so the code graph is ready. Missing tokensave or a sync failure is non-fatal — the agent still starts. Use `--no-tokensave` to skip.

The interactive picker and `list` show **remaining quota** best-effort (Cursor period % and Claude week % today; others show `n/a`). Probes run in parallel with an ~8s timeout (Claude `/usage` is slow) and cache for ~2 minutes under `~/.cache/agentpick/`. If Claude `/usage` fails, a local session reading from `~/.config/Claude/plan-usage-history.json` is used as fallback.

`headroom wrap` hardcodes `--port` default **8787** (and ignores `HEADROOM_PORT` for that flag). Port **8787** is also Cursor’s `mcp login` OAuth callback, so agentpick always passes an explicit `--port` (long form — `wrap claude` has no `-p`, and Claude itself uses `-p` for prompts):

1. `DUREEF_HEADROOM_PORT` if set
2. else `HEADROOM_PORT` if set
3. else **8788** (shared Headroom)

If the resolved port is **8787**, agentpick remaps to **8788** unless `AGENTPICK_ALLOW_HEADROOM_8787=1`. When the shared proxy’s `/readyz` is healthy, wrap also gets `--no-proxy` so it reuses that instance instead of starting another — **except** `copilot --subscription`, which needs a dedicated Headroom instance with session auth seeds (shared Anthropic proxy alone returns HTTP 401).

## Install

Requires Go 1.22+.

```bash
git clone https://github.com/marko-durasic/agentpick.git
cd agentpick
./install.sh
```

Installs `~/.local/bin/agentpick` plus short aliases: `hcursor`, `hclaude`, `hopus`, `hcodex`, `hgrok`, `hcopilot`, `hagy`, `hagents` (→ `list`). Existing non-symlink scripts are backed up as `*.pre-agentpick`.

## Usage

```bash
agentpick --help
agentpick list
agentpick --dry-run claude          # print resolved argv (+ planned tokensave syncs)
agentpick --no-headroom codex       # force native CLI
agentpick --no-tokensave grok       # skip graph sync preflight
agentpick claude --resume <id>      # passthrough args
hclaude                             # same as agentpick claude
hcursor                             # same as agentpick cursor
```

Global flags go **before** the provider name: `--dry-run`, `--no-headroom`, `--no-tokensave`.

## Bumping defaults

Edit [`defaults.yaml`](defaults.yaml) (symlink to [`internal/defaults/defaults.yaml`](internal/defaults/defaults.yaml)), rebuild:

```bash
./install.sh
# or: go build -o ~/.local/bin/agentpick ./cmd/agentpick
```

Each provider entry has a one-line `why` — keep that honest when you change models.

## Providers (v3)

| Provider | Default | Headroom | Quota probe |
|----------|---------|----------|-------------|
| `cursor` | Auto · `--model auto` | native only | Cursor period % left |
| `claude` | Opus 5 · `--1m` · effort high | `wrap claude` | Claude week % left |
| `codex` | GPT-5.6 Sol · reasoning high | `wrap codex` | n/a |
| `grok` | grok-4.5 · effort high | native only (xAI; Headroom Anthropic proxy breaks catalog) | n/a |
| `copilot` | gpt-5.6-luna · `--subscription` | `wrap copilot` | n/a |
| `agy` | gemini-3.6-flash-high · effort high | native only (Google harness) | n/a |

## License

MIT
