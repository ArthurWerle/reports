# reports

An automated monthly financial report service for the homelab finance stack. On
a configured day/hour each month it generates a report about the **previous
month** and emails it (HTML with embedded PNG charts) to the configured
recipients. The report has:

1. **Header** — an LLM-written headline plus income/expense totals with
   month-over-month deltas.
2. **Charts** — expenses by category, by subcategory, by location, and a
   6-month income-vs-expense history.
3. **Footer** — LLM-written highlights, concerns, and a closing suggestion.

Built in Go, mirroring the `transactions` service conventions: Gin + GORM +
`log/slog` JSON logs + embedded SQL migrations + Postgres.

## How it works

```
┌──────────┐  1. internal scheduler (60s ticker) fires on day-of-month/hour
│ reports  │──── creates report_executions row (status=pending)
│ service  │──── 2. enqueues a job in events: POST {EVENTS_BASE_URL}/api/events
└──────────┘        payload {"execution_id":N}, callback_url {REPORTS_CALLBACK_BASE_URL}/api/v1/jobs/generate
      ▲
      │ 3. events delivers: GET /api/v1/jobs/generate?job_type=...&payload=...
      │    reports marks execution running, returns 200 immediately, generates in a goroutine
      │
      ├── 4. GET transactions /api/v2/transactions/reports/*  (aggregated numbers)
      ├── 5. POST ai-internal /report-insights                (structured LLM insights)
      ├── 6. render 4 PNG charts (go-chart) + HTML template
      ├── 7. send email via SMTP (charts embedded via cid: inline attachments)
      └── 8. persist html/charts/insights + status/duration/error on report_executions
```

The callback returns `200` **before** generation finishes: the events processor
delivers with a ~10s HTTP timeout and would otherwise time out and re-deliver
mid-generation (the LLM call alone can exceed 10s). Consequently the events
retry mechanism only covers *delivery*; generation failures are recorded in
`report_executions.error_message` and retried manually from the UI's **Run now**
button.

## Configuration (environment variables)

All have defaults (via `getEnv`); override in `stack.env`.

| Variable | Default | Purpose |
| --- | --- | --- |
| `SERVER_PORT` | `8080` | HTTP port |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE` | `localhost` / `5432` / `reports` / `reports_dev_password` / `reports_db` / `disable` | Postgres |
| `LOG_LEVEL` | `info` | slog level |
| `REPORTING_TIMEZONE` | `America/Sao_Paulo` | calendar for scheduling + period math (must match transactions) |
| `TRANSACTIONS_BASE_URL` | `http://localhost:1235/api/v2` | transactions service |
| `EVENTS_BASE_URL` | `http://localhost:3000` | events service |
| `AI_INTERNAL_BASE_URL` | `http://localhost:3005` | ai-internal service |
| `REPORTS_CALLBACK_BASE_URL` | `http://localhost:8080` | address the **events container** calls back on (e.g. `http://reports:8080` in prod) |
| `SCHEDULER_ENABLED` | `true` | set `false` to run API/UI without the ticker (useful in staging) |
| `REPORT_LANGUAGE` | `en` | passed to `/report-insights` |
| `REPORT_CURRENCY` | `BRL` | display formatting in the HTML (e.g. `R$ 1.234,56`) |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` / `SMTP_FROM` | — | email delivery. **Email is required for a generation to succeed** — `SMTP_HOST`, `SMTP_PORT`, and `SMTP_FROM` must all be set, otherwise every generation fails with `email not sent: SMTP not configured` (the report HTML is still stored and viewable in the UI). `SMTP_USERNAME` / `SMTP_PASSWORD` are optional (leave empty for an auth-less local relay). |

## HTTP API

| Method & path | Behavior |
| --- | --- |
| `GET /` | config dashboard (single embedded HTML page) |
| `GET /health` | health check |
| `GET /api/v1/reports` | list report configs |
| `POST /api/v1/reports` | create (`name`, `day_of_month`, `hour`, `recipients`, `enabled`) |
| `PUT /api/v1/reports/:id` | update those fields |
| `DELETE /api/v1/reports/:id` | delete (cascades executions) |
| `POST /api/v1/reports/:id/run` | manual run; body `{"month":6,"year":2026}` optional (defaults to previous month) |
| `GET /api/v1/executions?report_id=&limit=` | newest-first execution list (no html blob) |
| `GET /api/v1/executions/:id/html` | stored web-mode HTML |
| `GET /api/v1/executions/:id/charts/:name` | a chart PNG |
| `GET /api/v1/jobs/generate` | events delivery callback |

## Local development

```bash
# 1. Start a local Postgres (the compose file provides one)
docker compose --env-file stack.env up -d postgres

# 2. Run the service (migrations apply automatically at startup)
go run ./cmd/server

# 3. Open the dashboard
open http://localhost:8080/
```

`go build ./... && go test ./...` builds and tests the service.

### Local end-to-end with Mailpit

To eyeball the rendered email locally, run [Mailpit](https://github.com/axllent/mailpit)
as the SMTP target (SMTP on `1025`, web UI on `8025`):

```bash
docker run -d --name mailpit -p 1025:1025 -p 8025:8025 axllent/mailpit
```

Set `SMTP_HOST=localhost`, `SMTP_PORT=1025`, `SMTP_FROM=reports@local` (leave
`SMTP_USERNAME`/`SMTP_PASSWORD` empty — no auth, no TLS), set recipients on the
seeded report, click **Run now**, and open http://localhost:8025 to view the
message. Do NOT add Mailpit to the prod compose files.

## Deployment (Docker + Portainer)

The `Dockerfile` builds a static binary; templates and migrations are embedded
via `go:embed`, so the image is self-contained. `docker-compose.yml` (prod) and
`docker-compose.staging.yml` (staging) run Postgres + the reports service, driven
entirely by a gitignored `stack.env`. Both join the shared external network
`financer-transactions_transactions-network` (created by the transactions stack)
so reports can reach transactions / ai-internal / events by container name and
the events callback can reach reports.

| Stack | Compose file | Container | Host port |
| --- | --- | --- | --- |
| prod | `docker-compose.yml` | `reports` | `3010` |
| staging | `docker-compose.staging.yml` | `reports-staging` | `3011` |

Example **prod** `stack.env`:

```dotenv
# postgres
POSTGRES_USER=reports
POSTGRES_PASSWORD=change-me
POSTGRES_DB=reports_db
POSTGRES_CONTAINER_NAME=reports-postgres
POSTGRES_PORT=5432

# reports service (compose interpolation)
SERVICE_CONTAINER_NAME=reports
SERVICE_PORT=3010

# reports app config
SERVER_PORT=8080
DB_HOST=reports-postgres
DB_PORT=5432
DB_USER=reports
DB_PASSWORD=change-me
DB_NAME=reports_db
DB_SSLMODE=disable
LOG_LEVEL=info
REPORTING_TIMEZONE=America/Sao_Paulo
TRANSACTIONS_BASE_URL=http://transaction-service-v2:1235/api/v2
EVENTS_BASE_URL=http://events:3000
AI_INTERNAL_BASE_URL=http://ai-internal:3005
REPORTS_CALLBACK_BASE_URL=http://reports:8080
SCHEDULER_ENABLED=true
REPORT_LANGUAGE=en
REPORT_CURRENCY=BRL
# SMTP (Gmail app password example)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=you@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM=you@gmail.com
```

The **staging** `stack.env` uses distinct names/ports and points at `-staging`
containers, e.g. `SERVICE_CONTAINER_NAME=reports-staging`, `SERVICE_PORT=3011`,
`POSTGRES_CONTAINER_NAME=reports-staging-postgres`, `POSTGRES_PORT=5434`,
`TRANSACTIONS_BASE_URL=http://transaction-service-v2-staging:1235/api/v2`,
`EVENTS_BASE_URL=http://events-staging:3000`,
`AI_INTERNAL_BASE_URL=http://ai-internal-staging:3005`,
`REPORTS_CALLBACK_BASE_URL=http://reports-staging:8080`, and
`SCHEDULER_ENABLED=false` (so only prod emails on the 1st).

In Portainer, create two stacks — one per compose file — each with its own
`stack.env`.

```bash
# local prod-style
make compose-up
# local staging-style
make staging-compose-up
```

## Dependency on the events client

This service imports `github.com/ArthurWerle/events/client` to enqueue jobs. The
`events` repo is public; `go.mod` pins a commit pseudo-version of the branch that
renamed the events module. **After** the events PR merges and `v0.1.0` is tagged,
bump `go.mod` to `github.com/ArthurWerle/events v0.1.0` and run `go mod tidy`.
