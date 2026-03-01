package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/webframp/quoteqt/db/dbgen"
	"go.opentelemetry.io/otel/attribute"
)

// StartManagedChannelSync starts the background sync job for managed channels.
// It checks every 5 minutes for channels due for sync.
func (s *Server) StartManagedChannelSync(ctx context.Context) {
	if s.Encryptor == nil {
		slog.Info("managed channel sync disabled: NIGHTBOT_SESSION_KEY not configured")
		return
	}

	go func() {
		// Run immediately on startup
		s.syncDueChannels(ctx)

		// Then check every 5 minutes
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("managed channel sync stopped")
				return
			case <-ticker.C:
				s.syncDueChannels(ctx)
			}
		}
	}()

	slog.Info("managed channel sync started")
}

func (s *Server) syncDueChannels(ctx context.Context) {
	q := dbgen.New(s.DB)

	channels, err := q.GetManagedChannelsDueForSync(ctx)
	if err != nil {
		slog.Error("get managed channels due for sync", "error", err)
		return
	}

	if len(channels) == 0 {
		return
	}

	slog.Info("syncing managed channels", "count", len(channels))

	for _, ch := range channels {
		if err := s.syncManagedChannel(ctx, ch); err != nil {
			slog.Error("sync managed channel",
				"channel", ch.ChannelName,
				"error", err)
		}

		// Small delay between channels to avoid hammering the API
		time.Sleep(nightbotAPIRateDelay)
	}
}

func (s *Server) syncManagedChannel(ctx context.Context, ch dbgen.NightbotManagedChannel) error {
	q := dbgen.New(s.DB)

	// Decrypt session token
	sessionToken, err := s.Encryptor.Decrypt(ch.SessionTokenEncrypted)
	if err != nil {
		_ = q.UpdateManagedChannelSyncStatus(ctx, dbgen.UpdateManagedChannelSyncStatusParams{
			LastSyncStatus: strPtr("decrypt_error"),
			LastError:      strPtr("failed to decrypt session token"),
			ID:             ch.ID,
		})
		return fmt.Errorf("decrypt token: %w", err)
	}

	// Fetch commands from Nightbot
	commands, channelInfo, err := s.fetchManagedChannelCommands(ctx, sessionToken, ch.ChannelID)
	if err != nil {
		status := "api_error"
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
			status = "auth_failed"
			// Disable sync on auth failure
			_ = q.DisableManagedChannelSync(ctx, ch.ID)
		}
		_ = q.UpdateManagedChannelSyncStatus(ctx, dbgen.UpdateManagedChannelSyncStatusParams{
			LastSyncStatus: strPtr(status),
			LastError:      strPtr(err.Error()),
			ID:             ch.ID,
		})
		return fmt.Errorf("fetch commands: %w", err)
	}

	// Create snapshot
	commandJSON, err := json.Marshal(commands)
	if err != nil {
		return fmt.Errorf("marshal commands: %w", err)
	}

	channelName := ch.ChannelName
	if channelInfo != nil && channelInfo.DisplayName != "" {
		channelName = channelInfo.DisplayName
	}

	_, err = q.CreateNightbotSnapshot(ctx, dbgen.CreateNightbotSnapshotParams{
		ChannelName:  channelName,
		CommandCount: int64(len(commands)),
		CommandsJson: string(commandJSON),
		CreatedBy:    "auto-sync",
		Note:         strPtr(fmt.Sprintf("Auto-sync from managed channel (interval: %dm)", ch.SyncIntervalMinutes)),
	})
	if err != nil {
		_ = q.UpdateManagedChannelSyncStatus(ctx, dbgen.UpdateManagedChannelSyncStatusParams{
			LastSyncStatus: strPtr("db_error"),
			LastError:      strPtr(err.Error()),
			ID:             ch.ID,
		})
		return fmt.Errorf("create snapshot: %w", err)
	}

	// Update sync status
	_ = q.UpdateManagedChannelSyncStatus(ctx, dbgen.UpdateManagedChannelSyncStatusParams{
		LastSyncStatus: strPtr("success"),
		LastError:      nil,
		ID:             ch.ID,
	})

	slog.Info("managed channel synced",
		"channel", channelName,
		"commands", len(commands))

	return nil
}

// fetchManagedChannelCommands fetches commands using a session token
func (s *Server) fetchManagedChannelCommands(ctx context.Context, sessionToken, channelID string) ([]NightbotCommand, *nightbotChannelResponse, error) {
	// Fetch commands
	req, err := http.NewRequestWithContext(ctx, "GET", nightbotAPIBase+"/commands", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Session "+sessionToken)
	if channelID != "" {
		req.Header.Set("Nightbot-Channel", channelID)
	}

	resp, err := nightbotAPICall(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("commands API returned %d: %s", resp.StatusCode, string(body))
	}

	var cmdResp struct {
		Commands []NightbotCommand `json:"commands"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
		return nil, nil, fmt.Errorf("decode commands: %w", err)
	}

	// Fetch channel info
	req2, err := http.NewRequestWithContext(ctx, "GET", nightbotAPIBase+"/channel", nil)
	if err != nil {
		return cmdResp.Commands, nil, nil // commands succeeded, channel info optional
	}
	req2.Header.Set("Authorization", "Session "+sessionToken)
	if channelID != "" {
		req2.Header.Set("Nightbot-Channel", channelID)
	}

	resp2, err := nightbotAPICall(ctx, req2)
	if err != nil {
		return cmdResp.Commands, nil, nil
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return cmdResp.Commands, nil, nil
	}

	var chanResp struct {
		Channel nightbotChannelResponse `json:"channel"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&chanResp); err != nil {
		return cmdResp.Commands, nil, nil
	}

	return cmdResp.Commands, &chanResp.Channel, nil
}

// HandleManagedChannelsAdmin shows the managed channels admin page
func (s *Server) HandleManagedChannelsAdmin(w http.ResponseWriter, r *http.Request) {
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

	// Check if feature is enabled
	if s.Encryptor == nil {
		http.Error(w, "Managed channels feature not enabled (NIGHTBOT_SESSION_KEY not configured)", http.StatusServiceUnavailable)
		return
	}

	q := dbgen.New(s.DB)
	channels, err := q.GetAllManagedChannels(ctx)
	if err != nil {
		slog.Error("get managed channels", "error", err)
		channels = nil
	}

	type ChannelView struct {
		ID                  int64
		ChannelID           string
		ChannelName         string
		SyncEnabled         bool
		SyncIntervalMinutes int64
		LastSyncAt          string
		LastSyncStatus      string
		LastError           string
		StatusClass         string // CSS class for status badge
	}

	var channelViews []ChannelView
	for _, ch := range channels {
		cv := ChannelView{
			ID:                  ch.ID,
			ChannelID:           ch.ChannelID,
			ChannelName:         ch.ChannelName,
			SyncEnabled:         ch.SyncEnabled == 1,
			SyncIntervalMinutes: ch.SyncIntervalMinutes,
		}
		if ch.LastSyncAt != nil {
			cv.LastSyncAt = formatTimeAgo(*ch.LastSyncAt)
		}
		if ch.LastSyncStatus != nil {
			cv.LastSyncStatus = *ch.LastSyncStatus
			switch *ch.LastSyncStatus {
			case "success":
				cv.StatusClass = "badge-success"
			case "auth_failed", "decrypt_error":
				cv.StatusClass = "badge-danger"
			case "disabled":
				cv.StatusClass = "badge-secondary"
			default:
				cv.StatusClass = "badge-warning"
			}
		}
		if ch.LastError != nil {
			cv.LastError = *ch.LastError
		}
		channelViews = append(channelViews, cv)
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
		Channels        []ChannelView
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		IsAdmin:         true,
		IsAuthenticated: true,
		IsPublicPage:    false,
		Success:         r.URL.Query().Get("success"),
		Error:           r.URL.Query().Get("error"),
		Channels:        channelViews,
	}

	if err := s.templates["admin_managed_channels.html"].Execute(w, data); err != nil {
		slog.Error("render managed channels template", "error", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// HandleManagedChannelAdd adds a new managed channel
func (s *Server) HandleManagedChannelAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" || !s.isAdmin(userEmail) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	if s.Encryptor == nil {
		http.Error(w, "Feature not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	channelID := strings.TrimSpace(r.FormValue("channel_id"))
	channelName := strings.TrimSpace(r.FormValue("channel_name"))
	sessionToken := strings.TrimSpace(r.FormValue("session_token"))
	intervalStr := r.FormValue("sync_interval")

	if channelID == "" || channelName == "" || sessionToken == "" {
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("All fields are required"), http.StatusSeeOther)
		return
	}

	interval := int64(60)
	if intervalStr != "" {
		if i, err := strconv.ParseInt(intervalStr, 10, 64); err == nil && i >= 15 {
			interval = i
		}
	}

	// Encrypt the session token
	encryptedToken, err := s.Encryptor.Encrypt(sessionToken)
	if err != nil {
		slog.Error("encrypt session token", "error", err)
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Failed to encrypt token"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	_, err = q.CreateManagedChannel(ctx, dbgen.CreateManagedChannelParams{
		UserEmail:             userEmail,
		ChannelID:             channelID,
		ChannelName:           channelName,
		SessionTokenEncrypted: encryptedToken,
		SyncEnabled:           1,
		SyncIntervalMinutes:   interval,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Channel already exists"), http.StatusSeeOther)
		} else {
			slog.Error("create managed channel", "error", err)
			http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Failed to add channel"), http.StatusSeeOther)
		}
		return
	}

	slog.Info("managed channel added",
		"channel", channelName,
		"by", userEmail)

	http.Redirect(w, r, "/admin/nightbot/managed?success="+url.QueryEscape("Channel added: "+channelName), http.StatusSeeOther)
}

// HandleManagedChannelToggle enables/disables sync for a channel
func (s *Server) HandleManagedChannelToggle(w http.ResponseWriter, r *http.Request) {
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
	action := r.FormValue("action") // "enable" or "disable"

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Invalid channel ID"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)

	if action == "enable" {
		err = q.EnableManagedChannelSync(ctx, id)
	} else {
		err = q.DisableManagedChannelSync(ctx, id)
	}

	if err != nil {
		slog.Error("toggle managed channel", "error", err)
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Failed to update channel"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/nightbot/managed?success="+url.QueryEscape("Channel updated"), http.StatusSeeOther)
}

// HandleManagedChannelDelete removes a managed channel
func (s *Server) HandleManagedChannelDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Invalid channel ID"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.DeleteManagedChannel(ctx, id); err != nil {
		slog.Error("delete managed channel", "error", err)
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Failed to delete channel"), http.StatusSeeOther)
		return
	}

	slog.Info("managed channel deleted", "id", id, "by", userEmail)
	http.Redirect(w, r, "/admin/nightbot/managed?success="+url.QueryEscape("Channel deleted"), http.StatusSeeOther)
}

// HandleManagedChannelSyncNow triggers an immediate sync for a channel
func (s *Server) HandleManagedChannelSyncNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" || !s.isAdmin(userEmail) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	if s.Encryptor == nil {
		http.Error(w, "Feature not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Invalid channel ID"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	ch, err := q.GetManagedChannel(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Channel not found"), http.StatusSeeOther)
		return
	}

	if err := s.syncManagedChannel(ctx, ch); err != nil {
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Sync failed: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/nightbot/managed?success="+url.QueryEscape("Synced: "+ch.ChannelName), http.StatusSeeOther)
}

// HandleManagedChannelUpdateToken updates the session token for a channel
func (s *Server) HandleManagedChannelUpdateToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userEmail == "" || !s.isAdmin(userEmail) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	if s.Encryptor == nil {
		http.Error(w, "Feature not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	sessionToken := strings.TrimSpace(r.FormValue("session_token"))

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Invalid channel ID"), http.StatusSeeOther)
		return
	}

	if sessionToken == "" {
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Session token is required"), http.StatusSeeOther)
		return
	}

	encryptedToken, err := s.Encryptor.Encrypt(sessionToken)
	if err != nil {
		slog.Error("encrypt session token", "error", err)
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Failed to encrypt token"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.UpdateManagedChannelToken(ctx, dbgen.UpdateManagedChannelTokenParams{
		SessionTokenEncrypted: encryptedToken,
		ID:                    id,
	}); err != nil {
		slog.Error("update managed channel token", "error", err)
		http.Redirect(w, r, "/admin/nightbot/managed?error="+url.QueryEscape("Failed to update token"), http.StatusSeeOther)
		return
	}

	// Re-enable sync after token update
	_ = q.EnableManagedChannelSync(ctx, id)

	slog.Info("managed channel token updated", "id", id, "by", userEmail)
	http.Redirect(w, r, "/admin/nightbot/managed?success="+url.QueryEscape("Token updated and sync re-enabled"), http.StatusSeeOther)
}

// Helper for creating string pointers
func strPtr(s string) *string {
	return &s
}
