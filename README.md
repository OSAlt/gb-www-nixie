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

### Development

To generate templates and build the server:

```bash
mage build
```

To run the server:

```bash
mage run
```

The server will be available at `http://localhost:8080`.

### HTMX Usage

HTMX is included via CDN in the `Layout` component. Use `hx-` attributes in your templ components to trigger AJAX requests.

Example:
```html
<button hx-get="/hello" hx-target="#message">Click Me</button>
```
