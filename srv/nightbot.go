package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

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

	// Check if user has connected Nightbot
	q := dbgen.New(s.DB)
	token, err := q.GetNightbotToken(ctx, userEmail)
	var connected bool
	var channelName string
	if err == nil {
		connected = true
		if token.ChannelDisplayName != nil {
			channelName = *token.ChannelDisplayName
		}
		if channelName == "" && token.ChannelName != nil {
			channelName = *token.ChannelName
		}
	}

	data := struct {
		Hostname         string
		UserEmail        string
		LogoutURL        string
		IsAdmin          bool
		IsAuthenticated  bool
		IsPublicPage     bool
		Success          string
		Error            string
		NightbotConnected bool
		NightbotChannel  string
		ConnectURL       string
	}{
		Hostname:         s.Hostname,
		UserEmail:        userEmail,
		LogoutURL:        "/__exe.dev/logout",
		IsAdmin:          true,
		IsAuthenticated:  true,
		IsPublicPage:     false,
		Success:          r.URL.Query().Get("success"),
		Error:            r.URL.Query().Get("error"),
		NightbotConnected: connected,
		NightbotChannel:  channelName,
		ConnectURL:       s.nightbotAuthURL(),
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

	// Store token
	q := dbgen.New(s.DB)
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	
	var channelName, channelDisplayName string
	if channel != nil {
		channelName = channel.Name
		channelDisplayName = channel.DisplayName
	}

	err = q.UpsertNightbotToken(ctx, dbgen.UpsertNightbotTokenParams{
		UserEmail:          userEmail,
		AccessToken:        tokenResp.AccessToken,
		RefreshToken:       tokenResp.RefreshToken,
		ExpiresAt:          expiresAt,
		ChannelName:        toStringPtr(channelName),
		ChannelDisplayName: toStringPtr(channelDisplayName),
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
func (s *Server) getValidNightbotToken(ctx context.Context, userEmail string) (string, error) {
	q := dbgen.New(s.DB)
	token, err := q.GetNightbotToken(ctx, userEmail)
	if err != nil {
		return "", fmt.Errorf("no nightbot token found")
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
		AccessToken:        newToken.AccessToken,
		RefreshToken:       newToken.RefreshToken,
		ExpiresAt:          expiresAt,
		ChannelName:        token.ChannelName,
		ChannelDisplayName: token.ChannelDisplayName,
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

	accessToken, err := s.getValidNightbotToken(ctx, userEmail)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Not connected to Nightbot"), http.StatusSeeOther)
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

	// Build backup
	channelName := "unknown"
	if channel != nil {
		channelName = channel.DisplayName
		if channelName == "" {
			channelName = channel.Name
		}
	}

	backup := NightbotBackup{
		ExportedAt:   time.Now().UTC().Format(time.RFC3339),
		Channel:      channelName,
		CommandCount: len(commands),
		Commands:     commands,
	}

	// Return as JSON download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="nightbot-commands-%s-%s.json"`, channelName, time.Now().Format("2006-01-02")))

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

	accessToken, err := s.getValidNightbotToken(ctx, userEmail)
	if err != nil {
		http.Redirect(w, r, "/admin/nightbot?error="+url.QueryEscape("Not connected to Nightbot"), http.StatusSeeOther)
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

// HandleNightbotDisconnect removes the stored Nightbot token
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

	q := dbgen.New(s.DB)
	if err := q.DeleteNightbotToken(ctx, userEmail); err != nil {
		slog.Error("delete nightbot token", "error", err)
	}

	http.Redirect(w, r, "/admin/nightbot?success="+url.QueryEscape("Disconnected from Nightbot"), http.StatusSeeOther)
}

// toStringPtr converts a string to *string (nil if empty)
func toStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
