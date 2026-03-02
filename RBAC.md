# RBAC Model

Role-Based Access Control implementation for QuoteQT.

## Roles (Hierarchical)

| Role | How Assigned | Scope |
|------|--------------|-------|
| **Admin** | `ADMIN_EMAILS` env var | Global |
| **Channel Owner** | `channel_owners` table (invited by admin) | Per-channel |
| **Channel Moderator** | `nightbot_channel_moderators` table (added by admin) | Per-channel (Nightbot only) |
| **Authenticated User** | exe.dev login | Limited |
| **Anonymous** | No login | Public pages only |

## Authentication Methods

| Method | Used By | How It Works |
|--------|---------|-------------|
| **exe.dev** | Admins, Channel Owners | Proxy sets `X-ExeDev-Email` header |
| **Twitch OAuth** | Channel Moderators | Session cookie from `/auth/twitch` flow |

## Permission Matrix

| Resource/Action | Admin | Channel Owner | Channel Moderator | Authenticated | Anonymous |
|-----------------|-------|---------------|-------------------|---------------|----------|
| **Quotes** |
| View all quotes | ✓ | Own channel | ✗ | ✗ | ✗ |
| Add/Edit/Delete quotes | ✓ | Own channel | ✗ | ✗ | ✗ |
| Browse quotes (public) | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Civilizations** |
| View/Edit civs | ✓ | ✓ | ✗ | ✗ | ✗ |
| **Suggestions** |
| Submit suggestion | ✓ | ✓ | ✓ | ✓ | ✓ |
| View/Approve/Reject | ✓ | Own channel | ✗ | ✗ | ✗ |
| **Nightbot Backup** |
| Admin page (`/admin/nightbot`) | ✓ | ✗ | ✗ | ✗ | ✗ |
| View snapshots | ✓ | Own channel | Assigned channel | ✗ | ✗ |
| Download snapshots | ✓ | Own channel | Assigned channel | ✗ | ✗ |
| Compare snapshots | ✓ | Own channel | Assigned channel | ✗ | ✗ |
| Save snapshot (OAuth) | ✓ | ✗ | ✗ | ✗ | ✗ |
| Restore/Delete snapshots | ✓ | ✗ | ✗ | ✗ | ✗ |
| Import commands | ✓ | ✗ | ✗ | ✗ | ✗ |
| Diff against live | ✓ | Own channel (OAuth) | ✗ | ✗ | ✗ |
| **Managed Channels** |
| Configure auto-sync | ✓ | ✗ | ✗ | ✗ | ✗ |
| **Administration** |
| Manage channel owners | ✓ | ✗ | ✗ | ✗ | ✗ |
| Manage channel moderators | ✓ | ✗ | ✗ | ✗ | ✗ |
| View users list | ✓ | ✗ | ✗ | ✗ | ✗ |

## Authorization Functions

Defined in `srv/server.go` and `srv/twitch_auth.go`:

```go
// Get unified auth info from request (exe.dev headers or Twitch session)
func (s *Server) getAuthInfo(r *http.Request) AuthInfo

// Check if user is admin (global)
func (s *Server) isAdmin(email string) bool

// Get channels user owns
func (s *Server) getOwnedChannels(ctx, email) ([]string, error)

// Check if user can manage a channel (admin OR owner)
func (s *Server) canManageChannel(ctx, email, channel) bool

// Check if user can view Nightbot snapshots (admin OR owner OR moderator)
// Use canViewNightbotChannelWithTwitch for Twitch-authenticated users
func (s *Server) canViewNightbotChannel(ctx, email, channel) bool
func (s *Server) canViewNightbotChannelWithTwitch(ctx, email, twitchUsername, channel) bool

// Get all channels user can view Nightbot data for
func (s *Server) getViewableNightbotChannels(ctx, email) ([]string, error)
func (s *Server) getViewableNightbotChannelsWithTwitch(ctx, email, twitchUsername) ([]string, error)
```

## Channel Owner vs Channel Moderator

| Capability | Channel Owner | Channel Moderator |
|------------|---------------|-------------------|
| Quotes CRUD | ✓ (own channel) | ✗ |
| Approve suggestions | ✓ (own channel) | ✗ |
| View Nightbot snapshots | ✓ | ✓ |
| Download snapshots | ✓ | ✓ |
| Compare snapshots | ✓ | ✓ |
| Diff against live (OAuth) | ✓ | ✗ |
| Restore snapshots | ✗ | ✗ |

## Database Tables

| Table | Purpose |
|-------|--------|
| `channel_owners` | Maps users → channels they own (quotes/suggestions access) |
| `nightbot_channel_moderators` | Maps users → channels they can view Nightbot data for (by email OR twitch_username) |
| `nightbot_tokens` | OAuth tokens for channel owners (write access to Nightbot API) |
| `nightbot_managed_channels` | Session tokens for admin auto-sync (read-only backup) |
| `twitch_sessions` | Active Twitch OAuth sessions for moderator authentication |

## Nightbot Access Types

| Type | Description | Capabilities |
|------|-------------|-------------|
| **OAuth Connected** | Channel owner authorized via Nightbot OAuth | Save, export, import, restore, diff vs live |
| **Managed Channel** | Admin configured auto-sync with session token | Automatic backups, view history |
| **Imported** | One-time Tampermonkey import | View history only |

## Implementation Notes

1. **Always normalize identifiers** - Use `strings.ToLower(strings.TrimSpace(...))` for emails and Twitch usernames
2. **Check authorization early** - Return 403 before doing any work
3. **Template defense-in-depth** - Even if handler checks auth, templates should also use `{{if .IsAdmin}}` for sensitive sections
4. **Log security events** - Use `RecordSecurityEvent()` for auth failures
5. **Use `getAuthInfo(r)`** - For handlers that support both exe.dev and Twitch auth
6. **Twitch moderators can't diff live** - They only have read access to snapshots, not Nightbot API access
