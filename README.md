# Nixie Site (Go Version)

[![Test](https://github.com/OSAlt/gb-www-nixie/actions/workflows/test.yml/badge.svg)](https://github.com/OSAlt/gb-www-nixie/actions/workflows/test.yml)[![Build and Publish Docker Image](https://github.com/OSAlt/gb-www-nixie/actions/workflows/publish.yml/badge.svg)](https://github.com/OSAlt/gb-www-nixie/actions/workflows/publish.yml)

This is a modern reimplementation of the Nixie Site using:
- **Go**: High-performance backend.
- **HTMX**: For dynamic, AJAX-like interactions without complex JavaScript.
- **Templ**: Type-safe HTML templates for Go.
- **Postgres + River**: shared feed caches and background jobs.
- **Magefile**: Build automation.

## Project Structure

- `cmd/server/`: Main application entry point.
- `internal/adapters/http/`: HTTP request handlers.
- `internal/adapters/{db,blog,contact,media,social,youtube}/`: external
  integrations and storage.
- `internal/jobs/`: River workers and schedules.
- `internal/templates/`: Templ components (`.templ` files).
- `static/`: Static assets (CSS, JS, Images).
- `db/migrations/`: goose migrations (plus River's own migrations, applied by the
  app).
- `Magefile.go`: Build scripts.

## Getting Started

### Prerequisites

1. **Go** 1.27+
2. **Docker** (for the local Postgres and the River UI)
3. **mage**: `go install github.com/magefile/mage@latest`

`templ`, `sqlc`, `goose` and `mockery` do not need to be installed; the Magefile
runs the pinned versions from `tools/go.mod`.

### Development Workflow

#### 1. Setup Environment

```bash
cp env.template .env
```

#### 2. Start the development services

```bash
mage dbup
```

This starts Postgres (`localhost:5432`) and the River UI at
<http://localhost:8081>. The UI shows the jobs the app works; the `river` schema
must exist first (step 3).

#### 3. Apply migrations

```bash
mage migrate
```

This runs goose, then River's migrations, then refreshes `db/schema.sql`. To
create a new migration:

```bash
mage migratecreate <name>
```

#### 4. Run the application

```bash
mage run
```

Or use the all-in-one command:

```bash
mage dev
```

The server is available at `http://localhost:8080`; the River UI at
`http://localhost:8081`.

### Tests

```bash
go test ./...        # includes testcontainers DB tests
go test -short ./... # skips Postgres
```

### Health

- `/api/v1/social/health` — follower-count freshness.
- `/api/v1/content/health` — media, blog and contact cache freshness.

### Docker

The project includes two Docker configurations:
- `docker-compose.yaml`: production-like, using external networks and links.
- `docker-compose.dev.yaml`: local Postgres and River UI, used by `mage`.

To start the application and database together in containers for production:

```bash
docker-compose up
```

To connect to an external database, override `DATABASE_URL`:

```bash
DATABASE_URL=postgres://user:pass@external_host:5432/dbname docker-compose up
```

### HTMX Usage

HTMX is included via CDN in the `Layout` component. Use `hx-` attributes in your
templ components to trigger AJAX requests.

Example:

```html
<button hx-get="/hello" hx-target="#message">Click Me</button>
```
