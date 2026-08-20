# agentpick

Launch coding agents with **bang-for-buck** defaults — one CLI, versioned model registry, Headroom-aware when available.

```bash
agentpick              # start command: quota picker → AWS CAO vibe session (localhost :9889)
agentpick --dry-run    # print cao-server + cao launch argv
agentpick --no-cao     # single-CLI orchestrator (old briefing path)
agentpick list         # show optimal defaults + quota
agentpick cursor       # Auto · quota-smart default
agentpick claude       # Opus 5 · 1M · effort high
agentpick codex        # GPT-5.6 Sol · reasoning high
agentpick grok         # account default (4.6) · effort high · native
agentpick copilot      # gpt-5.6-luna · subscription
agentpick agy          # gemini-3.6-flash-high · effort high
agentpick ollama       # qwen3.5:4b · local tiny/helper (not coding default)
```

## Why

Each agent CLI has different model flags and effort knobs. The “good” default changes as labs ship new models. `agentpick` keeps those choices in one YAML file so you can bump a model in a PR instead of rewriting shell aliases.

When [Headroom](https://github.com/headroomlabs-ai/headroom) is on your `PATH`, eligible providers run through `headroom wrap …` (context compression). If Headroom is missing, `agentpick` falls back to the native CLI with the same model/effort flags.

When [tokensave](https://github.com/aovestdipaperino/tokensave) is on your `PATH`, `agentpick` runs `tokensave sync` on **every** indexed project (`tokensave list -a`) before launch so the code graph is ready. Missing tokensave or a sync failure is non-fatal — the agent still starts. Use `--no-tokensave` to skip.

The interactive picker and `list` show **remaining quota** with explicit windows:

| Window | Meaning |
|--------|---------|
| `week` | Claude weekly plan **or** Codex primary window (~7 days) |
| `session` | Claude rolling session window (~5 hours) |
| `billing-period` | Cursor plan usage for the **current billing cycle** |
| `month` | Copilot monthly quota (when the CLI reports it) |
| `available…` | Probe succeeded but that CLI/API exposed no percentage |

**How we probe**

| Provider | Method |
|----------|--------|
| Cursor | Local token + Cursor usage API |
| Claude | `claude /usage` (+ local session history fallback) |
| Codex | ChatGPT `codex/usage` API from `~/.codex/auth.json` (fallback: tiny `codex exec` when API forbids) |
| Copilot | Tiny `copilot -p` scrape (`monthly quota` / `AI Credits N`) |
| Grok / agy | Grok: tiny print/single prompt (status vs limit). Agy: Models & Quota panel (`/usage`) — Gemini weekly % when JSON/cache is present; never `agy -p` (print mode does not show quota) |

Probes run in parallel (~20s timeout) and cache for ~2 minutes under `~/.cache/agentpick/`.

`headroom wrap` hardcodes `--port` default **8787** (and ignores `HEADROOM_PORT` for that flag). Port **8787** is also Cursor’s `mcp login` OAuth callback, so agentpick always passes an explicit `--port` (long form — `wrap claude` has no `-p`, and Claude itself uses `-p` for prompts):

1. `DUREEF_HEADROOM_PORT` if set
2. else `HEADROOM_PORT` if set
3. else **8788** (shared Headroom)

If the resolved port is **8787**, agentpick remaps to **8788** unless `AGENTPICK_ALLOW_HEADROOM_8787=1`. When the shared proxy’s `/readyz` is healthy, wrap also gets `--no-proxy` so it reuses that instance instead of starting another — **except** `copilot --subscription`, which needs a dedicated Headroom instance with session auth seeds (shared Anthropic proxy alone returns HTTP 401).


## Orchestration (vibe-code session)

**Start command is still `agentpick`.** That is the name you run. It is not `cao launch`.

```bash
agentpick
```

You get a list of installed CLIs. One is **recommended as supervisor** from remaining quota plus the role matrix (`cursor` first through the prepaid Auto window, then `claude`). Press Enter to take the recommendation, or type a number / name.

That:

1. Writes `~/.cache/agentpick/orchestrator-brief.md`
2. **Warm-starts the healthy fleet** from live quota + role/model rank and installs CAO profiles: `agentpick_supervisor`, `agentpick_dev`, `agentpick_review`, plus extras (`agentpick_agy`, …)
3. Starts **AWS CAO** `cao-server` on **127.0.0.1:9889** if needed (pin `cli-agent-orchestrator==2.4.1`, never `@main`, never `--yolo`, never ports **8787** / **8788**)
4. Opens a **Cursor CLI** supervisor and **pre-spawns the healthy fleet** in the same CAO session. Before every new slice, the supervisor re-runs `agentpick route` itself, combining role/model rank with fresh quota and free capacity. It may use multiple isolated instances of one provider when that remains the best choice; a provider is not a 1:1 slot. At most four safe independent specialist tasks run at once by default. Ready panes may stay idle, and Agentpick never invents work merely to keep every CLI busy.

Lower the safety ceiling with `AGENTPICK_MAX_ACTIVE_AGENTS=1..4`. Four is a hard cap; it is never a target.

CAO 2.4.1 still cannot spawn **Grok** or **Ollama** (no Spawn Agent provider — that is unchanged; the earlier “Grok fix” was dispatch-only). agentpick still puts Grok in the session routing table: the supervisor runs `agentpick dispatch --prefer grok`. Exhausted CLIs (for example Codex at 0% until reset) are skipped. Spawn Agent will not list Grok/Ollama; that is expected until a later CAO pin that adds those providers.

This is **full Cursor CLI**, including workspace slash commands from `.cursor/commands` (`/start`, `/wrap-up`, `/do-next`, …). Orchestration **adds** tmux workers; it does not strip CLI features. `agentpick` defaults `--working-directory` to a DuReef tree that has those commands (isolated clone when present), not the agentpick source repo. Override with `--dir` or `AGENTPICK_CAO_WORKDIR`.

UI: [http://127.0.0.1:9889](http://127.0.0.1:9889). Each `agentpick` starts a **new** CAO session (unique name) so a second terminal does not collide. Reuse one session with `AGENTPICK_CAO_SESSION=agentpick`. Stop leftover sessions: `cao shutdown --all`.

Scroll in a pane: **mouse wheel** or **PageUp / PageDown** (agentpick turns tmux `mouse` on; CAO 2.4.1 leaves it off, so wheel otherwise does nothing in Cursor/Claude TUIs). Raw process output: `Ctrl-b` then `[`, then wheel or arrows; `q` to leave copy-mode. In the web UI, click the terminal so it has focus (otherwise the page steals PageUp).

`dureef-sprint` (scheduled shifts) does **not** start CAO. `agentpick cursor` / `agentpick claude` still launch a single CLI.

`--no-cao` (or `AGENTPICK_NO_CAO=1`) falls back to one CLI plus a briefing.

Grok is not a CAO 2.4.1 Spawn Agent provider — it is still in the agentpick pool via `agentpick dispatch`. Same for Ollama (tiny/local only).

Copilot joins Grok and Ollama in supervisor dispatch routing. CAO's Copilot pane loses agentpick's subscription/model settings; `agentpick dispatch --prefer copilot` preserves its dedicated Headroom configuration while remaining parallel-capable.

### route + dispatch (manual / scripts)

Rank providers by **role**, **quota**, and **role_priority** — then optionally run headless:

```bash
agentpick route --role review
agentpick route --role review --exclude cursor --json
agentpick dispatch --role plan -p "Design the API for user settings"
agentpick dispatch --role review --exclude cursor --prompt-file review-brief.md --dry-run
```

Roles: `implement`, `review`, `plan`, `tiny`, `debug`, `orchestrator`  
Aliases: `independent_review` → `review`, `idea_proposal` → `plan`, `tiny_task` → `tiny`

Route history (JSONL): `~/.cache/agentpick/route-history.jsonl`  
Optional rankings feed: `AGENTPICK_FEED_RANKINGS=1` appends to `~/.cache/dureef/model-rankings.json.agentpick-route.jsonl`

DuReef sprint opt-in: `DUREEF_AGENTPICK_ROUTING=1` uses `agentpick route --json` inside `ResolveRole`.


## Install

Requires Go 1.22+.

```bash
git clone https://github.com/marko-durasic/agentpick.git
cd agentpick
./install.sh
```

Installs `~/.local/bin/agentpick` plus short aliases: `hcursor`, `hclaude`, `hopus`, `hcodex`, `hgrok`, `hcopilot`, `hagy`, `hollama`, `hagents` (→ `list`). Existing non-symlink scripts are backed up as `*.pre-agentpick`.

## Usage

```bash
agentpick                    # orchestrator picker + launch
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

## Providers (v4)

| Provider | Default | Headroom | Quota probe |
|----------|---------|----------|-------------|
| `cursor` | Auto · `--model auto` | native only | billing-period % left |
| `claude` | Opus 5 · `--1m` · effort high | `wrap claude` | week % left (+ session detail) |
| `codex` | GPT-5.6 Sol · reasoning high | `wrap codex` | week % via Codex usage API |
| `grok` | account default (4.6) · effort high | native only (xAI; Headroom Anthropic proxy breaks catalog) | CLI scrape (often status-only) |
| `copilot` | gpt-5.6-luna · `--subscription` | `wrap copilot` | month / AI credits via CLI scrape |
| `agy` | gemini-3.6-flash-high · effort high | native only (Google harness) | Gemini weekly % from Models & Quota (`/usage`); `agy -p` is not a quota probe |
| `ollama` | qwen3.5:4b · local tiny/helper | none (local) | no quota probe |

## Local / tiny (not optimal coding)

`ollama` launches a local model (default `qwen3.5:4b`) for tiny classify/format/helper
jobs when you do not have a powerful GPU/laptop, or want offline help. It is listed for
convenience; it is **not** the bang-for-buck coding default — use `cursor` / `claude` /
`codex` / … for real agent work. On weaker machines prefer smaller tags (`llama3.2:3b`
etc.) by passing args after `--` if supported, or change the defaults.yaml model.

## Future (not now)

The CAO dashboard at [http://127.0.0.1:9889](http://127.0.0.1:9889) already has **Home / Agents / Flows / Settings / Memory**. Those stay **localhost-only**. Next product step (when we choose it) is to *use* more of that surface from `agentpick` — scheduled Flows, cross-session Memory — without replacing the terminal start command.

- **Keep:** `agentpick` in the terminal is the vibe start.
- **Later:** richer localhost web companion (same CAO UI, still `127.0.0.1`, never `0.0.0.0`).
- **Not:** a hosted/cloud “AI employee”, LobeHub, or OpenClaw as the orchestra. OpenClaw remains optional phone *hands* later, not the supervisor.
- **Not:** `dureef-sprint` starting CAO. Sprint routing stays [#750](https://github.com/DuReef/workspace/issues/750) and is not closed by this dogfood.

## License

MIT
