package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/webframp/quoteqt/db/dbgen"
	"go.opentelemetry.io/otel/attribute"
)

const (
	nightbotAuthorizeURL = "https://api.nightbot.tv/oauth2/authorize"
	nightbotTokenURL     = "https://api.nightbot.tv/oauth2/token"
	nightbotAPIBase      = "https://api.nightbot.tv/1"
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

// nightbotRedirectURI returns the OAuth callback URL
func (s *Server) nightbotRedirectURI() string {
	scheme := "https"
	if strings.Contains(s.Hostname, "localhost") {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/admin/nightbot/callback", scheme, s.Hostname)
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

	// Get all connected channels for this user
	q := dbgen.New(s.DB)
	tokens, err := q.GetNightbotTokensByUser(ctx, userEmail)
	if err != nil {
		slog.Warn("get nightbot tokens", "error", err)
		tokens = nil
	}

	type ChannelInfo struct {
		Name        string
		DisplayName string
	}
	var channels []ChannelInfo
	for _, t := range tokens {
		displayName := t.ChannelName
		if t.ChannelDisplayName != nil && *t.ChannelDisplayName != "" {
			displayName = *t.ChannelDisplayName
		}
		channels = append(channels, ChannelInfo{
			Name:        t.ChannelName,
			DisplayName: displayName,
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
		Channels        []ChannelInfo
		ConnectURL      string
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		IsAdmin:         true,
		IsAuthenticated: true,
		IsPublicPage:    false,
		Success:         r.URL.Query().Get("success"),
		Error:           r.URL.Query().Get("error"),
		Channels:        channels,
		ConnectURL:      s.nightbotAuthURL(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "admin_nightbot.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

// nightbotAuthURL generates the OAuth authorization URL
func (s *Server) nightbotAuthURL() string {
	if s.Config.NightbotClientID == "" {
		return ""
	}
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", s.Config.NightbotClientID)
	params.Set("redirect_uri", s.nightbotRedirectURI())
	params.Set("scope", "commands channel")
	return nightbotAuthorizeURL + "?" + params.Encode()
}

// HandleNightbotCallback handles the OAuth callback from Nightbot
func (s *Server) HandleNightbotCallback(w http.ResponseWriter, r *http.Request) {
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

	code := r.URL.Query().Get("code")
	if code == "" {
		errorMsg := r.URL.Query().Get("error")
		if errorMsg == "" {
			errorMsg = "No authorization code received"
		}
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape(errorMsg), http.StatusSeeOther)
		return
	}

	// Exchange code for token
	tokenResp, err := s.exchangeNightbotCode(ctx, code)
	if err != nil {
		slog.Error("nightbot token exchange", "error", err)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to connect: "+err.Error()), http.StatusSeeOther)
		return
	}

	// Get channel info
	channel, err := s.getNightbotChannel(ctx, tokenResp.AccessToken)
	if err != nil {
		slog.Warn("get nightbot channel", "error", err)
	}

	// Also try /me endpoint to see user info
	if meResp, err := s.getNightbotMe(ctx, tokenResp.AccessToken); err == nil {
		slog.Info("nightbot /me response", "user", meResp)
	} else {
		slog.Warn("nightbot /me failed", "error", err)
	}

	// Log what we got from Nightbot
	if channel != nil {
		slog.Info("nightbot channel info", "name", channel.Name, "displayName", channel.DisplayName, "provider", channel.Provider, "id", channel.ID)
	} else {
		slog.Warn("nightbot channel info is nil")
	}

	// Store token - require channel info
	if channel == nil || channel.Name == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to get channel info"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	err = q.UpsertNightbotToken(ctx, dbgen.UpsertNightbotTokenParams{
		UserEmail:          userEmail,
		ChannelName:        channel.Name,
		ChannelDisplayName: toStringPtr(channel.DisplayName),
		AccessToken:        tokenResp.AccessToken,
		RefreshToken:       tokenResp.RefreshToken,
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		slog.Error("store nightbot token", "error", err)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to store token"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/nightbot?success="+url.QueryEscape("Connected to Nightbot!"), http.StatusSeeOther)
}

type nightbotTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (s *Server) exchangeNightbotCode(ctx context.Context, code string) (*nightbotTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", s.Config.NightbotClientID)
	data.Set("client_secret", s.Config.NightbotClientSecret)
	data.Set("redirect_uri", s.nightbotRedirectURI())
	data.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, "POST", nightbotTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(body))
	}

	var tokenResp nightbotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func (s *Server) refreshNightbotToken(ctx context.Context, refreshToken string) (*nightbotTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", s.Config.NightbotClientID)
	data.Set("client_secret", s.Config.NightbotClientSecret)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", nightbotTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed: %s - %s", resp.Status, string(body))
	}

	var tokenResp nightbotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func (s *Server) getNightbotMe(ctx context.Context, accessToken string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", nightbotAPIBase+"/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get /me failed: %s - %s", resp.Status, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Server) getNightbotChannel(ctx context.Context, accessToken string) (*nightbotChannelResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", nightbotAPIBase+"/channel", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get channel failed: %s", resp.Status)
	}

	var result struct {
		Channel nightbotChannelResponse `json:"channel"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result.Channel, nil
}

// getValidNightbotToken returns a valid access token, refreshing if needed
func (s *Server) getValidNightbotToken(ctx context.Context, userEmail, channelName string) (string, error) {
	q := dbgen.New(s.DB)
	token, err := q.GetNightbotToken(ctx, dbgen.GetNightbotTokenParams{
		UserEmail:   userEmail,
		ChannelName: channelName,
	})
	if err != nil {
		return "", fmt.Errorf("no nightbot token found for channel %s", channelName)
	}

	// Check if token is expired or about to expire (within 5 minutes)
	if time.Now().Add(5 * time.Minute).Before(token.ExpiresAt) {
		return token.AccessToken, nil
	}

	// Refresh the token
	newToken, err := s.refreshNightbotToken(ctx, token.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("failed to refresh token: %w", err)
	}

	// Store the new token
	expiresAt := time.Now().Add(time.Duration(newToken.ExpiresIn) * time.Second)
	err = q.UpsertNightbotToken(ctx, dbgen.UpsertNightbotTokenParams{
		UserEmail:          userEmail,
		ChannelName:        token.ChannelName,
		ChannelDisplayName: token.ChannelDisplayName,
		AccessToken:        newToken.AccessToken,
		RefreshToken:       newToken.RefreshToken,
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		slog.Warn("failed to store refreshed token", "error", err)
	}

	return newToken.AccessToken, nil
}

// HandleNightbotExport exports all custom commands as JSON
func (s *Server) HandleNightbotExport(w http.ResponseWriter, r *http.Request) {
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

	channelName := r.URL.Query().Get("channel")
	if channelName == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Channel parameter required"), http.StatusSeeOther)
		return
	}

	accessToken, err := s.getValidNightbotToken(ctx, userEmail, channelName)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Not connected to channel: "+channelName), http.StatusSeeOther)
		return
	}

	// Get channel info
	channel, err := s.getNightbotChannel(ctx, accessToken)
	if err != nil {
		slog.Warn("get channel for export", "error", err)
	}

	// Get commands
	commands, err := s.getNightbotCommands(ctx, accessToken)
	if err != nil {
		slog.Error("get nightbot commands", "error", err)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to fetch commands: "+err.Error()), http.StatusSeeOther)
		return
	}

	// Build backup - use display name if available
	displayName := channelName
	if channel != nil && channel.DisplayName != "" {
		displayName = channel.DisplayName
	}

	backup := NightbotBackup{
		ExportedAt:   time.Now().UTC().Format(time.RFC3339),
		Channel:      displayName,
		CommandCount: len(commands),
		Commands:     commands,
	}

	// Return as JSON download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="nightbot-commands-%s-%s.json"`, displayName, time.Now().Format("2006-01-02")))

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(backup); err != nil {
		slog.Error("encode backup", "error", err)
	}
}

func (s *Server) getNightbotCommands(ctx context.Context, accessToken string) ([]NightbotCommand, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", nightbotAPIBase+"/commands", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get commands failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Commands []NightbotCommand `json:"commands"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Clean up commands for export (remove server-generated fields we don't need)
	for i := range result.Commands {
		result.Commands[i].ID = ""
		result.Commands[i].Count = 0
		result.Commands[i].CreatedAt = ""
		result.Commands[i].UpdatedAt = ""
	}

	return result.Commands, nil
}

// HandleNightbotImport imports commands from a JSON backup
func (s *Server) HandleNightbotImport(w http.ResponseWriter, r *http.Request) {
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

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form first to get channel
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to parse upload"), http.StatusSeeOther)
		return
	}

	channelName := r.FormValue("channel")
	if channelName == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Channel is required"), http.StatusSeeOther)
		return
	}

	accessToken, err := s.getValidNightbotToken(ctx, userEmail, channelName)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Not connected to channel: "+channelName), http.StatusSeeOther)
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

	// Get existing commands to check for duplicates
	existing, err := s.getNightbotCommands(ctx, accessToken)
	if err != nil {
		slog.Warn("get existing commands", "error", err)
		existing = nil
	}

	existingNames := make(map[string]bool)
	for _, cmd := range existing {
		existingNames[strings.ToLower(cmd.Name)] = true
	}

	// Import commands
	var created, skipped int
	var errors []string

	for _, cmd := range backup.Commands {
		if existingNames[strings.ToLower(cmd.Name)] {
			skipped++
			continue
		}

		if err := s.createNightbotCommand(ctx, accessToken, cmd); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", cmd.Name, err))
			continue
		}
		created++
	}

	msg := fmt.Sprintf("Imported %d commands, skipped %d existing", created, skipped)
	if len(errors) > 0 {
		msg += fmt.Sprintf(", %d errors", len(errors))
	}

	http.Redirect(w, r, "/admin/nightbot?success="+url.QueryEscape(msg), http.StatusSeeOther)
}

func (s *Server) createNightbotCommand(ctx context.Context, accessToken string, cmd NightbotCommand) error {
	data := url.Values{}
	data.Set("name", cmd.Name)
	data.Set("message", cmd.Message)
	data.Set("coolDown", fmt.Sprintf("%d", cmd.CoolDown))
	data.Set("userLevel", cmd.UserLevel)

	req, err := http.NewRequestWithContext(ctx, "POST", nightbotAPIBase+"/commands", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s - %s", resp.Status, string(body))
	}

	return nil
}

func (s *Server) updateNightbotCommand(ctx context.Context, accessToken string, id string, cmd NightbotCommand) error {
	data := url.Values{}
	data.Set("message", cmd.Message)
	data.Set("coolDown", fmt.Sprintf("%d", cmd.CoolDown))
	data.Set("userLevel", cmd.UserLevel)

	req, err := http.NewRequestWithContext(ctx, "PUT", nightbotAPIBase+"/commands/"+id, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s - %s", resp.Status, string(body))
	}

	return nil
}

func (s *Server) deleteNightbotCommand(ctx context.Context, accessToken string, id string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", nightbotAPIBase+"/commands/"+id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s - %s", resp.Status, string(body))
	}

	return nil
}

// HandleNightbotDisconnect removes the stored Nightbot token for a channel
func (s *Server) HandleNightbotDisconnect(w http.ResponseWriter, r *http.Request) {
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

	channelName := r.URL.Query().Get("channel")
	if channelName == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Channel parameter required"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.DeleteNightbotToken(ctx, dbgen.DeleteNightbotTokenParams{
		UserEmail:   userEmail,
		ChannelName: channelName,
	}); err != nil {
		slog.Error("delete nightbot token", "error", err)
	}

	http.Redirect(w, r, "/admin/nightbot?success="+url.QueryEscape("Disconnected "+channelName), http.StatusSeeOther)
}

// toStringPtr converts a string to *string (nil if empty)
func toStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// int64Ptr converts an int64 to *int64
func int64Ptr(i int64) *int64 {
	return &i
}

// HandleNightbotSaveSnapshot saves current commands as a snapshot
func (s *Server) HandleNightbotSaveSnapshot(w http.ResponseWriter, r *http.Request) {
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

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	channelName := r.FormValue("channel")
	if channelName == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Channel parameter required"), http.StatusSeeOther)
		return
	}

	note := r.FormValue("note")

	// Get valid token for this channel
	accessToken, err := s.getValidNightbotToken(ctx, userEmail, channelName)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Not connected to channel: "+channelName), http.StatusSeeOther)
		return
	}

	// Fetch current commands from Nightbot
	commands, err := s.getNightbotCommands(ctx, accessToken)
	if err != nil {
		slog.Error("fetch nightbot commands for snapshot", "error", err, "channel", channelName)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to fetch commands: "+err.Error()), http.StatusSeeOther)
		return
	}

	// Serialize commands to JSON
	commandsJSON, err := json.Marshal(commands)
	if err != nil {
		slog.Error("marshal commands for snapshot", "error", err)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to save snapshot"), http.StatusSeeOther)
		return
	}

	// Save snapshot
	q := dbgen.New(s.DB)
	_, err = q.CreateNightbotSnapshot(ctx, dbgen.CreateNightbotSnapshotParams{
		ChannelName:  channelName,
		CommandCount: int64(len(commands)),
		CommandsJson: string(commandsJSON),
		CreatedBy:    userEmail,
		Note:         toStringPtr(note),
	})
	if err != nil {
		slog.Error("save nightbot snapshot", "error", err)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to save snapshot"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/nightbot?success="+url.QueryEscape(fmt.Sprintf("Saved snapshot with %d commands", len(commands))), http.StatusSeeOther)
}

// HandleNightbotSnapshots shows saved snapshots for a channel
func (s *Server) HandleNightbotSnapshots(w http.ResponseWriter, r *http.Request) {
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

	channelName := r.URL.Query().Get("channel")
	if channelName == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Channel parameter required"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	snapshots, err := q.GetNightbotSnapshots(ctx, dbgen.GetNightbotSnapshotsParams{
		ChannelName: channelName,
		Limit:       50,
	})
	if err != nil {
		slog.Error("get nightbot snapshots", "error", err)
		snapshots = nil
	}

	data := struct {
		ChannelName     string
		Snapshots       []dbgen.NightbotSnapshot
		Success         string
		Error           string
		IsAuthenticated bool
		IsAdmin         bool
		IsPublicPage    bool
		LogoutURL       string
	}{
		ChannelName:     channelName,
		Snapshots:       snapshots,
		Success:         r.URL.Query().Get("success"),
		Error:           r.URL.Query().Get("error"),
		IsAuthenticated: true,
		IsAdmin:         true, // Only admins can access this page
		IsPublicPage:    false,
		LogoutURL:       "/__exe.dev/logout",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "admin_nightbot_snapshots.html", data); err != nil {
		slog.Error("render snapshots template", "error", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// HandleNightbotSnapshotDownload downloads a snapshot as JSON
func (s *Server) HandleNightbotSnapshotDownload(w http.ResponseWriter, r *http.Request) {
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

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "Missing snapshot ID", http.StatusBadRequest)
		return
	}

	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Error(w, "Invalid snapshot ID", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	snapshot, err := q.GetNightbotSnapshot(ctx, id)
	if err != nil {
		http.Error(w, "Snapshot not found", http.StatusNotFound)
		return
	}

	// Parse stored commands
	var commands []NightbotCommand
	if err := json.Unmarshal([]byte(snapshot.CommandsJson), &commands); err != nil {
		http.Error(w, "Failed to parse snapshot", http.StatusInternalServerError)
		return
	}

	// Build backup format
	backup := NightbotBackup{
		ExportedAt:   snapshot.SnapshotAt.Format(time.RFC3339),
		Channel:      snapshot.ChannelName,
		CommandCount: len(commands),
		Commands:     commands,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="nightbot-snapshot-%s-%s.json"`,
		snapshot.ChannelName, snapshot.SnapshotAt.Format("2006-01-02-150405")))

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(backup)
}

// CommandDiff represents the diff status of a command
// CommandDiff represents the diff status of a command
type CommandDiff struct {
	Name       string
	Status     string // "added", "removed", "modified"
	UnifiedDiff string // git-style unified diff output
}

// HandleNightbotSnapshotDiff shows diff between snapshot and current config
func (s *Server) HandleNightbotSnapshotDiff(w http.ResponseWriter, r *http.Request) {
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

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Missing snapshot ID"), http.StatusSeeOther)
		return
	}

	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Invalid snapshot ID"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	snapshot, err := q.GetNightbotSnapshot(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Snapshot not found"), http.StatusSeeOther)
		return
	}

	// Get valid token for this channel
	accessToken, err := s.getValidNightbotToken(ctx, userEmail, snapshot.ChannelName)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Not connected to channel: "+snapshot.ChannelName), http.StatusSeeOther)
		return
	}

	// Fetch current commands from Nightbot
	currentCommands, err := s.getNightbotCommands(ctx, accessToken)
	if err != nil {
		slog.Error("fetch nightbot commands for diff", "error", err)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to fetch current commands"), http.StatusSeeOther)
		return
	}

	// Parse snapshot commands
	var snapshotCommands []NightbotCommand
	if err := json.Unmarshal([]byte(snapshot.CommandsJson), &snapshotCommands); err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to parse snapshot"), http.StatusSeeOther)
		return
	}

	// Build maps for comparison
	snapshotMap := make(map[string]NightbotCommand)
	for _, cmd := range snapshotCommands {
		snapshotMap[cmd.Name] = cmd
	}
	currentMap := make(map[string]NightbotCommand)
	for _, cmd := range currentCommands {
		currentMap[cmd.Name] = cmd
	}

	// Generate diff
	var diffs []CommandDiff
	var added, removed, modified, unchanged int

	// Helper to format command as text for diffing
	formatCmd := func(cmd NightbotCommand) string {
		return fmt.Sprintf("message: %s\ncooldown: %d\nuserlevel: %s", cmd.Message, cmd.CoolDown, cmd.UserLevel)
	}

	// Helper to generate unified diff
	genDiff := func(oldText, newText, oldName, newName string) string {
		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(oldText),
			B:        difflib.SplitLines(newText),
			FromFile: oldName,
			ToFile:   newName,
			Context:  3,
		}
		result, _ := difflib.GetUnifiedDiffString(diff)
		return result
	}

	// Check for removed and modified commands (in snapshot but changed/missing in current)
	for name, snapCmd := range snapshotMap {
		if curCmd, exists := currentMap[name]; exists {
			if snapCmd.Message != curCmd.Message || snapCmd.CoolDown != curCmd.CoolDown || snapCmd.UserLevel != curCmd.UserLevel {
				unifiedDiff := genDiff(
					formatCmd(snapCmd),
					formatCmd(curCmd),
					"snapshot/"+name,
					"current/"+name,
				)
				diffs = append(diffs, CommandDiff{
					Name:        name,
					Status:      "modified",
					UnifiedDiff: unifiedDiff,
				})
				modified++
			} else {
				unchanged++
			}
		} else {
			unifiedDiff := genDiff(
				formatCmd(snapCmd),
				"",
				"snapshot/"+name,
				"/dev/null",
			)
			diffs = append(diffs, CommandDiff{
				Name:        name,
				Status:      "removed",
				UnifiedDiff: unifiedDiff,
			})
			removed++
		}
	}

	// Check for added commands (in current but not in snapshot)
	for name, curCmd := range currentMap {
		if _, exists := snapshotMap[name]; !exists {
			unifiedDiff := genDiff(
				"",
				formatCmd(curCmd),
				"/dev/null",
				"current/"+name,
			)
			diffs = append(diffs, CommandDiff{
				Name:        name,
				Status:      "added",
				UnifiedDiff: unifiedDiff,
			})
			added++
		}
	}

	// Sort diffs: removed first, then modified, then added
	sort.Slice(diffs, func(i, j int) bool {
		order := map[string]int{"removed": 0, "modified": 1, "added": 2}
		if order[diffs[i].Status] != order[diffs[j].Status] {
			return order[diffs[i].Status] < order[diffs[j].Status]
		}
		return diffs[i].Name < diffs[j].Name
	})

	// Cache the diff results for display in snapshots list
	if err := q.UpdateSnapshotDiffCache(ctx, dbgen.UpdateSnapshotDiffCacheParams{
		LastDiffAdded:    int64Ptr(int64(added)),
		LastDiffRemoved:  int64Ptr(int64(removed)),
		LastDiffModified: int64Ptr(int64(modified)),
		ID:               snapshot.ID,
	}); err != nil {
		slog.Warn("update snapshot diff cache", "error", err)
	}

	data := struct {
		ChannelName      string
		SnapshotAt       string
		SnapshotID       int64
		SnapshotCount    int
		CurrentCount     int
		Diffs            []CommandDiff
		Added            int
		Removed          int
		Modified         int
		Unchanged        int
		HasChanges       bool
		IsAuthenticated  bool
		IsAdmin          bool
		IsPublicPage     bool
		LogoutURL        string
	}{
		ChannelName:      snapshot.ChannelName,
		SnapshotAt:       snapshot.SnapshotAt.Format("Jan 2, 2006 3:04 PM"),
		SnapshotID:       snapshot.ID,
		SnapshotCount:    len(snapshotCommands),
		CurrentCount:     len(currentCommands),
		Diffs:            diffs,
		Added:            added,
		Removed:          removed,
		Modified:         modified,
		Unchanged:        unchanged,
		HasChanges:       added > 0 || removed > 0 || modified > 0,
		IsAuthenticated:  true,
		IsAdmin:          true,
		IsPublicPage:     false,
		LogoutURL:        "/__exe.dev/logout",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "admin_nightbot_diff.html", data); err != nil {
		slog.Error("render diff template", "error", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// HandleNightbotSnapshotRestore restores a snapshot to Nightbot (full restore)
func (s *Server) HandleNightbotSnapshotRestore(w http.ResponseWriter, r *http.Request) {
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

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	if idStr == "" {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Missing snapshot ID"), http.StatusSeeOther)
		return
	}

	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Invalid snapshot ID"), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	snapshot, err := q.GetNightbotSnapshot(ctx, id)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Snapshot not found"), http.StatusSeeOther)
		return
	}

	// Get valid token for this channel
	accessToken, err := s.getValidNightbotToken(ctx, userEmail, snapshot.ChannelName)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Not connected to channel: "+snapshot.ChannelName), http.StatusSeeOther)
		return
	}

	// Parse snapshot commands
	var snapshotCommands []NightbotCommand
	if err := json.Unmarshal([]byte(snapshot.CommandsJson), &snapshotCommands); err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to parse snapshot"), http.StatusSeeOther)
		return
	}

	// Fetch current commands from Nightbot
	currentCommands, err := s.getNightbotCommands(ctx, accessToken)
	if err != nil {
		slog.Error("fetch nightbot commands for restore", "error", err)
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Failed to fetch current commands"), http.StatusSeeOther)
		return
	}

	// Build maps for comparison
	snapshotMap := make(map[string]NightbotCommand)
	for _, cmd := range snapshotCommands {
		snapshotMap[cmd.Name] = cmd
	}
	currentMap := make(map[string]NightbotCommand)
	for _, cmd := range currentCommands {
		currentMap[cmd.Name] = cmd
	}

	var created, updated, deleted, errors int

	// Delete commands that exist in current but not in snapshot
	for name, curCmd := range currentMap {
		if _, exists := snapshotMap[name]; !exists {
			if err := s.deleteNightbotCommand(ctx, accessToken, curCmd.ID); err != nil {
				slog.Warn("delete command during restore", "name", name, "error", err)
				errors++
			} else {
				deleted++
			}
		}
	}

	// Create or update commands from snapshot
	for name, snapCmd := range snapshotMap {
		if curCmd, exists := currentMap[name]; exists {
			// Update if different
			if snapCmd.Message != curCmd.Message || snapCmd.CoolDown != curCmd.CoolDown || snapCmd.UserLevel != curCmd.UserLevel {
				if err := s.updateNightbotCommand(ctx, accessToken, curCmd.ID, snapCmd); err != nil {
					slog.Warn("update command during restore", "name", name, "error", err)
					errors++
				} else {
					updated++
				}
			}
		} else {
			// Create new command
			if err := s.createNightbotCommand(ctx, accessToken, snapCmd); err != nil {
				slog.Warn("create command during restore", "name", name, "error", err)
				errors++
			} else {
				created++
			}
		}
	}

	msg := fmt.Sprintf("Restored snapshot: %d created, %d updated, %d deleted", created, updated, deleted)
	if errors > 0 {
		msg += fmt.Sprintf(", %d errors", errors)
	}

	slog.Info("snapshot restored", "channel", snapshot.ChannelName, "snapshot_id", id, "created", created, "updated", updated, "deleted", deleted, "errors", errors, "user", userEmail)

	http.Redirect(w, r, "/admin/nightbot/snapshots?channel="+url.QueryEscape(snapshot.ChannelName)+"&success="+url.QueryEscape(msg), http.StatusSeeOther)
}
