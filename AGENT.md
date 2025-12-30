# Agent Instructions

This is an AoE4 Quote Database application built for exe.dev.

See README.md for full documentation.

## Tech Stack

- **Go 1.25+** - Backend server
- **SQLite** with **sqlc** - Database with type-safe generated queries
- **Go html/template** - Server-side HTML rendering
- **Vanilla CSS** - No frameworks, CSS custom properties for theming
- **Lucide Icons** - Icon library (loaded via CDN)
- **OpenTelemetry** - Observability via Honeycomb

## Key Conventions

### API Routes

All endpoints under `/api/*` are intended for programmatic access:
- Return JSON or plain text responses (not HTML)
- Designed for chat bots, stream overlays, and external integrations
- Should remain stable for external consumers

### Web Routes

All other routes serve HTML pages for browser-based interaction.

### Authentication

- Public routes: `/`, `/browse`, `/api/*`
- Authenticated routes: `/quotes`, `/civs`, `/suggestions` (require exe.dev login)

#### exe.dev Auth Headers

When a user is logged in via exe.dev, the proxy sets these headers:

| Header | Description |
|--------|-------------|
| `X-ExeDev-UserID` | Unique user ID |
| `X-ExeDev-Email` | User's email address |

**Important**: Always use `strings.TrimSpace()` when reading these headers.

Example:
```go
userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
if userID == "" {
    http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
    return
}
```

### Database

- SQLite with sqlc for type-safe queries
- Migrations in `db/migrations/` run automatically on startup
- After modifying `db/queries/*.sql`, regenerate with `cd db && go generate`

### API Documentation

- API docs are auto-generated using [swaggo/swag](https://github.com/swaggo/swag)
- OpenAPI spec served at `/api/openapi.json`
- Interactive Swagger UI at `/api/`
- After modifying API handlers, regenerate docs: `make swagger`
- The swagger.json is embedded in the binary via `//go:embed`

**Adding swagger annotations to new handlers:**
```go
// HandleExample godoc
// @Summary Brief description
// @Description Longer description
// @Tags tagname
// @Produce json
// @Param name query string false "Parameter description"
// @Success 200 {object} ResponseType
// @Router /path [get]
func (s *Server) HandleExample(w http.ResponseWriter, r *http.Request) {
```

### Testing

- Run `make test-unit` for unit tests, `make test-integration` for API tests
- **Always clean up test data**: When manually testing with curl, delete any quotes added:
  ```bash
  # After adding a test quote, delete it:
  sqlite3 db.sqlite3 "DELETE FROM quotes WHERE text = 'your test quote';"
  ```
- Integration tests should not leave persistent data in the database
- Use descriptive test quote text to make cleanup easier if needed
- Standard table-driven tests; no property-based testing currently

### Civilizations

- All 22 AoE4 civilizations are tracked with shortnames for API filtering
- Shortnames follow aoe4world.com conventions (e.g., `hre`, `delhi`, `zhuxi`)

### Quotes vs Matchup Tips

- Regular quotes: Have optional `civilization` field for civ-specific quotes
- Matchup tips: Have both `civilization` (your civ) and `opponent_civ` fields
- Both are stored in the same `quotes` table; matchup tips just have `opponent_civ` set
- Accessed via separate endpoints: `/api/quote` vs `/api/matchup`

---

## Frontend Architecture

### CSS Organization

**Two CSS files:**
- `theme.css` - Shared design system (variables, components, base styles)
- `style.css` - Legacy styles (being phased out)

**Always use `theme.css`** for new work. Each template includes it:
```html
<link rel="stylesheet" href="/static/theme.css?v=8">
```

Page-specific styles go in `<style>` blocks within templates, using CSS variables from theme.css.

### CSS Custom Properties (Variables)

All colors, spacing, and styling use CSS variables defined in `theme.css`. This enables:
- Dark/light theme switching
- Consistent design language
- Easy updates across all pages

**Key variable categories:**
```css
/* Backgrounds */
--bg-primary, --bg-secondary, --bg-card, --bg-card-hover

/* Text */
--text-primary, --text-secondary, --text-heading

/* Accent (purple) */
--accent, --accent-hover, --accent-soft

/* Status colors */
--success, --danger (and hover variants)

/* Borders & shadows */
--border, --border-subtle, --shadow

/* Sizing */
--radius (16px), --radius-sm (10px)
```

### Theme Switching

Dark mode is default. Light mode activated via `data-theme="light"` on `<html>`.

Each page includes a theme toggle button:
```html
<button class="theme-toggle" onclick="toggleTheme()">
    <span id="theme-icon"><i data-lucide="sun"></i></span>
</button>
```

Theme preference is stored in `localStorage`.

### Button Styles

**Ghost/outline style is the default.** Buttons have transparent backgrounds with colored borders.

| Class | Use Case | Appearance |
|-------|----------|------------|
| `button` / `.btn` | Default actions | Purple outline, transparent bg |
| `.btn-primary` | Main CTAs (Sign In, Submit) | Filled purple |
| `.btn-secondary` | Secondary actions | Gray outline |
| `.btn-danger` | Destructive actions (Delete) | Red outline |
| `.btn-success` / `.btn-approve` | Positive actions (Approve) | Green outline |
| `.btn-small` | Compact buttons (tables) | Smaller padding |

**Important:** Don't override button styles inline. Use theme.css classes.

### Cards

Use `.card` class for content containers:
```html
<div class="card">
    <h2>Title</h2>
    <p>Content</p>
</div>
```

### Forms

Form elements (`input`, `textarea`, `select`) are styled in theme.css with:
- Consistent padding and borders
- Focus states with accent color glow
- Dark/light mode support

### Icons

Using [Lucide Icons](https://lucide.dev/) via CDN:
```html
<i data-lucide="swords"></i>
<script src="https://unpkg.com/lucide@latest/dist/umd/lucide.min.js"></script>
<script>lucide.createIcons();</script>
```

Common icons used: `swords`, `bar-chart-3`, `inbox`, `coffee`, `sun`, `moon`, `check`, `x`

### Responsive Design

- Max-width containers (800-900px) centered with `margin: 0 auto`
- Flexbox for layouts
- Mobile breakpoints at 600px where needed

---

## Template Patterns

### Standard Page Structure

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Page Title - AoE4 Quote Database</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/static/theme.css?v=8">
    <style>
        /* Page-specific styles using CSS variables */
        body { max-width: 800px; margin: 0 auto; padding: 2rem; }
    </style>
</head>
<body>
    <div class="nav">
        <a href="/">← Home</a>
        <!-- other nav links -->
    </div>

    <h1><i data-lucide="icon-name"></i> Page Title</h1>
    
    <!-- content -->

    <footer class="site-footer">
        <a href="https://ko-fi.com/webframp" class="kofi-link" target="_blank" rel="noopener">
            <i data-lucide="coffee"></i> Support this project on Ko-fi
        </a>
    </footer>

    <button class="theme-toggle" onclick="toggleTheme()" title="Toggle theme">
        <span id="theme-icon"><i data-lucide="sun"></i></span>
    </button>
    <script>
        // Theme toggle script
    </script>
    <script src="https://unpkg.com/lucide@latest/dist/umd/lucide.min.js"></script>
    <script>lucide.createIcons();</script>
</body>
</html>
```

### Navigation

Authenticated pages include role-appropriate nav:
```html
<div class="nav">
    <a href="/">← Home</a>
    <a href="/quotes">Manage Quotes</a>
    <a href="/civs">Civilizations</a>
    <a href="/suggestions">Suggestions</a>
    {{if .IsAdmin}}<a href="/admin/owners">Channel Owners</a>{{end}}
    <a href="{{.LogoutURL}}">Logout</a>
</div>
```

### Messages (Success/Error)

```html
{{if .Success}}
    <div class="message success">{{.Success}}</div>
{{end}}
{{if .Error}}
    <div class="message error">{{.Error}}</div>
{{end}}
```

---

## Code Style

### Go

- Standard Go formatting (`gofmt`)
- Error handling: return errors, don't panic
- HTTP handlers follow `func(w http.ResponseWriter, r *http.Request)` pattern
- Template data structs defined near handlers that use them

### CSS

- Use CSS variables from theme.css, not hardcoded colors
- Page-specific styles in template `<style>` blocks
- BEM-ish naming for custom classes (e.g., `.quote-card`, `.quote-text`)
- Mobile-first isn't strictly followed; add responsive overrides as needed

### HTML Templates

- Go template syntax: `{{.Field}}`, `{{if}}`, `{{range}}`
- Keep logic minimal in templates
- Use semantic HTML elements
