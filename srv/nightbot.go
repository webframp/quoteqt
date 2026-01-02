package srv

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// NightbotCommand represents a custom command from Nightbot API
type NightbotCommand struct {
	ID        string `json:"_id,omitempty"`
	Name      string `json:"name"`
	Message   string `json:"message"`
	CoolDown  int    `json:"coolDown"`
	Count     int    `json:"count,omitempty"`
	UserLevel string `json:"userLevel"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// NightbotBackup represents the exported backup format
type NightbotBackup struct {
	ExportedAt  string             `json:"exportedAt"`
	Channel     string             `json:"channel"`
	CommandCount int               `json:"commandCount"`
	Commands    []NightbotCommand  `json:"commands"`
}

// nightbotChannelResponse represents channel info from Nightbot API
type nightbotChannelResponse struct {
	ID          string `json:"_id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Provider    string `json:"provider"`
}

// HandleNightbotAdmin shows the Nightbot backup/restore admin page
func (s *Server) HandleNightbotAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" {
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	if !s.isAdmin(userEmail) {
		RecordSecurityEvent(ctx, "admin_required",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
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
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		IsAdmin:         true,
		IsAuthenticated: true,
		IsPublicPage:    false,
		Success:         r.URL.Query().Get("success"),
		Error:           r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "admin_nightbot.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}



// HandleNightbotValidate validates and displays a backup file
func (s *Server) HandleNightbotValidate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" {
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	if !s.isAdmin(userEmail) {
		RecordSecurityEvent(ctx, "admin_required",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to parse upload"), http.StatusSeeOther)
		return
	}

	file, _, err := r.FormFile("backup")
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("No file uploaded"), http.StatusSeeOther)
		return
	}
	defer file.Close()

	var backup NightbotBackup
	if err := json.NewDecoder(file).Decode(&backup); err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Invalid backup file: "+err.Error()), http.StatusSeeOther)
		return
	}

	// Render validation result page
	data := struct {
		Hostname        string
		UserEmail       string
		LogoutURL       string
		IsAdmin         bool
		IsAuthenticated bool
		IsPublicPage    bool
		Backup          NightbotBackup
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		IsAdmin:         true,
		IsAuthenticated: true,
		IsPublicPage:    false,
		Backup:          backup,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "admin_nightbot_view.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}


