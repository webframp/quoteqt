# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it responsibly:

1. **Do not** open a public GitHub issue for security vulnerabilities
2. Email the maintainer directly or use GitHub's private vulnerability reporting feature

## Security Considerations

### Runtime Dependencies

This application has minimal runtime dependencies:
- `modernc.org/sqlite` - Pure Go SQLite driver

### Build-time Dependencies

The `sqlc` code generator is used at build time only. Vulnerabilities in `sqlc`'s transitive dependencies (e.g., PostgreSQL parsing libraries) do not affect the running application, as the generated code does not import these packages.

### Authentication & Authorization

- Authentication is handled by exe.dev's proxy layer via `X-ExeDev-*` headers
- The application trusts these headers when running behind the exe.dev proxy
- **Important**: Do not expose this application directly to the internet without the exe.dev proxy, as the auth headers could be spoofed

### Data Validation

- All user inputs are validated for length limits
- Request body size is limited to 64KB
- SQL injection is prevented via parameterized queries (sqlc)
- XSS is mitigated via Go's html/template auto-escaping

### Rate Limiting

Public API endpoints (`/api/*`) are rate-limited to 30 requests per minute per IP with a burst of 10.

## Known Limitations

1. **Authorization scope**: Currently, any authenticated user can edit/delete any quote (not just their own). This is by design for a small trusted user base but may need refinement for larger deployments.

2. **No CSRF tokens**: Form submissions rely on exe.dev session authentication rather than CSRF tokens.
