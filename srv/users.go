package srv

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/webframp/quoteqt/db/dbgen"
)

// userTracker debounces user tracking to avoid DB writes on every request
type userTracker struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time // userID -> last tracked time
}

var tracker = &userTracker{
	lastSeen: make(map[string]time.Time),
}

// shouldTrack returns true if we should record this user visit
// (debounces to once per 5 minutes per user)
func (t *userTracker) shouldTrack(userID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	last, ok := t.lastSeen[userID]
	if !ok || time.Since(last) > 5*time.Minute {
		t.lastSeen[userID] = time.Now()
		return true
	}
	return false
}

// UserTracking middleware records authenticated user visits
func (s *Server) UserTracking(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
		userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

		if userID != "" && userEmail != "" && tracker.shouldTrack(userID) {
			go func() {
				q := dbgen.New(s.DB)
				if err := q.UpsertUser(r.Context(), dbgen.UpsertUserParams{
					UserID: userID,
					Email:  strings.ToLower(userEmail),
				}); err != nil {
					slog.Warn("track user", "error", err)
				}
			}()
		}

		next.ServeHTTP(w, r)
	})
}

// HandleAdminUsers shows the user list for admins
func (s *Server) HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" {
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	if !s.isAdmin(userEmail) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	q := dbgen.New(s.DB)
	users, err := q.GetAllUsers(ctx)
	if err != nil {
		slog.Error("get users", "error", err)
		users = nil
	}

	type UserView struct {
		ID          int64
		UserID      string
		Email       string
		FirstSeenAt string
		LastSeenAt  string
		VisitCount  int64
		IsAdmin     bool
		IsOnline    bool // seen in last 15 minutes
	}

	var userViews []UserView
	for _, u := range users {
		userViews = append(userViews, UserView{
			ID:          u.ID,
			UserID:      u.UserID,
			Email:       u.Email,
			FirstSeenAt: formatTimeAgo(u.FirstSeenAt),
			LastSeenAt:  formatTimeAgo(u.LastSeenAt),
			VisitCount:  u.VisitCount,
			IsAdmin:     s.isAdmin(u.Email),
			IsOnline:    time.Since(u.LastSeenAt) < 15*time.Minute,
		})
	}

	data := struct {
		Hostname        string
		UserEmail       string
		LogoutURL       string
		IsAdmin         bool
		IsAuthenticated bool
		IsPublicPage    bool
		Success         string
		Error           string
		Users           []UserView
		TotalUsers      int
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		IsAdmin:         true,
		IsAuthenticated: true,
		IsPublicPage:    false,
		Users:           userViews,
		TotalUsers:      len(userViews),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["admin_users.html"].Execute(w, data); err != nil {
		slog.Error("render users template", "error", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}
