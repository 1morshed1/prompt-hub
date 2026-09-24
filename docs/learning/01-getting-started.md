# 01 — Getting started (run it locally)

By the end of this doc you'll have PromptHub running, seeded with a demo prompt, and you'll
have driven an A/B experiment to a statistically significant winner — all from the command line.

## Prerequisites

- **Go 1.22+** (`go version`)
- **PostgreSQL 14+** — either a server you already run, or use the helper script below which
  spins up a throwaway cluster in a temp directory (no root, no Docker needed).
- **Python 3.9+** (only if you want to try the SDK)

## 1. Start a database

If you have your own Postgres, just create a database and note the connection URL. Otherwise
use the bundled helper, which runs an isolated cluster on port 5433:

```bash
./scripts/dev_db.sh start      # initdb (first time) + start, creates the 'prompthub' db
./scripts/dev_db.sh status     # is it up?
./scripts/dev_db.sh stop       # shut it down
./scripts/dev_db.sh psql       # open a psql shell into it
```

> The script must run as a non-root user (Postgres refuses to run as root). It stores its data
> under `$PROMPTHUB_PGDATA` (default `./.pgdata`), so it never touches a system Postgres.

The connection URL for the helper is:

```
postgres://postgres@localhost:5433/prompthub?sslmode=disable
```

## 2. Run the server

```bash
cd server
export DATABASE_URL="postgres://postgres@localhost:5433/prompthub?sslmode=disable"
go run ./cmd/server
```

On boot the server **applies any pending SQL migrations** from `server/migrations/`, then
listens on `:8080`. You'll see log lines for each migration and then `listening on :8080`.

Configuration is via environment variables (see `server/internal/config`):

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | *(required)* | Postgres connection string |
| `PORT` | `8080` | HTTP listen port |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |

## 3. Seed demo data

```bash
./scripts/seed.sh      # creates an 'order-assistant' prompt (v1..v3), tags, a shared
                       # include, and an A/B experiment driven to a significant winner
```

This uses the Python SDK under the hood (`scripts/seed.py`), so it doubles as a
worked example of the SDK's management surface.

Then open **http://localhost:8080/** for the dashboard, or hit the API directly:

```bash
# List prompts
curl -s localhost:8080/api/v1/prompts | jq

# Render the production version with variables
curl -s -X POST localhost:8080/api/v1/prompts/order-assistant/render \
  -H 'content-type: application/json' \
  -d '{"tag":"production","variables":{"role":"support agent","topic":"refunds","query":"Where is my order?","language":"en"}}' | jq
```

## 4. Drive an A/B test from the command line

```bash
# See the experiment the seed script created
curl -s localhost:8080/api/v1/experiments | jq

# The seed already drove ~300 requests. To add more, simulate traffic for a prompt
# that has a running experiment (each call assigns an arm and logs a result):
./scripts/simulate_traffic.sh order-assistant 500

# Ask for the verdict (also triggers auto-promotion if the gates pass)
EXP=$(curl -s "localhost:8080/api/v1/experiments?prompt=order-assistant" | jq -r '.experiments[0].id')
curl -s "localhost:8080/api/v1/experiments/$EXP/report" | jq
```

The report tells you sample sizes, average rating/latency/cost per arm, the p-values, whether
the result is significant, the winner, and a plain-English recommendation. If the variant wins
significantly and the experiment has auto-promote enabled, the `production` tag has already
moved — verify with:

```bash
curl -s localhost:8080/api/v1/prompts/order-assistant/tags | jq
```

## Where to go next

- Curious how a request becomes a rendered prompt? → `02-architecture.md`
- Want to understand the schema? → `03-database-design.md`
- Want to see the template engine internals? → `04-template-engine.md`
- Want the statistics explained? → `06-ab-testing-and-stats.md`
