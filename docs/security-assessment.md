# Security Assessment - OWASP Top 10 2025

**Date:** December 28, 2024  
**Application:** AoE4 Quote Database (quoteqt)  
**Assessed against:** [OWASP Top 10 2025](https://owasp.org/Top10/2025/)

---

## Summary

| Category | Risk Level | Status |
|----------|------------|--------|
| A01: Broken Access Control | Low | ✅ Acceptable |
| A02: Security Misconfiguration | Low-Medium | ⚠️ Needs attention |
| A03: Software Supply Chain Failures | Medium | ⚠️ Needs attention |
| A04: Cryptographic Failures | Low | ✅ Acceptable |
| A05: Injection | Low | ✅ Acceptable |
| A06: Insecure Design | Low | ✅ Acceptable |
| A07: Authentication Failures | Low | ✅ Acceptable |
| A08: Software or Data Integrity Failures | Medium | ⚠️ Needs attention |
| A09: Security Logging & Alerting Failures | Low-Medium | ⚠️ Needs attention |
| A10: Mishandling of Exceptional Conditions | Low | ✅ Acceptable |

---

## Detailed Analysis

### A01: Broken Access Control - ✅ LOW RISK

**Strengths:**
- Authorization checks on all protected routes (`canManageChannel`, `isAdmin`)
- Channel ownership properly scoped - owners can only manage their channels
- Admin-only routes (`/admin/owners`) protected with `isAdmin()` check
- Case-insensitive email comparison prevents bypass attempts
- Whitespace trimming on auth headers prevents header injection

**Minor concerns:**
- No CSRF protection on forms (mitigated by exe.dev proxy handling auth via headers, not cookies)

**Tests covering this:**
- `srv/auth_test.go` - comprehensive authorization tests

---

### A02: Security Misconfiguration - ⚠️ LOW-MEDIUM RISK

**Strengths:**
- No debug endpoints exposed in production
- Database uses parameterized queries via sqlc
- Health endpoint doesn't leak sensitive info

**Concerns:**
- [ ] No `Content-Security-Policy` header
- [ ] No `X-Frame-Options` header (clickjacking risk)
- [ ] No `X-Content-Type-Options` header (MIME sniffing)
- [ ] Static files cached with long TTL (1 year) - compromised assets won't be invalidated quickly

---

### A03: Software Supply Chain Failures - ⚠️ MEDIUM RISK

**Strengths:**
- Using well-known, maintained Go libraries
- sqlc generates code at build time (not runtime dependency)

**Concerns:**
- [ ] 273 lines in go.sum = many transitive dependencies
- [ ] Lucide icons loaded from CDN (`unpkg.com`) with `@latest` - unpinned version
- [ ] Google Fonts loaded from external CDN
- [ ] No Subresource Integrity (SRI) on external scripts
- [ ] No dependency scanning/auditing in place

**Affected templates:**
- All templates load: `https://unpkg.com/lucide@latest/dist/umd/lucide.min.js`
- All templates load: `https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap`

---

### A04: Cryptographic Failures - ✅ LOW RISK

**Strengths:**
- No sensitive data stored (no passwords, tokens, PII)
- Authentication handled entirely by exe.dev proxy
- No encryption requirements for this use case
- Database contains only public quote content

**Notes:**
- If future features require storing sensitive data, this should be re-evaluated

---

### A05: Injection - ✅ LOW RISK

**Strengths:**
- Using `html/template` package (auto-escapes HTML output)
- sqlc generates parameterized queries - no string concatenation SQL
- No raw SQL execution anywhere in codebase
- User input validated and trimmed before use
- Input length limits enforced (`MaxQuoteTextLen`, `MaxAuthorLen`, etc.)

**Verified:**
```go
// All database queries use sqlc-generated code:
q := dbgen.New(s.DB)
q.CreateQuote(ctx, dbgen.CreateQuoteParams{...})  // Parameterized
```

**Tests covering this:**
- `srv/validation_test.go` - input validation tests

---

### A06: Insecure Design - ✅ LOW RISK

**Strengths:**
- Rate limiting on API endpoints (30 req/min per IP, burst of 10)
- Separate rate limiting for suggestions (5 per hour per IP)
- Clear role separation: Admin → Channel Owner → Public
- Nightbot channel-based rate limiting (prevents per-viewer abuse)
- Defense in depth: validation at multiple layers

**Tests covering this:**
- `srv/ratelimit_test.go` - rate limiter tests

---

### A07: Authentication Failures - ✅ LOW RISK

**Strengths:**
- Authentication delegated to exe.dev proxy (not custom implementation)
- Email normalization: case-insensitive, whitespace trimmed
- No session management to get wrong (stateless via headers)
- No password storage or handling

**Trust assumptions:**
- exe.dev proxy is properly configured and trusted
- `X-ExeDev-UserID` and `X-ExeDev-Email` headers cannot be spoofed by clients
- Proxy strips these headers from incoming requests before adding authenticated values

---

### A08: Software or Data Integrity Failures - ⚠️ MEDIUM RISK

**Concerns:**
- [ ] CDN resources loaded without Subresource Integrity (SRI) hashes
- [ ] Using `@latest` for Lucide - version could change unexpectedly
- [ ] If unpkg.com is compromised, malicious JS could be injected
- [ ] No CI/CD pipeline verification of go.sum

**Example of vulnerable pattern:**
```html
<!-- Current (vulnerable) -->
<script src="https://unpkg.com/lucide@latest/dist/umd/lucide.min.js"></script>

<!-- Should be (with SRI) -->
<script src="https://unpkg.com/lucide@0.294.0/dist/umd/lucide.min.js"
        integrity="sha384-[hash]" crossorigin="anonymous"></script>
```

---

### A09: Security Logging & Alerting Failures - ⚠️ LOW-MEDIUM RISK

**Strengths:**
- OpenTelemetry tracing to Honeycomb
- Rate limit events recorded as span events
- Error logging with structured slog
- Request logging for slow/error responses

**Concerns:**
- [ ] No explicit security event logging for:
  - Failed authentication attempts
  - Permission denied events
  - Suspicious input patterns
- [ ] No alerting configured for:
  - Unusual rate limiting patterns
  - Repeated authorization failures
  - Error rate spikes

---

### A10: Mishandling of Exceptional Conditions - ✅ LOW RISK

**Strengths:**
- All errors handled explicitly (no panics in handlers)
- Database errors return generic 500, not raw error messages
- Input parsing errors return 400 with safe messages
- sql.ErrNoRows handled separately from other errors
- No ignored errors on critical paths

**Verified patterns:**
```go
if err != nil {
    if err == sql.ErrNoRows {
        http.Error(w, "Not found", http.StatusNotFound)
        return
    }
    slog.Error("operation failed", "error", err)
    http.Error(w, "Internal server error", http.StatusInternalServerError)
    return
}
```

---

## Remediation Checklist

### High Priority

- [x] **Add SRI hashes to CDN scripts** ✅ (2024-12-28)
  - Pinned Lucide to v0.462.0 with integrity hash
  - All 7 templates updated
  
- [x] **Pin CDN versions** ✅ (2024-12-28)
  - Changed `lucide@latest` to `lucide@0.462.0`
  - SRI hash: `sha384-8nT3SpButyvenpAdKYPJzXdSz3zidMGduMoaMvwjKnAWVv238n6P1mhveiJJQWrV`

### Medium Priority

- [x] **Add security headers middleware** ✅ (2024-12-28)
  - Added `SecurityHeaders` middleware in `srv/middleware.go`
  - `X-Frame-Options: DENY` - prevents clickjacking
  - `X-Content-Type-Options: nosniff` - prevents MIME sniffing
  - `Referrer-Policy: strict-origin-when-cross-origin`
  - `Content-Security-Policy` with restricted sources
  - Note: `'unsafe-inline'` needed for existing inline scripts/handlers

- [ ] **Add security event logging**
  - Log permission denied with user context
  - Log rate limit hits with endpoint context
  - Add structured fields for security filtering

- [ ] **Set up dependency scanning**
  - Add `govulncheck` to CI
  - Consider Dependabot or similar for alerts

### Lower Priority

- [ ] **Consider self-hosting static assets**
  - Download Lucide and serve from `/static/`
  - Download Inter font and serve locally
  - Eliminates CDN trust requirement

- [ ] **Add CSRF tokens to forms** (defense in depth)
  - Even though exe.dev uses header auth, CSRF tokens add another layer
  - Use `gorilla/csrf` or similar

- [ ] **Configure Honeycomb alerts**
  - Alert on error rate > threshold
  - Alert on rate limiting spike
  - Alert on 403/401 patterns

- [ ] **Review static file caching**
  - Consider shorter cache for CSS/JS during active development
  - Use cache-busting query params (already doing `?v=8`)

---

## Testing Recommendations

To maintain security posture, consider adding:

1. **Authorization boundary tests**
   - Test that channel owners can't access other channels
   - Test that non-owners can't access management routes
   - Test admin override works correctly

2. **Input fuzzing**
   - Fuzz quote text with special characters
   - Fuzz channel names with path traversal attempts
   - Test Unicode edge cases

3. **Rate limit integration tests**
   - Verify rate limiting works end-to-end
   - Test rate limit bypass attempts

---

## Conclusion

The application has a solid security foundation with proper authorization, parameterized queries, and rate limiting. The main areas for improvement are:

1. **Supply chain security** - Pin and verify external resources
2. **Security headers** - Add standard protective headers
3. **Security logging** - Better visibility into security events

None of the findings represent critical vulnerabilities requiring immediate action, but addressing the medium-priority items would significantly improve the security posture.
