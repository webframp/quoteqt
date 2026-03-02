package srv

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/webframp/quoteqt/db/dbgen"
)

const (
	twitchAuthURL    = "https://id.twitch.tv/oauth2/authorize"
	twitchTokenURL   = "https://id.twitch.tv/oauth2/token"
	twitchUsersURL   = "https://api.twitch.tv/helix/users"
	sessionCookieName = "quoteqt_session"
	sessionDuration   = 7 * 24 * time.Hour // 1 week
)

// TwitchUser represents user data from Twitch API
type TwitchUser struct {
	ID          string `json:"id"`
	Login       string `json:"login"`        // lowercase username
	DisplayName string `json:"display_name"` // display name with case
}

// TwitchSession represents an authenticated Twitch user session
type TwitchSession struct {
	TwitchID       string
	TwitchUsername string
	DisplayName    string
	ExpiresAt      time.Time
}

// HandleTwitchAuth initiates the Twitch OAuth flow
func (s *Server) HandleTwitchAuth(w http.ResponseWriter, r *http.Request) {
	if s.Config.TwitchClientID == "" {
		http.Error(w, "Twitch OAuth not configured", http.StatusServiceUnavailable)
		return
	}

	// Store the original destination in a cookie
	redirectTo := r.URL.Query().Get("redirect")
	if redirectTo == "" {
		redirectTo = "/"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_redirect",
		Value:    redirectTo,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	// Generate state parameter for CSRF protection
	state := generateState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	// Build authorization URL
	params := url.Values{}
	params.Set("client_id", s.Config.TwitchClientID)
	params.Set("redirect_uri", s.twitchRedirectURI())
	params.Set("response_type", "code")
	params.Set("scope", "") // We only need basic user info, no special scopes required
	params.Set("state", state)

	authURL := twitchAuthURL + "?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleTwitchCallback handles the OAuth callback from Twitch
func (s *Server) HandleTwitchCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify state parameter
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		slog.Warn("twitch oauth state mismatch")
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	// Check for error from Twitch
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Warn("twitch oauth error", "error", errParam, "description", errDesc)
		http.Redirect(w, r, "/?error="+url.QueryEscape("Twitch login failed: "+errDesc), http.StatusSeeOther)
		return
	}

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	accessToken, err := s.exchangeTwitchCode(ctx, code)
	if err != nil {
		slog.Error("twitch token exchange failed", "error", err)
		http.Redirect(w, r, "/?error="+url.QueryEscape("Failed to authenticate with Twitch"), http.StatusSeeOther)
		return
	}

	// Get user info from Twitch
	user, err := s.getTwitchUser(ctx, accessToken)
	if err != nil {
		slog.Error("twitch get user failed", "error", err)
		http.Redirect(w, r, "/?error="+url.QueryEscape("Failed to get Twitch user info"), http.StatusSeeOther)
		return
	}

	slog.Info("twitch user authenticated", "twitch_id", user.ID, "username", user.Login)

	// Create session
	sessionID, err := s.createTwitchSession(ctx, user)
	if err != nil {
		slog.Error("create twitch session failed", "error", err)
		http.Redirect(w, r, "/?error="+url.QueryEscape("Failed to create session"), http.StatusSeeOther)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})

	// Redirect to original destination
	redirectTo := "/"
	if cookie, err := r.Cookie("auth_redirect"); err == nil {
		redirectTo = cookie.Value
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "auth_redirect",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// HandleTwitchLogout clears the session and logs out
func (s *Server) HandleTwitchLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get and delete session from database
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		q := dbgen.New(s.DB)
		_ = q.DeleteTwitchSession(ctx, cookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// getTwitchSession retrieves the current Twitch session from the request
func (s *Server) getTwitchSession(r *http.Request) *TwitchSession {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}

	q := dbgen.New(s.DB)
	session, err := q.GetTwitchSession(r.Context(), cookie.Value)
	if err != nil {
		return nil
	}

	return &TwitchSession{
		TwitchID:       session.TwitchID,
		TwitchUsername: session.TwitchUsername,
		DisplayName:    stringVal(session.DisplayName),
		ExpiresAt:      session.ExpiresAt,
	}
}

// twitchRedirectURI returns the OAuth callback URL
func (s *Server) twitchRedirectURI() string {
	scheme := "https"
	if strings.HasPrefix(s.Hostname, "localhost") {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/auth/twitch/callback", scheme, s.Hostname)
}

// exchangeTwitchCode exchanges an authorization code for an access token
func (s *Server) exchangeTwitchCode(ctx context.Context, code string) (string, error) {
	data := url.Values{}
	data.Set("client_id", s.Config.TwitchClientID)
	data.Set("client_secret", s.Config.TwitchClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", s.twitchRedirectURI())

	req, err := http.NewRequestWithContext(ctx, "POST", twitchTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

// getTwitchUser fetches the authenticated user's info from Twitch
func (s *Server) getTwitchUser(ctx context.Context, accessToken string) (*TwitchUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", twitchUsersURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", s.Config.TwitchClientID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Data []TwitchUser `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no user data returned")
	}

	return &result.Data[0], nil
}

// createTwitchSession creates a new session in the database
func (s *Server) createTwitchSession(ctx context.Context, user *TwitchUser) (string, error) {
	// Generate session ID
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	sessionID := base64.URLEncoding.EncodeToString(b)

	// Sign the session ID
	signedID := s.signSessionID(sessionID)

	expiresAt := time.Now().Add(sessionDuration)

	q := dbgen.New(s.DB)
	err := q.CreateTwitchSession(ctx, dbgen.CreateTwitchSessionParams{
		ID:             signedID,
		TwitchID:       user.ID,
		TwitchUsername: strings.ToLower(user.Login),
		DisplayName:    &user.DisplayName,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return "", err
	}

	return signedID, nil
}

// signSessionID creates an HMAC signature for the session ID
func (s *Server) signSessionID(sessionID string) string {
	mac := hmac.New(sha256.New, []byte(s.Config.SessionSecret))
	mac.Write([]byte(sessionID))
	signature := hex.EncodeToString(mac.Sum(nil))
	return sessionID + "." + signature[:16] // Use first 16 chars of signature
}

// generateState generates a random state parameter for CSRF protection
func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// stringVal safely dereferences a string pointer
func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// cleanupExpiredSessions removes expired Twitch sessions periodically
func (s *Server) cleanupExpiredSessions() {
	q := dbgen.New(s.DB)
	if err := q.DeleteExpiredTwitchSessions(context.Background()); err != nil {
		slog.Warn("cleanup expired sessions", "error", err)
	}
}

// AuthInfo contains authentication details from either exe.dev or Twitch
type AuthInfo struct {
	// From exe.dev headers
	Email  string
	UserID string

	// From Twitch session
	TwitchUsername string
	TwitchID       string
	DisplayName    string

	// Computed
	IsAuthenticated bool
	IsAdmin         bool
	AuthMethod      string // "exedev" or "twitch" or ""
}

// getAuthInfo gets authentication from both exe.dev headers and Twitch session
func (s *Server) getAuthInfo(r *http.Request) AuthInfo {
	info := AuthInfo{}

	// Check exe.dev headers first (admin/owner auth)
	info.Email = strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	info.UserID = strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))

	if info.Email != "" {
		info.IsAuthenticated = true
		info.AuthMethod = "exedev"
		info.IsAdmin = s.isAdmin(info.Email)
		return info
	}

	// Check Twitch session (moderator auth)
	if session := s.getTwitchSession(r); session != nil {
		info.TwitchUsername = session.TwitchUsername
		info.TwitchID = session.TwitchID
		info.DisplayName = session.DisplayName
		info.IsAuthenticated = true
		info.AuthMethod = "twitch"
		// Twitch users are never admins
		info.IsAdmin = false
	}

	return info
}

// DisplayIdentity returns a user-friendly identifier for the authenticated user
func (a AuthInfo) DisplayIdentity() string {
	if a.Email != "" {
		return a.Email
	}
	if a.DisplayName != "" {
		return a.DisplayName
	}
	if a.TwitchUsername != "" {
		return a.TwitchUsername
	}
	return ""
}
