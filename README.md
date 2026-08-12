# ossfind

A small Go CLI that searches GitHub for recently opened, **unassigned**
issues matching a set of languages and a rough experience level
(beginner / intermediate / advanced).

No external dependencies — stdlib only (`net/http`, `encoding/json`,
`flag`, `text/tabwriter`).

## Build

```bash
go build -o ossfind .
```

Same command on Linux, macOS, and Windows — no build tags, no cgo, no
platform-specific files. On Windows this produces `ossfind.exe`
automatically; run it as `.\ossfind.exe` (or just `ossfind` from most
shells).

## Platform notes

Pure Go stdlib, no `os/exec`/`syscall`/cgo, and local state correctly
resolves to the OS-appropriate config directory via `os.UserConfigDir()`:

| OS      | State file location                          |
|---------|-----------------------------------------------|
| Linux   | `~/.config/ossfind/state.json` (or `$XDG_CONFIG_HOME`) |
| macOS   | `~/Library/Application Support/ossfind/state.json` |
| Windows | `%AppData%\ossfind\state.json`                |

The one thing that differs across platforms is **how you set an
environment variable** like `GITHUB_TOKEN` — the `export ...` shown
throughout this README is bash/zsh syntax:

```bash
# bash / zsh (Linux, macOS, Git Bash on Windows)
export GITHUB_TOKEN=ghp_xxxxxxxx

# PowerShell (Windows)
$env:GITHUB_TOKEN = "ghp_xxxxxxxx"

# cmd.exe (Windows)
set GITHUB_TOKEN=ghp_xxxxxxxx
```

Everything else — flags, output, state file format — is identical
across platforms.

## Usage

```bash
# Beginner-friendly Go + Python issues opened in the last 7 days
./ossfind -languages Go,Python -level beginner

# Custom label set, wider time window
./ossfind -languages Go -labels "good first issue,help wanted" -since 14

# Advanced: no label constraint, just recent + unassigned + language match
./ossfind -languages Go -level advanced -since 3

# JSON output, piped into jq
./ossfind -languages Python -level intermediate -json | jq '.[].html_url'

# Rank by rough difficulty (easiest first) instead of recency
./ossfind -languages Go -level beginner -sort difficulty

# Don't re-show issues you've already seen in the last N days (default: 7)
./ossfind -languages Go -level beginner -cooldown 3

# Check your local streak/history
./ossfind stats

# Auto-detect your languages + level from your own GitHub history instead
# of guessing at flags (uses top languages from your non-fork repos +
# merged-PR-to-other-repos count as an experience proxy).
# See "Platform notes" above for how to set GITHUB_TOKEN on your shell.
export GITHUB_TOKEN=ghp_xxxxxxxx
./ossfind -auto

# Auto-detect for someone else's public profile explicitly
./ossfind -auto -github-user torvalds

# Wipe local history
./ossfind reset-state -yes

# Raise your rate limit with a token (recommended, especially with -score)
export GITHUB_TOKEN=ghp_xxxxxxxx
./ossfind -languages Go -level beginner -v
```

## Flags

| Flag                | Default    | Description                                                        |
|----------------------|------------|----------------------------------------------------------------------|
| `-languages`         | `Go`       | Comma-separated GitHub language names                                |
| `-level`             | `beginner` | `beginner`, `intermediate`, or `advanced` — sets default labels      |
| `-labels`            | (from level) | Comma-separated labels; overrides `-level`'s defaults               |
| `-since`             | `7`        | Only issues created in the last N days (`0` = no limit)             |
| `-limit`             | `20`       | Max issues printed after merging + deduping                          |
| `-per-query`         | `20`       | Max results fetched per individual search query                     |
| `-sort`              | `updated`  | `updated`, `created`, or `difficulty`                                |
| `-score`             | `false`    | Attach a difficulty estimate (comments + repo stars) to each issue   |
| `-score-pool`        | `40`       | With `-sort difficulty`, how many recent candidates to score before ranking |
| `-token`             | `$GITHUB_TOKEN` | GitHub token — raises rate limit from 10→30 search req/min, 60→5000 repo lookups/hour |
| `-include-assigned`  | `false`    | Include issues that already have an assignee                        |
| `-json`              | `false`    | Print raw JSON instead of a table                                    |
| `-v`                 | `false`    | Log each query + rate-limit status to stderr                        |
| `-cooldown`          | `7`        | Days to hide an issue after it's been shown once (`0` = never hide repeats) |
| `-show-repeats`      | `false`    | Ignore the cooldown for this run only                                |
| `-no-state`          | `false`    | Don't read or write local history at all (fully stateless run)      |
| `-state-path`        | (default location) | Override the local state file path                          |
| `-auto`              | `false`    | Auto-detect `-languages`/`-level` from a GitHub user's own repo languages + PRs merged into *other* repos |
| `-github-user`       | (token's own user) | Username to auto-detect from; defaults to the token's own account |
| `-health`            | `false`    | Filter out issues from stale or unresponsive repos (~2 extra API calls per unique repo) |
| `-max-stale-days`    | `180`      | With `-health`, filter out repos with no push in this many days     |
| `-min-merge-rate`    | `0.2`      | With `-health`, filter out repos whose recent PR merge rate is below this (0.0-1.0) |
| `-pr-sample`         | `10`       | With `-health`, how many recently-closed PRs to sample for merge rate |
| `-health-pool`       | `30`       | With `-health`, how many recent candidates to health-check (bounds API calls) |

## Auto-detected skill profile (`-auto`)

Instead of guessing `-languages`/`-level` by hand, `-auto` builds a rough
profile from a real GitHub account:

- **Languages**: tallies the primary language of that user's public,
  *non-fork* repos (up to 300, via pagination) and takes the top 3 —
  reflects what they've actually authored, not just cloned.
- **Level**: counts merged PRs *authored by* that user *into repos they
  don't own* (`is:pr is:merged author:USER -user:USER`) — PRs merged into
  their own repos don't count, since solo-repo PR habits aren't the same
  skill as landing a change in someone else's codebase. Maps to a level:
  `<6` → beginner, `6–29` → intermediate, `30+` → advanced.

By default `-auto` resolves the username from your `GITHUB_TOKEN` (via
`GET /user`) — pass `-github-user` to target a specific account instead
(useful for checking someone else's public profile, or if you're running
without a token). Explicit `-languages`/`-level`/`-labels` flags always
win over auto-detection — `-auto` only fills in what you didn't specify.

This is the thing neither CodeTriage nor goodfirstissue.dev do: they
match on static labels only, not on what you've actually shipped.

## Repo-health filter (`-health`)

A `good first issue` tag doesn't mean the repo is still alive. `-health`
checks each unique repo among your candidates (deduped -- one check per
repo, not per issue) against two signals:

- **Staleness**: days since the repo's last push (`pushed_at`). Over
  `-max-stale-days` (default 180) → filtered out.
- **Responsiveness**: merge rate across its most recently closed PRs
  (`-pr-sample`, default 10). Below `-min-merge-rate` (default 20%) →
  filtered out. Repos with fewer than 3 closed PRs ever are *not*
  penalized on this signal — too thin a sample to trust.

```bash
./ossfind -languages Go -level beginner -health -v
```

`-v` shows exactly why each repo got filtered (e.g. `no push in 240 days
(limit 180)` or `merge rate 10% (1/10 recent PRs) below 20% threshold`).
Costs roughly 2 extra API calls per unique repo, bounded by `-health-pool`
(default 30) so a large candidate set doesn't balloon into hundreds of
requests. Combine with `-score` freely — both reuse the same underlying
repo metadata call where possible.

## Difficulty scoring

`-score` fetches each unique repo's star count (deduped — one API call per
distinct repo, not per issue) and combines it with the issue's comment
count into a 0-4 score, labeled Easy / Medium / Hard:

| Signal              | 0 pts     | 1 pt         | 2 pts     |
|----------------------|-----------|--------------|-----------|
| Comments on issue    | ≤ 2       | 3–6          | 7+        |
| Repo stars            | < 200     | 200–5,000    | 5,000+    |

Score 0–1 → **Easy**, 2–3 → **Medium**, 4 → **Hard**. This is a cheap
heuristic, not a real complexity measure — a quiet issue in a small repo
is usually easier to land a first PR on than a heavily-discussed issue in
a massive, high-traffic one, but there will be exceptions.

`-sort difficulty` ranks by this score (easiest first). Since GitHub's
API can't sort by it directly, `ossfind` pulls the `-score-pool` most
recently updated candidates, scores them, ranks them, then applies
`-limit`. Repo lookups use GitHub's *core* rate limit (60/hour unauthenticated,
5000/hour with a token) — a separate, stingier bucket than search, so
set `GITHUB_TOKEN` before using `-score` on anything but small result sets.

## Local state (memory across runs)

`ossfind` keeps a small JSON file at `~/.config/ossfind/state.json`
recording every issue URL it's shown you and every day you've run it.
This does two things:

- **No repeat spam.** Issues shown within the last `-cooldown` days
  (default 7) are filtered out of future runs automatically, so
  running `ossfind` daily surfaces new candidates instead of the same
  handful on a loop. Override per-run with `-show-repeats`, or disable
  the whole mechanism with `-cooldown 0` or `-no-state`.
- **`ossfind stats`** — shows your run count, current daily streak,
  how many unique issues/repos you've been shown, and your
  most-surfaced repos.

```
$ ossfind stats
Runs:            12 (first: 2026-07-27, last: 2026-08-07)
Current streak:  4 day(s)
Issues surfaced: 38 unique (41 total impressions) across 22 repo(s)

Most-surfaced repos:
  gofiber/fiber                           3
  go-gitea/gitea                          2
  ...
```

`ossfind reset-state -yes` wipes it. State is pure local JSON — no
account, no server, nothing leaves your machine.

## Subcommands

- `ossfind [flags]` / `ossfind find [flags]` — find matching issues (default)
- `ossfind stats` — show local run history/streak
- `ossfind reset-state -yes` — clear local history

## How it works

GitHub's search API can't `OR` across `language:` qualifiers, so
`ossfind` runs one query per `(language, label)` pair, merges the
results, dedupes by issue URL, and sorts by `updated_at` descending.
It respects `X-RateLimit-Remaining`/`X-RateLimit-Reset` and sleeps +
retries once if you get rate-limited mid-run.

## Level → label mapping (defaults, override with `-labels`)

- `beginner` → `good first issue`
- `intermediate` → `help wanted`
- `advanced` → *(no label filter — relies on language + recency only)*

## Ideas for next steps

(From a critical comparison against CodeTriage — ranked by leverage;
✅ = done.)

1. ✅ Auto-detect `-languages`/`-level` from your own GitHub PR history (`-auto`)
2. ✅ Repo-health filter: skip repos with stale last-commit dates or a low PR merge rate (`-health`)
3. Doc-gap detection for Go (`go doc`-style missing-comment scan) and Python (`ast`-based missing-docstring scan)
4. `-type triage` mode surfacing issues that need repro/confirmation, not just code fixes
5. ✅ Local habit loop — state, cooldown, streaks, `stats` subcommand
6. "Why this matched" transparency in output
7. Multi-forge support (Codeberg/Forgejo)

Also on the list: cache repo star lookups to disk between runs to cut
down on `-score` API usage; `--watch` mode that polls on an interval.
