package srv

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/webframp/quoteqt/db/dbgen"
	"go.opentelemetry.io/otel/attribute"
)

// HandleNightbotModerators shows the moderator management page
func (s *Server) HandleNightbotModerators(w http.ResponseWriter, r *http.Request) {
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

	q := dbgen.New(s.DB)

	// Get all moderators
	moderators, err := q.GetAllModerators(ctx)
	if err != nil {
		slog.Error("get all moderators", "error", err)
		moderators = nil
	}

	// Get all channels that have snapshots (for the dropdown)
	channels, err := q.GetAllChannelsLastSnapshot(ctx)
	if err != nil {
		slog.Error("get channels", "error", err)
		channels = nil
	}

	type ModeratorView struct {
		ID             int64
		ChannelName    string
		UserEmail      string
		TwitchUsername string
		AddedBy        string
		AddedAt        string
	}

	var modViews []ModeratorView
	for _, m := range moderators {
		twitchUsername := ""
		if m.TwitchUsername != nil {
			twitchUsername = *m.TwitchUsername
		}
		userEmail := ""
		if m.UserEmail != nil {
			userEmail = *m.UserEmail
		}
		modViews = append(modViews, ModeratorView{
			ID:             m.ID,
			ChannelName:    m.ChannelName,
			UserEmail:      userEmail,
			TwitchUsername: twitchUsername,
			AddedBy:        m.AddedBy,
			AddedAt:        formatTimeAgo(m.AddedAt),
		})
	}

	var channelNames []string
	for _, c := range channels {
		channelNames = append(channelNames, c.ChannelName)
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
		Moderators      []ModeratorView
		Channels        []string
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		IsAdmin:         true,
		IsAuthenticated: true,
		IsPublicPage:    false,
		Success:         r.URL.Query().Get("success"),
		Error:           r.URL.Query().Get("error"),
		Moderators:      modViews,
		Channels:        channelNames,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["admin_nightbot_moderators.html"].Execute(w, data); err != nil {
		slog.Error("render moderators template", "error", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// HandleNightbotModeratorAdd adds a new moderator
func (s *Server) HandleNightbotModeratorAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" || !s.isAdmin(userEmail) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	channelName := strings.ToLower(strings.TrimSpace(r.FormValue("channel_name")))
	authType := r.FormValue("auth_type")
	modEmail := strings.ToLower(strings.TrimSpace(r.FormValue("user_email")))
	twitchUsername := strings.ToLower(strings.TrimSpace(r.FormValue("twitch_username")))

	if channelName == "" {
		http.Redirect(w, r, "/admin/nightbot/moderators?error="+url.QueryEscape("Channel is required"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	var identifier string

	if authType == "twitch" && twitchUsername != "" {
		// Add by Twitch username
		err := q.AddChannelModeratorByTwitch(ctx, dbgen.AddChannelModeratorByTwitchParams{
			ChannelName:    channelName,
			TwitchUsername: &twitchUsername,
			AddedBy:        userEmail,
		})
		if err != nil {
			slog.Error("add moderator by twitch", "error", err)
			http.Redirect(w, r, "/admin/nightbot/moderators?error="+url.QueryEscape("Failed to add moderator"), http.StatusSeeOther)
			return
		}
		identifier = "@" + twitchUsername
	} else if modEmail != "" {
		// Add by email
		err := q.AddChannelModerator(ctx, dbgen.AddChannelModeratorParams{
			ChannelName: channelName,
			UserEmail:   &modEmail,
			AddedBy:     userEmail,
		})
		if err != nil {
			slog.Error("add moderator", "error", err)
			http.Redirect(w, r, "/admin/nightbot/moderators?error="+url.QueryEscape("Failed to add moderator"), http.StatusSeeOther)
			return
		}
		identifier = modEmail
	} else {
		http.Redirect(w, r, "/admin/nightbot/moderators?error="+url.QueryEscape("Email or Twitch username is required"), http.StatusSeeOther)
		return
	}

	slog.Info("moderator added",
		"channel", channelName,
		"moderator", identifier,
		"by", userEmail)

	http.Redirect(w, r, "/admin/nightbot/moderators?success="+url.QueryEscape("Added "+identifier+" as moderator for "+channelName), http.StatusSeeOther)
}

// HandleNightbotModeratorRemove removes a moderator
func (s *Server) HandleNightbotModeratorRemove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" || !s.isAdmin(userEmail) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot/moderators?error="+url.QueryEscape("Invalid moderator ID"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.RemoveChannelModerator(ctx, id); err != nil {
		slog.Error("remove moderator", "error", err)
		http.Redirect(w, r, "/admin/nightbot/moderators?error="+url.QueryEscape("Failed to remove moderator"), http.StatusSeeOther)
		return
	}

	slog.Info("moderator removed", "id", id, "by", userEmail)
	http.Redirect(w, r, "/admin/nightbot/moderators?success="+url.QueryEscape("Moderator removed"), http.StatusSeeOther)
}
