# Nixie Site (Go Version)

This is a modern reimplementation of the Nixie Site using:
- **Go**: High-performance backend.
- **HTMX**: For dynamic, AJAX-like interactions without complex JavaScript.
- **Templ**: Type-safe HTML templates for Go.
- **Magefile**: Build automation.

## Project Structure

- `cmd/server/`: Main application entry point.
- `internal/handlers/`: HTTP request handlers.
- `internal/templates/`: Templ components (`.templ` files).
- `static/`: Static assets (CSS, JS, Images).
- `Magefile.go`: Build scripts.

## Getting Started

### Prerequisites

1.  **Go** (1.25+)
2.  **templ** CLI: Install via `go install github.com/a-h/templ/cmd/templ@latest`
3.  **mage** CLI (Optional, can use `go run mage.go`): Install via `go install github.com/magefile/mage@latest`

### Development Workflow

The project uses `mage` for build automation. You can run `go run mage.go` if you don't have `mage` installed.

#### 1. Setup Environment
Copy the template environment file:
```bash
cp env.template .env
```

#### 2. Start Database
```bash
mage dbup
```

#### 3. Database Migrations
To apply migrations:
```bash
mage migrate
```

To create a new migration:
```bash
mage migratecreate <name>
```

#### 4. Run Application
To generate code, build, and run the server:
```bash
mage run
```

Or use the **all-in-one** development command:
```bash
mage dev
```

The server will be available at `http://localhost:8080`.

### Docker

The project includes two Docker configurations:
- `docker-compose.yaml`: Configured for production-like environments, using external networks and links.
- `docker-compose.dev.yaml`: Used by `mage` targets for local development to provide a self-contained database.

To start the application and database together in containers for production:
```bash
docker-compose up
```

To connect to an external database, override the `DATABASE_URL` environment variable:
```bash
DATABASE_URL=postgres://user:pass@external_host:5432/dbname docker-compose up
```

### HTMX Usage

HTMX is included via CDN in the `Layout` component. Use `hx-` attributes in your templ components to trigger AJAX requests.

Example:
```html
<button hx-get="/hello" hx-target="#message">Click Me</button>
```
