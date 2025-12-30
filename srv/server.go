package srv

// @title AoE4 Quote Database API
// @version 1.0
// @description API for Age of Empires IV quotes and matchup tips. Designed for chat bots (Nightbot, Moobot) and stream overlays.
// @termsOfService https://quotes.exe.dev/terms

// @contact.name API Support
// @contact.url https://quotes.exe.dev

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @BasePath /api
// @schemes https http

// @tag.name quotes
// @tag.description Get random quotes, optionally filtered by civilization
// @tag.name matchups
// @tag.description Get matchup-specific tips for civ vs civ scenarios
// @tag.name suggestions
// @tag.description Submit quote suggestions for review

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"github.com/webframp/quoteqt/db"
	"github.com/webframp/quoteqt/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	APILimiter   *RateLimiter
	AdminEmails  map[string]bool
	Markers      *MarkerClient
	templates    map[string]*template.Template
	httpServer   *http.Server
}

type pageData struct {
	Hostname    string
	Now         string
	UserEmail   string
	UserID      string
	LoginURL    string
	LogoutURL   string
	Quotes      []QuoteView
	Error       string
	Success     string
	QuoteCount  int64
	LastUpdated string
	Civs        []CivWithCount
	// Pagination
	Page       int
	PageSize   int
	TotalPages int
	HasPrev    bool
	HasNext    bool
	// Authorization
	IsAdmin         bool
	IsAuthenticated bool
	IsPublicPage    bool
	OwnedChannels   []string
	// Filtering
	Channels        []string
	SelectedChannel string
}

type QuoteView struct {
	ID           int64
	Text         string
	Author       string
	Civilization string
	OpponentCiv  string
	Channel      string
	CreatedBy    string
	RequestedBy  string
	CreatedAt    string
}

type CivWithCount struct {
	ID         int64
	Name       string
	Shortname  string
	VariantOf  string
	Dlc        string
	QuoteCount int64
}

func New(dbPath, hostname string, adminEmails []string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)

	adminSet := make(map[string]bool)
	for _, email := range adminEmails {
		email = strings.TrimSpace(strings.ToLower(email))
		if email != "" {
			adminSet[email] = true
		}
	}

	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
		// Rate limit: 30 requests per minute per IP, burst of 10
		APILimiter:   NewRateLimiter(30, time.Minute, 10),
		AdminEmails:  adminSet,
		Markers:      NewMarkerClient(),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	if err := srv.loadTemplates(); err != nil {
		return nil, err
	}

	// Create deploy marker on startup
	srv.Markers.CreateDeployMarker()

	return srv, nil
}

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	if err := s.DB.PingContext(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "unhealthy: database unreachable")
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	q := dbgen.New(s.DB)
	count, _ := q.CountQuotes(r.Context())

	var lastUpdated string
	if ts, err := q.GetLastUpdated(r.Context()); err == nil {
		lastUpdated = formatTimeAgo(ts)
	}

	data := pageData{
		Hostname:    s.Hostname,
		Now:         time.Now().Format(time.RFC3339),
		UserEmail:   userEmail,
		UserID:      userID,
		LoginURL:    loginURLForRequest(r),
		LogoutURL:   "/__exe.dev/logout",
		QuoteCount:  count,
		LastUpdated: lastUpdated,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "index.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func quotesToViews(quotes []dbgen.Quote) []QuoteView {
	views := make([]QuoteView, len(quotes))
	for i, q := range quotes {
		createdBy := q.UserID
		if q.CreatedByEmail != nil && *q.CreatedByEmail != "" {
			createdBy = *q.CreatedByEmail
		}
		views[i] = QuoteView{
			ID:        q.ID,
			Text:      q.Text,
			CreatedBy: createdBy,
			CreatedAt: formatTimeAgo(q.CreatedAt),
		}
		if q.Author != nil {
			views[i].Author = *q.Author
		}
		if q.Civilization != nil {
			views[i].Civilization = *q.Civilization
		}
		if q.OpponentCiv != nil {
			views[i].OpponentCiv = *q.OpponentCiv
		}
		if q.Channel != nil {
			views[i].Channel = *q.Channel
		}
		if q.RequestedBy != nil {
			views[i].RequestedBy = *q.RequestedBy
		}
	}
	return views
}

func (s *Server) HandleQuotes(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userID == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	isAdmin := s.isAdmin(userEmail)
	ownedChannels, _ := s.getOwnedChannels(ctx, userEmail)

	// If not admin and not a channel owner, deny access
	if !isAdmin && len(ownedChannels) == 0 {
		RecordSecurityEvent(ctx, "permission_denied",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
			attribute.String("reason", "not_channel_owner"),
		)
		http.Error(w, "You don't have permission to manage quotes. Contact an admin to get access.", http.StatusForbidden)
		return
	}

	q := dbgen.New(s.DB)
	var quotes []dbgen.Quote
	var err error

	if isAdmin {
		// Admins see all quotes
		quotes, err = q.ListAllQuotes(ctx)
	} else {
		// Channel owners see only their channel's quotes
		// For now, just use the first owned channel (most users will own one)
		// TODO: add channel selector if user owns multiple channels
		quotes, err = q.ListQuotesByChannelOnly(ctx, &ownedChannels[0])
	}
	if err != nil {
		slog.Error("list quotes", "error", err)
	}

	data := pageData{
		Hostname:        s.Hostname,
		Now:             time.Now().Format(time.RFC3339),
		UserEmail:       userEmail,
		UserID:          userID,
		LoginURL:        loginURLForRequest(r),
		LogoutURL:       "/__exe.dev/logout",
		Quotes:          quotesToViews(quotes),
		Success:         r.URL.Query().Get("success"),
		IsAdmin:         isAdmin,
		IsAuthenticated: true,
		OwnedChannels:   ownedChannels,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "quotes.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAddQuote(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userID == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(r.FormValue("text"))
	author := strings.TrimSpace(r.FormValue("author"))
	civ := strings.TrimSpace(r.FormValue("civilization"))
	opponentCiv := strings.TrimSpace(r.FormValue("opponent_civ"))
	channel := strings.TrimSpace(r.FormValue("channel"))

	// Check permission: must be admin or own this channel
	if !s.canManageChannel(ctx, userEmail, channel) {
		RecordSecurityEvent(ctx, "permission_denied",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
			attribute.String("resource", "quote"),
			attribute.String("channel", channel),
			attribute.String("reason", "not_channel_owner"),
		)
		http.Error(w, "You don't have permission to add quotes to this channel", http.StatusForbidden)
		return
	}

	// Validate inputs
	if err := ValidateQuoteText(text); err != nil {
		http.Redirect(w, r, "/quotes?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := ValidateAuthor(author); err != nil {
		http.Redirect(w, r, "/quotes?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	var authorPtr, civPtr, opponentPtr, channelPtr *string
	if author != "" {
		authorPtr = &author
	}
	if civ != "" {
		civPtr = &civ
	}
	if opponentCiv != "" {
		opponentPtr = &opponentCiv
	}
	if channel != "" {
		channelPtr = &channel
	}

	var emailPtr *string
	if userEmail != "" {
		emailPtr = &userEmail
	}

	err := q.CreateQuote(r.Context(), dbgen.CreateQuoteParams{
		UserID:         userID,
		CreatedByEmail: emailPtr,
		Text:           text,
		Author:         authorPtr,
		Civilization:   civPtr,
		OpponentCiv:    opponentPtr,
		Channel:        channelPtr,
		RequestedBy:    nil, // No requester for directly added quotes
		CreatedAt:      time.Now(),
	})
	if err != nil {
		slog.Error("create quote", "error", err)
		http.Redirect(w, r, "/quotes?error=Failed+to+save+quote", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/quotes?success=Quote+added!", http.StatusSeeOther)
}

func (s *Server) HandleCivs(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userID == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	civs, err := q.ListCivsWithQuoteCount(r.Context())
	if err != nil {
		slog.Error("list civs", "error", err)
	}

	civsWithCount := make([]CivWithCount, len(civs))
	for i, civ := range civs {
		var shortname, variantOf, dlc string
		if civ.Shortname != nil {
			shortname = *civ.Shortname
		}
		if civ.VariantOf != nil {
			variantOf = *civ.VariantOf
		}
		if civ.Dlc != nil {
			dlc = *civ.Dlc
		}
		civsWithCount[i] = CivWithCount{
			ID:         civ.ID,
			Name:       civ.Name,
			Shortname:  shortname,
			VariantOf:  variantOf,
			Dlc:        dlc,
			QuoteCount: civ.QuoteCount,
		}
	}

	data := pageData{
		Hostname:        s.Hostname,
		Now:             time.Now().Format(time.RFC3339),
		UserEmail:       userEmail,
		UserID:          userID,
		LoginURL:        loginURLForRequest(r),
		LogoutURL:       "/__exe.dev/logout",
		Civs:            civsWithCount,
		Success:         r.URL.Query().Get("success"),
		Error:           r.URL.Query().Get("error"),
		IsAdmin:         s.isAdmin(userEmail),
		IsAuthenticated: true,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "civs.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAddCiv(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	ctx := r.Context()

	if userID == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	shortname := strings.TrimSpace(r.FormValue("shortname"))
	variantOf := strings.TrimSpace(r.FormValue("variant_of"))
	dlc := strings.TrimSpace(r.FormValue("dlc"))

	// Validate inputs
	if err := ValidateCivName(name); err != nil {
		http.Redirect(w, r, "/civs?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := ValidateShortname(shortname); err != nil {
		http.Redirect(w, r, "/civs?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := ValidateDLC(dlc); err != nil {
		http.Redirect(w, r, "/civs?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	var shortnamePtr, variantPtr, dlcPtr *string
	if shortname != "" {
		shortnamePtr = &shortname
	}
	if variantOf != "" {
		variantPtr = &variantOf
	}
	if dlc != "" {
		dlcPtr = &dlc
	}

	err := q.CreateCiv(r.Context(), dbgen.CreateCivParams{
		Name:      name,
		Shortname: shortnamePtr,
		VariantOf: variantPtr,
		Dlc:       dlcPtr,
	})
	if err != nil {
		slog.Error("create civ", "error", err)
		http.Redirect(w, r, "/civs?error=Failed+to+add+civilization", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/civs?success=Civilization+added!", http.StatusSeeOther)
}

func (s *Server) HandleEditCiv(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	if userID == "" {
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	shortname := strings.TrimSpace(r.FormValue("shortname"))
	variantOf := strings.TrimSpace(r.FormValue("variant_of"))
	dlc := strings.TrimSpace(r.FormValue("dlc"))

	// Validate inputs
	if err := ValidateCivName(name); err != nil {
		http.Redirect(w, r, "/civs?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := ValidateShortname(shortname); err != nil {
		http.Redirect(w, r, "/civs?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := ValidateDLC(dlc); err != nil {
		http.Redirect(w, r, "/civs?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	var shortnamePtr, variantPtr, dlcPtr *string
	if shortname != "" {
		shortnamePtr = &shortname
	}
	if variantOf != "" {
		variantPtr = &variantOf
	}
	if dlc != "" {
		dlcPtr = &dlc
	}

	err = q.UpdateCiv(r.Context(), dbgen.UpdateCivParams{
		ID:        id,
		Name:      name,
		Shortname: shortnamePtr,
		VariantOf: variantPtr,
		Dlc:       dlcPtr,
	})
	if err != nil {
		slog.Error("update civ", "error", err)
		http.Redirect(w, r, "/civs?error=Failed+to+update+civilization", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/civs?success=Civilization+updated!", http.StatusSeeOther)
}

func (s *Server) HandleDeleteCiv(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)

	// Check if civ has quotes before deleting
	civ, err := q.GetCivByID(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Redirect(w, r, "/civs?error=Civilization+not+found", http.StatusSeeOther)
			return
		}
		slog.Error("get civ", "error", err)
		http.Redirect(w, r, "/civs?error=Failed+to+delete+civilization", http.StatusSeeOther)
		return
	}

	count, _ := q.CountQuotesByCiv(r.Context(), &civ.Name)
	if count > 0 {
		http.Redirect(w, r, fmt.Sprintf("/civs?error=Cannot+delete:+%d+quotes+reference+this+civilization", count), http.StatusSeeOther)
		return
	}

	err = q.DeleteCiv(r.Context(), id)
	if err != nil {
		slog.Error("delete civ", "error", err)
		http.Redirect(w, r, "/civs?error=Failed+to+delete+civilization", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/civs?success=Civilization+deleted", http.StatusSeeOther)
}

func (s *Server) HandleEditQuote(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userID == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)

	// Get the quote to check permission
	quote, err := q.GetQuoteByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Quote not found", http.StatusNotFound)
			return
		}
		slog.Error("get quote", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check permission: must be admin or own this channel
	existingChannel := ""
	if quote.Channel != nil {
		existingChannel = *quote.Channel
	}
	if !s.canManageChannel(ctx, userEmail, existingChannel) {
		RecordSecurityEvent(ctx, "permission_denied",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
			attribute.String("resource", "quote"),
			attribute.Int64("quote.id", id),
			attribute.String("channel", existingChannel),
			attribute.String("reason", "not_channel_owner"),
		)
		http.Error(w, "You don't have permission to edit this quote", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(r.FormValue("text"))
	author := strings.TrimSpace(r.FormValue("author"))
	civ := strings.TrimSpace(r.FormValue("civilization"))
	opponentCiv := strings.TrimSpace(r.FormValue("opponent_civ"))
	channel := strings.TrimSpace(r.FormValue("channel"))

	// Validate inputs
	if err := ValidateQuoteText(text); err != nil {
		http.Redirect(w, r, "/quotes?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := ValidateAuthor(author); err != nil {
		http.Redirect(w, r, "/quotes?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	var authorPtr, civPtr, opponentPtr, channelPtr *string
	if author != "" {
		authorPtr = &author
	}
	if civ != "" {
		civPtr = &civ
	}
	if opponentCiv != "" {
		opponentPtr = &opponentCiv
	}
	if channel != "" {
		channelPtr = &channel
	}

	err = q.UpdateQuote(r.Context(), dbgen.UpdateQuoteParams{
		ID:           id,
		Text:         text,
		Author:       authorPtr,
		Civilization: civPtr,
		OpponentCiv:  opponentPtr,
		Channel:      channelPtr,
	})
	if err != nil {
		slog.Error("update quote", "error", err)
		http.Redirect(w, r, "/quotes?error=Failed+to+update+quote", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/quotes?success=Quote+updated!", http.StatusSeeOther)
}

func (s *Server) HandleDeleteQuote(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userID == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)

	// Get the quote to check permission
	quote, err := q.GetQuoteByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Quote not found", http.StatusNotFound)
			return
		}
		slog.Error("get quote", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check permission: must be admin or own this channel
	channel := ""
	if quote.Channel != nil {
		channel = *quote.Channel
	}
	if !s.canManageChannel(ctx, userEmail, channel) {
		RecordSecurityEvent(ctx, "permission_denied",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
			attribute.String("resource", "quote"),
			attribute.Int64("quote.id", id),
			attribute.String("channel", channel),
			attribute.String("reason", "not_channel_owner"),
		)
		http.Error(w, "You don't have permission to delete this quote", http.StatusForbidden)
		return
	}

	err = q.DeleteQuoteByID(ctx, id)
	if err != nil {
		slog.Error("delete quote", "error", err)
	}

	http.Redirect(w, r, "/quotes?success=Quote+deleted", http.StatusSeeOther)
}

type BulkRequest struct {
	IDs    []int64 `json:"ids"`
	Action string  `json:"action"`
	Value  string  `json:"value"`
}

func (s *Server) HandleBulkQuotes(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req BulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.IDs) == 0 {
		http.Error(w, "No quotes selected", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	var err error

	switch req.Action {
	case "channel":
		var channelPtr *string
		if req.Value != "" {
			channelPtr = &req.Value
		}
		err = q.BulkUpdateChannel(r.Context(), dbgen.BulkUpdateChannelParams{
			Channel: channelPtr,
			Ids:     req.IDs,
		})
	case "civilization":
		var civPtr *string
		if req.Value != "" {
			civPtr = &req.Value
		}
		err = q.BulkUpdateCivilization(r.Context(), dbgen.BulkUpdateCivilizationParams{
			Civilization: civPtr,
			Ids:          req.IDs,
		})
	case "clear-channel":
		err = q.BulkUpdateChannel(r.Context(), dbgen.BulkUpdateChannelParams{
			Channel: nil,
			Ids:     req.IDs,
		})
	case "delete":
		err = q.BulkDeleteQuotes(r.Context(), req.IDs)
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		slog.Error("bulk action failed", "action", req.Action, "error", err)
		http.Error(w, "Failed to apply action", http.StatusInternalServerError)
		return
	}

	// Create marker for bulk operation
	var opDesc string
	switch req.Action {
	case "channel":
		opDesc = fmt.Sprintf("Bulk set channel to '%s'", req.Value)
	case "civilization":
		opDesc = fmt.Sprintf("Bulk set civilization to '%s'", req.Value)
	case "clear-channel":
		opDesc = "Bulk clear channel"
	case "delete":
		opDesc = "Bulk delete"
	}
	s.Markers.CreateBulkOperationMarker(opDesc, len(req.IDs))

	slog.Info("bulk action completed", "action", req.Action, "count", len(req.IDs), "user", userID)
	w.WriteHeader(http.StatusOK)
}

type QuoteResponse struct {
	ID           int64   `json:"id"`
	Text         string  `json:"text"`
	Author       *string `json:"author,omitempty"`
	Civilization *string `json:"civilization,omitempty"`
	OpponentCiv  *string `json:"opponent_civ,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

const defaultPageSize = 20

func (s *Server) HandleQuotesPublic(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	ctx := r.Context()

	// Parse pagination params
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	// Parse channel filter
	selectedChannel := strings.TrimSpace(r.URL.Query().Get("channel"))

	// Get list of channels for the filter dropdown
	channelPtrs, _ := q.ListChannels(ctx)
	var channels []string
	for _, ch := range channelPtrs {
		if ch != nil {
			channels = append(channels, *ch)
		}
	}

	// Get count and quotes based on filter
	var count int64
	var quotes []dbgen.Quote
	var err error

	if selectedChannel != "" {
		count, _ = q.CountQuotesByChannel(ctx, &selectedChannel)
		totalPages := int((count + defaultPageSize - 1) / defaultPageSize)
		if totalPages < 1 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}
		offset := (page - 1) * defaultPageSize
		quotes, err = q.ListQuotesByChannelPaginated(ctx, dbgen.ListQuotesByChannelPaginatedParams{
			Channel: &selectedChannel,
			Limit:   defaultPageSize,
			Offset:  int64(offset),
		})
	} else {
		count, _ = q.CountQuotes(ctx)
		totalPages := int((count + defaultPageSize - 1) / defaultPageSize)
		if totalPages < 1 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}
		offset := (page - 1) * defaultPageSize
		quotes, err = q.ListQuotesPaginated(ctx, dbgen.ListQuotesPaginatedParams{
			Limit:  defaultPageSize,
			Offset: int64(offset),
		})
	}

	if err != nil {
		slog.Error("list quotes paginated", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	totalPages := int((count + defaultPageSize - 1) / defaultPageSize)
	if totalPages < 1 {
		totalPages = 1
	}

	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))

	data := pageData{
		Hostname:        s.Hostname,
		Now:             time.Now().Format(time.RFC3339),
		UserEmail:       userEmail,
		UserID:          userID,
		LoginURL:        loginURLForRequest(r),
		LogoutURL:       "/__exe.dev/logout",
		Quotes:          quotesToViews(quotes),
		QuoteCount:      count,
		Page:            page,
		PageSize:        defaultPageSize,
		TotalPages:      totalPages,
		HasPrev:         page > 1,
		HasNext:         page < totalPages,
		Channels:        channels,
		SelectedChannel: selectedChannel,
		IsPublicPage:    true,
		IsAuthenticated: userEmail != "",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "quotes_public.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

// HandleListAllQuotes godoc
// @Summary List all quotes
// @Description Returns all quotes in the database as JSON
// @Tags quotes
// @Produce json
// @Success 200 {array} QuoteResponse "List of all quotes"
// @Failure 500 {string} string "Internal server error"
// @Router /quotes [get]
func (s *Server) HandleListAllQuotes(w http.ResponseWriter, r *http.Request) {
	AddNightbotAttributes(r)

	q := dbgen.New(s.DB)
	quotes, err := q.ListAllQuotes(r.Context())
	if err != nil {
		slog.Error("list all quotes", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]QuoteResponse, len(quotes))
	for i, quote := range quotes {
		response[i] = QuoteResponse{
			ID:           quote.ID,
			Text:         quote.Text,
			Author:       quote.Author,
			Civilization: quote.Civilization,
			CreatedAt:    quote.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleGetQuote godoc
// @Summary Get a specific quote by ID
// @Description Returns a single quote by its database ID
// @Tags quotes
// @Produce plain
// @Produce json
// @Param id path int true "Quote ID"
// @Success 200 {object} QuoteResponse "Quote found"
// @Failure 400 {string} string "Invalid quote ID"
// @Failure 404 {string} string "Quote not found"
// @Router /quote/{id} [get]
func (s *Server) HandleGetQuote(w http.ResponseWriter, r *http.Request) {
	AddNightbotAttributes(r)
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid quote ID", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	dbCtx, span := StartDBSpan(ctx, "GetQuoteByID", attribute.Int64("quote.id", id))
	quote, err := q.GetQuoteByID(dbCtx, id)
	span.End()

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Quote not found", http.StatusNotFound)
			return
		}
		RecordError(trace.SpanFromContext(ctx), err)
		slog.Error("get quote by id", "error", err, "id", id)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := QuoteResponse{
		ID:           quote.ID,
		Text:         quote.Text,
		Author:       quote.Author,
		Civilization: quote.Civilization,
		OpponentCiv:  quote.OpponentCiv,
		CreatedAt:    quote.CreatedAt.Format(time.RFC3339),
	}

	WriteQuoteResponse(w, r, response)
}

// HandleMatchup godoc
// @Summary Get a matchup tip
// @Description Returns a random tip for a specific civilization matchup (your civ vs opponent civ).
// @Description Supports two query formats: standard (?civ=X&vs=Y) or Nightbot querystring (?X Y).
// @Tags matchups
// @Produce plain
// @Produce json
// @Param civ query string false "Your civilization shortname (e.g., hre)"
// @Param vs query string false "Opponent civilization shortname (e.g., french)"
// @Success 200 {object} QuoteResponse "Matchup tip found"
// @Success 200 {string} string "Matchup tip text (plain text default)"
// @Failure 400 {string} string "Usage: /api/matchup?civ=X&vs=Y"
// @Router /matchup [get]
func (s *Server) HandleMatchup(w http.ResponseWriter, r *http.Request) {
	AddNightbotAttributes(r)
	ctx := r.Context()

	q := dbgen.New(s.DB)
	playCiv := r.URL.Query().Get("civ")
	vsCiv := r.URL.Query().Get("vs")

	// Get channel from bot headers (Nightbot, Moobot) or query param
	var channel string
	if bc := GetBotChannel(r); bc != nil {
		channel = bc.Name
	}

	// Log incoming request for debugging
	slog.Info("matchup request", "rawQuery", r.URL.RawQuery, "fullURL", r.URL.String())

	// Support Nightbot querystring format: /api/matchup?hre french
	// The raw query will be "hre french" or "hre%20french"
	if playCiv == "" && vsCiv == "" {
		rawQuery := r.URL.RawQuery
		if rawQuery != "" {
			// URL decode and split by space
			decoded, _ := url.QueryUnescape(rawQuery)
			parts := strings.Fields(decoded)
			if len(parts) >= 2 {
				playCiv = parts[0]
				vsCiv = parts[1]
			}
		}
	}

	if playCiv == "" || vsCiv == "" {
		rootSpan := trace.SpanFromContext(ctx)
		rootSpan.AddEvent("invalid_request", trace.WithAttributes(
			attribute.String("reason", "missing_civ_params"),
			attribute.String("civ", playCiv),
			attribute.String("vs", vsCiv),
		))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, "Usage: /api/matchup?civ=X&vs=Y or /api/matchup?X Y")
		return
	}

	// Resolve shortnames
	dbCtx, span := StartDBSpan(ctx, "ResolveCivName", attribute.String("civ.input", playCiv))
	if resolved, err := q.ResolveCivName(dbCtx, dbgen.ResolveCivNameParams{
		Shortname: &playCiv,
		LOWER:     playCiv,
	}); err == nil {
		playCiv = resolved
		span.SetAttributes(attribute.String("civ.resolved", playCiv))
	}
	span.End()

	dbCtx, span = StartDBSpan(ctx, "ResolveCivName", attribute.String("civ.input", vsCiv))
	if resolved, err := q.ResolveCivName(dbCtx, dbgen.ResolveCivNameParams{
		Shortname: &vsCiv,
		LOWER:     vsCiv,
	}); err == nil {
		vsCiv = resolved
		span.SetAttributes(attribute.String("civ.resolved", vsCiv))
	}
	span.End()

	var quote dbgen.Quote
	var err error
	if channel != "" {
		dbCtx, span := StartDBSpan(ctx, "GetRandomMatchupQuote",
			attribute.String("civ", playCiv),
			attribute.String("vs", vsCiv),
			attribute.String("channel", channel))
		quote, err = q.GetRandomMatchupQuote(dbCtx, dbgen.GetRandomMatchupQuoteParams{
			Civilization: &playCiv,
			OpponentCiv:  &vsCiv,
			Channel:      &channel,
		})
		if err != nil && err != sql.ErrNoRows {
			RecordError(span, err)
		}
		span.End()
	} else {
		dbCtx, span := StartDBSpan(ctx, "GetRandomMatchupQuoteGlobal",
			attribute.String("civ", playCiv),
			attribute.String("vs", vsCiv))
		quote, err = q.GetRandomMatchupQuoteGlobal(dbCtx, dbgen.GetRandomMatchupQuoteGlobalParams{
			Civilization: &playCiv,
			OpponentCiv:  &vsCiv,
		})
		if err != nil && err != sql.ErrNoRows {
			RecordError(span, err)
		}
		span.End()
	}
	if err != nil {
		if err == sql.ErrNoRows {
			span := trace.SpanFromContext(ctx)
			span.AddEvent("no_results", trace.WithAttributes(
				attribute.String("query_type", "matchup"),
				attribute.String("civ", playCiv),
				attribute.String("vs", vsCiv),
			))
			// Return 200 so bots like Nightbot don't treat it as an error
			WriteNoResultsResponse(w, r, fmt.Sprintf("No tips for %s vs %s yet.", playCiv, vsCiv))
			return
		}
		// Record error on parent span too
		RecordError(trace.SpanFromContext(ctx), err)
		slog.Error("get matchup quote", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Record successful quote retrieval
	rootSpan := trace.SpanFromContext(ctx)
	rootSpan.AddEvent("quote_served", trace.WithAttributes(
		attribute.Int64("quote.id", quote.ID),
		attribute.String("query_type", "matchup"),
	))

	response := QuoteResponse{
		ID:           quote.ID,
		Text:         quote.Text,
		Author:       quote.Author,
		Civilization: quote.Civilization,
		OpponentCiv:  quote.OpponentCiv,
		CreatedAt:    quote.CreatedAt.Format(time.RFC3339),
	}
	WriteQuoteResponse(w, r, response)
}

// HandleRandomQuote godoc
// @Summary Get a random quote
// @Description Returns a random quote from the database. Supports filtering by civilization and channel.
// @Tags quotes
// @Produce plain
// @Produce json
// @Param civ query string false "Civilization shortname (e.g., hre, french, mongols)"
// @Param channel query string false "Channel name for channel-specific quotes"
// @Success 200 {object} QuoteResponse "Quote found (JSON when Accept: application/json)"
// @Success 200 {string} string "Quote text (plain text default)"
// @Header 200 {string} Content-Type "text/plain or application/json based on Accept header"
// @Router /quote [get]
func (s *Server) HandleRandomQuote(w http.ResponseWriter, r *http.Request) {
	AddNightbotAttributes(r)
	ctx := r.Context()

	q := dbgen.New(s.DB)
	civ := r.URL.Query().Get("civ")

	// Get channel from bot headers (Nightbot, Moobot) or query param
	var channel string
	if bc := GetBotChannel(r); bc != nil {
		channel = bc.Name
	}

	// Resolve shortname to full civ name
	if civ != "" {
		dbCtx, span := StartDBSpan(ctx, "ResolveCivName", attribute.String("civ.input", civ))
		if resolved, err := q.ResolveCivName(dbCtx, dbgen.ResolveCivNameParams{
			Shortname: &civ,
			LOWER:     civ,
		}); err == nil {
			civ = resolved
			span.SetAttributes(attribute.String("civ.resolved", civ))
		}
		span.End()
	}

	var quote dbgen.Quote
	var err error
	if civ != "" {
		if channel != "" {
			dbCtx, span := StartDBSpan(ctx, "GetRandomQuoteByCiv",
				attribute.String("civ", civ),
				attribute.String("channel", channel))
			quote, err = q.GetRandomQuoteByCiv(dbCtx, dbgen.GetRandomQuoteByCivParams{
				Civilization: &civ,
				Channel:      &channel,
			})
			if err != nil && err != sql.ErrNoRows {
				RecordError(span, err)
			}
			span.End()
		} else {
			dbCtx, span := StartDBSpan(ctx, "GetRandomQuoteByCivGlobal",
				attribute.String("civ", civ))
			quote, err = q.GetRandomQuoteByCivGlobal(dbCtx, &civ)
			if err != nil && err != sql.ErrNoRows {
				RecordError(span, err)
			}
			span.End()
		}
	} else {
		if channel != "" {
			dbCtx, span := StartDBSpan(ctx, "GetRandomQuote",
				attribute.String("channel", channel))
			quote, err = q.GetRandomQuote(dbCtx, &channel)
			if err != nil && err != sql.ErrNoRows {
				RecordError(span, err)
			}
			span.End()
		} else {
			dbCtx, span := StartDBSpan(ctx, "GetRandomQuoteGlobal")
			quote, err = q.GetRandomQuoteGlobal(dbCtx)
			if err != nil && err != sql.ErrNoRows {
				RecordError(span, err)
			}
			span.End()
		}
	}

	if err != nil {
		if err == sql.ErrNoRows {
			span := trace.SpanFromContext(ctx)
			span.AddEvent("no_results", trace.WithAttributes(
				attribute.String("query_type", "quote"),
				attribute.String("civ", civ),
			))
			// Return 200 so bots like Nightbot don't treat it as an error
			if civ != "" {
				WriteNoResultsResponse(w, r, fmt.Sprintf("No quotes available for %s.", civ))
			} else {
				WriteNoResultsResponse(w, r, "No quotes available.")
			}
			return
		}
		// Record error on parent span too
		RecordError(trace.SpanFromContext(ctx), err)
		slog.Error("get random quote", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Record successful quote retrieval
	rootSpan := trace.SpanFromContext(ctx)
	rootSpan.AddEvent("quote_served", trace.WithAttributes(
		attribute.Int64("quote.id", quote.ID),
		attribute.String("query_type", "quote"),
	))

	response := QuoteResponse{
		ID:           quote.ID,
		Text:         quote.Text,
		Author:       quote.Author,
		Civilization: quote.Civilization,
		CreatedAt:    quote.CreatedAt.Format(time.RFC3339),
	}
	WriteQuoteResponse(w, r, response)
}

func loginURLForRequest(r *http.Request) string {
	path := r.URL.RequestURI()
	v := url.Values{}
	v.Set("redirect", path)
	return "/__exe.dev/login?" + v.Encode()
}

func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

var templateFuncs = template.FuncMap{
	"add":      func(a, b int) int { return a + b },
	"subtract": func(a, b int) int { return a - b },
}

func (s *Server) loadTemplates() error {
	s.templates = make(map[string]*template.Template)
	templateFiles := []string{"index.html", "quotes.html", "quotes_public.html", "civs.html", "suggestions.html", "suggest.html", "admin_owners.html"}
	navPath := filepath.Join(s.TemplatesDir, "nav.html")
	for _, name := range templateFiles {
		path := filepath.Join(s.TemplatesDir, name)
		tmpl, err := template.New(name).Funcs(templateFuncs).ParseFiles(path, navPath)
		if err != nil {
			return fmt.Errorf("parse template %q: %w", name, err)
		}
		s.templates[name] = tmpl
	}
	slog.Info("templates loaded", "count", len(s.templates))
	return nil
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) error {
	tmpl, ok := s.templates[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("execute template %q: %w", name, err)
	}
	return nil
}

func (s *Server) setUpDatabase(dbPath string) error {
	wdb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	s.DB = wdb

	migrations, err := db.RunMigrations(wdb)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Create markers for each migration that was applied
	for _, m := range migrations {
		s.Markers.CreateMigrationMarker(m.Filename, m.StartTime, m.EndTime)
	}

	return nil
}

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.HandleRoot)
	mux.HandleFunc("GET /health", s.HandleHealth)
	mux.HandleFunc("GET /browse", s.HandleQuotesPublic)
	mux.HandleFunc("GET /suggest", s.HandleSuggestForm)
	mux.HandleFunc("GET /quotes", s.HandleQuotes)
	mux.HandleFunc("POST /quotes", s.HandleAddQuote)
	mux.HandleFunc("POST /quotes/bulk", s.HandleBulkQuotes)
	mux.HandleFunc("POST /quotes/{id}/edit", s.HandleEditQuote)
	mux.HandleFunc("POST /quotes/{id}/delete", s.HandleDeleteQuote)
	mux.HandleFunc("GET /civs", s.HandleCivs)
	mux.HandleFunc("POST /civs", s.HandleAddCiv)
	mux.HandleFunc("POST /civs/{id}/edit", s.HandleEditCiv)
	mux.HandleFunc("POST /civs/{id}/delete", s.HandleDeleteCiv)
	mux.HandleFunc("GET /suggestions", s.HandleListSuggestions)
	mux.HandleFunc("POST /suggestions/{id}/approve", s.HandleApproveSuggestion)
	mux.HandleFunc("POST /suggestions/{id}/reject", s.HandleRejectSuggestion)
	// Admin routes
	mux.HandleFunc("GET /admin/owners", s.HandleListChannelOwners)
	mux.HandleFunc("POST /admin/owners", s.HandleAddChannelOwner)
	mux.HandleFunc("POST /admin/owners/delete", s.HandleRemoveChannelOwner)
	mux.Handle("/static/", http.StripPrefix("/static/", StaticFileServer(s.StaticDir)))

	// API routes with rate limiting (including docs)
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/{$}", s.HandleAPIDocs)
	apiMux.HandleFunc("GET /api/openapi.json", s.HandleAPISpec)
	apiMux.HandleFunc("GET /api/quote", s.HandleRandomQuote)
	apiMux.HandleFunc("GET /api/quote/{id}", s.HandleGetQuote)
	apiMux.HandleFunc("GET /api/quotes", s.HandleListAllQuotes)
	apiMux.HandleFunc("GET /api/matchup", s.HandleMatchup)
	apiMux.HandleFunc("POST /api/suggestions", s.HandleSubmitSuggestion)
	apiMux.HandleFunc("GET /api/suggest", s.HandleBotSuggestion)
	mux.Handle("/api/", s.APILimiter.Middleware(apiMux))

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: otelhttp.NewHandler(SecurityHeaders(RequestLogger(Gzip(LimitRequestBody(mux)))), "quotes"),
	}

	slog.Info("starting server", "addr", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// SuggestionRequest is the JSON body for submitting a quote suggestion
type SuggestionRequest struct {
	Text         string  `json:"text"`
	Author       *string `json:"author,omitempty"`
	Civilization *string `json:"civilization,omitempty"`
	OpponentCiv  *string `json:"opponent_civ,omitempty"`
	Channel      string  `json:"channel"`
}

// SuggestionResponse is the JSON response for a suggestion
type SuggestionResponse struct {
	ID          int64   `json:"id"`
	Text        string  `json:"text"`
	Author      *string `json:"author,omitempty"`
	Civilization *string `json:"civilization,omitempty"`
	OpponentCiv *string `json:"opponent_civ,omitempty"`
	Channel     string  `json:"channel"`
	Status      string  `json:"status"`
	SubmittedAt string  `json:"submitted_at"`
}

// HandleSubmitSuggestion godoc
// @Summary Submit a quote suggestion
// @Description Submit a new quote for review. Rate limited to 5 suggestions per IP per hour.
// @Tags suggestions
// @Accept json
// @Produce json
// @Param suggestion body SuggestionRequest true "Quote suggestion"
// @Success 201 {object} map[string]string "Suggestion submitted successfully"
// @Failure 400 {string} string "Invalid request (missing fields or text too long)"
// @Failure 429 {string} string "Too many suggestions"
// @Failure 500 {string} string "Internal server error"
// @Router /suggestions [post]
func (s *Server) HandleSubmitSuggestion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get submitter info from auth headers (if logged in)
	submittedByUser := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	var submittedByUserPtr *string
	if submittedByUser != "" {
		submittedByUserPtr = &submittedByUser
	}

	// Get client IP for rate limiting and tracking
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
		// Strip port from RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
	}

	// Rate limit: max 5 suggestions per IP per hour
	q := dbgen.New(s.DB)
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	count, err := q.CountRecentSuggestionsByIP(ctx, dbgen.CountRecentSuggestionsByIPParams{
		SubmittedByIp: ip,
		SubmittedAt:   oneHourAgo,
	})
	if err != nil {
		slog.Error("count recent suggestions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if count >= 5 {
		RecordSecurityEvent(ctx, "suggestion_rate_limited",
			attribute.String("client.ip", ip),
			attribute.Int64("suggestion_count", count),
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Too many suggestions. Please try again later.", http.StatusTooManyRequests)
		return
	}

	// Parse request body
	var req SuggestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.Text) == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Channel) == "" {
		http.Error(w, "Channel is required", http.StatusBadRequest)
		return
	}

	// Limit text length
	if len(req.Text) > 500 {
		http.Error(w, "Text too long (max 500 characters)", http.StatusBadRequest)
		return
	}

	// Resolve civ shortnames if provided
	if req.Civilization != nil && *req.Civilization != "" {
		if resolved, err := q.ResolveCivName(ctx, dbgen.ResolveCivNameParams{
			Shortname: req.Civilization,
			LOWER:     strings.ToLower(*req.Civilization),
		}); err == nil {
			req.Civilization = &resolved
		}
	}
	if req.OpponentCiv != nil && *req.OpponentCiv != "" {
		if resolved, err := q.ResolveCivName(ctx, dbgen.ResolveCivNameParams{
			Shortname: req.OpponentCiv,
			LOWER:     strings.ToLower(*req.OpponentCiv),
		}); err == nil {
			req.OpponentCiv = &resolved
		}
	}

	// Create the suggestion
	now := time.Now()
	err = q.CreateSuggestion(ctx, dbgen.CreateSuggestionParams{
		Text:            req.Text,
		Author:          req.Author,
		Civilization:    req.Civilization,
		OpponentCiv:     req.OpponentCiv,
		Channel:         req.Channel,
		SubmittedByIp:   ip,
		SubmittedByUser: submittedByUserPtr,
		SubmittedAt:     now,
	})
	if err != nil {
		slog.Error("create suggestion", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	span := trace.SpanFromContext(ctx)
	span.AddEvent("suggestion_created", trace.WithAttributes(
		attribute.String("channel", req.Channel),
	))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Suggestion submitted for review",
		"channel": req.Channel,
	})
}

// HandleBotSuggestion godoc
// @Summary Submit a quote suggestion via GET (for chat bots)
// @Description Submit a quote suggestion using GET request. Designed for Nightbot/Moobot $(urlfetch) commands.
// @Description Channel is determined from bot headers (Nightbot-Channel, Moobot-Channel) or query param.
// @Tags suggestions
// @Produce plain
// @Param text query string true "Quote text to suggest"
// @Param channel query string false "Channel name (optional if bot headers present)"
// @Param author query string false "Quote author"
// @Param civ query string false "Civilization shortname"
// @Success 200 {string} string "Success message"
// @Failure 400 {string} string "Missing text or channel"
// @Failure 429 {string} string "Too many suggestions"
// @Router /suggest [get]
func (s *Server) HandleBotSuggestion(w http.ResponseWriter, r *http.Request) {
	AddBotAttributes(r)
	ctx := r.Context()

	// Get channel from bot headers or query param
	var channel string
	if bc := GetBotChannel(r); bc != nil {
		channel = bc.Name
	}
	if channel == "" {
		http.Error(w, "Could not determine channel. Make sure your bot sends channel headers.", http.StatusBadRequest)
		return
	}

	// Get submitter username from bot headers
	var submittedByUserPtr *string
	if botUser := GetBotUser(r); botUser != "" {
		submittedByUserPtr = &botUser
	}

	// Get quote text from query param
	text := strings.TrimSpace(r.URL.Query().Get("text"))
	if text == "" {
		http.Error(w, "Usage: !addquote <quote text>", http.StatusBadRequest)
		return
	}

	// Validate text length
	if len(text) < 3 {
		http.Error(w, "Quote too short (min 3 characters)", http.StatusBadRequest)
		return
	}
	if len(text) > 500 {
		http.Error(w, "Quote too long (max 500 characters)", http.StatusBadRequest)
		return
	}

	// Get client IP for rate limiting
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
	}

	// Rate limit: max 5 suggestions per channel per hour
	q := dbgen.New(s.DB)
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	count, err := q.CountRecentSuggestionsByChannel(ctx, dbgen.CountRecentSuggestionsByChannelParams{
		Channel:     channel,
		SubmittedAt: oneHourAgo,
	})
	if err != nil {
		slog.Error("count recent suggestions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if count >= 5 {
		RecordSecurityEvent(ctx, "suggestion_rate_limited",
			attribute.String("channel", channel),
			attribute.Int64("suggestion_count", count),
			attribute.String("path", r.URL.Path),
		)
		fmt.Fprint(w, "Too many suggestions for this channel. Try again later.")
		return
	}

	// Get optional author from query param
	var authorPtr *string
	if author := strings.TrimSpace(r.URL.Query().Get("author")); author != "" {
		authorPtr = &author
	}

	// Create the suggestion
	now := time.Now()
	err = q.CreateSuggestion(ctx, dbgen.CreateSuggestionParams{
		Text:            text,
		Author:          authorPtr,
		Civilization:    nil,
		OpponentCiv:     nil,
		Channel:         channel,
		SubmittedByIp:   ip,
		SubmittedByUser: submittedByUserPtr,
		SubmittedAt:     now,
	})
	if err != nil {
		slog.Error("create suggestion", "error", err)
		http.Error(w, "Failed to submit quote", http.StatusInternalServerError)
		return
	}

	span := trace.SpanFromContext(ctx)
	span.AddEvent("bot_suggestion_created", trace.WithAttributes(
		attribute.String("channel", channel),
		attribute.Int("text_length", len(text)),
	))

	slog.Info("bot suggestion created", "channel", channel, "text_length", len(text))
	fmt.Fprintf(w, "Quote submitted for review!")
}

func (s *Server) HandleListSuggestions(w http.ResponseWriter, r *http.Request) {
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userEmail == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	isAdmin := s.isAdmin(userEmail)
	ownedChannels, _ := s.getOwnedChannels(ctx, userEmail)

	// If not admin and not a channel owner, deny access
	if !isAdmin && len(ownedChannels) == 0 {
		RecordSecurityEvent(ctx, "permission_denied",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
			attribute.String("reason", "not_channel_owner"),
		)
		http.Error(w, "You don't have permission to review suggestions. Contact an admin to get access.", http.StatusForbidden)
		return
	}

	q := dbgen.New(s.DB)
	var suggestions []dbgen.QuoteSuggestion
	var err error

	if isAdmin {
		// Admins see all suggestions
		suggestions, err = q.ListPendingSuggestions(ctx)
	} else {
		// Channel owners see only their channel's suggestions
		suggestions, err = q.ListPendingSuggestionsByChannel(ctx, ownedChannels[0])
	}
	if err != nil {
		slog.Error("list suggestions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Hostname        string
		UserEmail       string
		LogoutURL       string
		Suggestions     []dbgen.QuoteSuggestion
		IsAdmin         bool
		IsAuthenticated bool
		IsPublicPage    bool
		OwnedChannels   []string
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		Suggestions:     suggestions,
		IsAdmin:         isAdmin,
		IsAuthenticated: true,
		IsPublicPage:    false,
		OwnedChannels:   ownedChannels,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["suggestions.html"].Execute(w, data); err != nil {
		slog.Error("execute template", "error", err)
	}
}

func (s *Server) HandleApproveSuggestion(w http.ResponseWriter, r *http.Request) {
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	ctx := r.Context()

	if userEmail == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)

	// Get the suggestion
	suggestion, err := q.GetSuggestionByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Suggestion not found", http.StatusNotFound)
			return
		}
		slog.Error("get suggestion", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check permission: must be admin or own this channel
	if !s.canManageChannel(ctx, userEmail, suggestion.Channel) {
		RecordSecurityEvent(ctx, "permission_denied",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
			attribute.String("resource", "suggestion"),
			attribute.Int64("suggestion.id", id),
			attribute.String("channel", suggestion.Channel),
			attribute.String("reason", "not_channel_owner"),
		)
		http.Error(w, "You don't have permission to approve suggestions for this channel", http.StatusForbidden)
		return
	}

	// Create the quote from the suggestion
	now := time.Now()
	err = q.CreateQuote(ctx, dbgen.CreateQuoteParams{
		UserID:         userID,
		CreatedByEmail: &userEmail,
		Text:           suggestion.Text,
		Author:         suggestion.Author,
		Civilization:   suggestion.Civilization,
		OpponentCiv:    suggestion.OpponentCiv,
		Channel:        &suggestion.Channel,
		RequestedBy:    suggestion.SubmittedByUser,
		CreatedAt:      now,
	})
	if err != nil {
		slog.Error("create quote from suggestion", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Mark suggestion as approved
	err = q.ApproveSuggestion(ctx, dbgen.ApproveSuggestionParams{
		ReviewedBy: &userEmail,
		ReviewedAt: &now,
		ID:         id,
	})
	if err != nil {
		slog.Error("approve suggestion", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/suggestions", http.StatusSeeOther)
}

func (s *Server) HandleRejectSuggestion(w http.ResponseWriter, r *http.Request) {
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userEmail == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)

	// Get the suggestion to check permission
	suggestion, err := q.GetSuggestionByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Suggestion not found", http.StatusNotFound)
			return
		}
		slog.Error("get suggestion", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check permission: must be admin or own this channel
	if !s.canManageChannel(ctx, userEmail, suggestion.Channel) {
		RecordSecurityEvent(ctx, "permission_denied",
			attribute.String("user.email", userEmail),
			attribute.String("path", r.URL.Path),
			attribute.String("resource", "suggestion"),
			attribute.Int64("suggestion.id", id),
			attribute.String("channel", suggestion.Channel),
			attribute.String("reason", "not_channel_owner"),
		)
		http.Error(w, "You don't have permission to reject suggestions for this channel", http.StatusForbidden)
		return
	}

	now := time.Now()

	err = q.RejectSuggestion(ctx, dbgen.RejectSuggestionParams{
		ReviewedBy: &userEmail,
		ReviewedAt: &now,
		ID:         id,
	})
	if err != nil {
		slog.Error("reject suggestion", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/suggestions", http.StatusSeeOther)
}

// Authorization helpers

func (s *Server) isAdmin(email string) bool {
	return s.AdminEmails[strings.ToLower(strings.TrimSpace(email))]
}

func (s *Server) getOwnedChannels(ctx context.Context, email string) ([]string, error) {
	q := dbgen.New(s.DB)
	return q.GetChannelsByOwner(ctx, strings.ToLower(strings.TrimSpace(email)))
}

func (s *Server) canManageChannel(ctx context.Context, email, channel string) bool {
	if s.isAdmin(email) {
		return true
	}
	channels, err := s.getOwnedChannels(ctx, email)
	if err != nil {
		return false
	}
	for _, ch := range channels {
		if strings.EqualFold(ch, channel) {
			return true
		}
	}
	return false
}

// Admin handlers for channel owner management

func (s *Server) HandleListChannelOwners(w http.ResponseWriter, r *http.Request) {
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userEmail == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
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

	owners, err := q.ListAllChannelOwners(ctx)
	if err != nil {
		slog.Error("list channel owners", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	channels, err := q.ListChannels(ctx)
	if err != nil {
		slog.Error("list channels", "error", err)
	}

	data := struct {
		Hostname        string
		UserEmail       string
		LogoutURL       string
		Owners          []dbgen.ChannelOwner
		Channels        []*string
		Success         string
		Error           string
		IsAdmin         bool
		IsAuthenticated bool
		IsPublicPage    bool
	}{
		Hostname:        s.Hostname,
		UserEmail:       userEmail,
		LogoutURL:       "/__exe.dev/logout",
		Owners:          owners,
		Channels:        channels,
		Success:         r.URL.Query().Get("success"),
		Error:           r.URL.Query().Get("error"),
		IsAdmin:         true,
		IsAuthenticated: true,
		IsPublicPage:    false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["admin_owners.html"].Execute(w, data); err != nil {
		slog.Error("execute template", "error", err)
	}
}

func (s *Server) HandleAddChannelOwner(w http.ResponseWriter, r *http.Request) {
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userEmail == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	channel := strings.TrimSpace(strings.ToLower(r.FormValue("channel")))
	ownerEmail := strings.TrimSpace(strings.ToLower(r.FormValue("email")))

	if channel == "" || ownerEmail == "" {
		http.Redirect(w, r, "/admin/owners?error=Channel+and+email+are+required", http.StatusSeeOther)
		return
	}
	q := dbgen.New(s.DB)

	err := q.AddChannelOwner(ctx, dbgen.AddChannelOwnerParams{
		Channel:   channel,
		UserEmail: ownerEmail,
		InvitedBy: userEmail,
	})
	if err != nil {
		slog.Error("add channel owner", "error", err)
		http.Redirect(w, r, "/admin/owners?error=Failed+to+add+owner", http.StatusSeeOther)
		return
	}

	// Create marker for config change
	s.Markers.CreateConfigChangeMarker(fmt.Sprintf("Channel owner added: %s for #%s", ownerEmail, channel))

	http.Redirect(w, r, "/admin/owners?success=Owner+added", http.StatusSeeOther)
}

func (s *Server) HandleRemoveChannelOwner(w http.ResponseWriter, r *http.Request) {
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	ctx := r.Context()

	if userEmail == "" {
		RecordSecurityEvent(ctx, "auth_required",
			attribute.String("path", r.URL.Path),
		)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	channel := strings.TrimSpace(r.FormValue("channel"))
	ownerEmail := strings.TrimSpace(r.FormValue("email"))

	if channel == "" || ownerEmail == "" {
		http.Redirect(w, r, "/admin/owners?error=Channel+and+email+are+required", http.StatusSeeOther)
		return
	}
	q := dbgen.New(s.DB)

	err := q.RemoveChannelOwner(ctx, dbgen.RemoveChannelOwnerParams{
		Channel:   channel,
		UserEmail: ownerEmail,
	})
	if err != nil {
		slog.Error("remove channel owner", "error", err)
		http.Redirect(w, r, "/admin/owners?error=Failed+to+remove+owner", http.StatusSeeOther)
		return
	}

	// Create marker for config change
	s.Markers.CreateConfigChangeMarker(fmt.Sprintf("Channel owner removed: %s from #%s", ownerEmail, channel))

	http.Redirect(w, r, "/admin/owners?success=Owner+removed", http.StatusSeeOther)
}

func (s *Server) HandleSuggestForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := dbgen.New(s.DB)

	civs, err := q.ListCivs(ctx)
	if err != nil {
		slog.Error("list civilizations", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Hostname        string
		Civs            []dbgen.Civilization
		IsPublicPage    bool
		IsAuthenticated bool
		IsAdmin         bool
		LoginURL        string
		LogoutURL       string
	}{
		Hostname:        s.Hostname,
		Civs:            civs,
		IsPublicPage:    true,
		IsAuthenticated: false,
		IsAdmin:         false,
		LoginURL:        loginURLForRequest(r),
		LogoutURL:       "/__exe.dev/logout",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates["suggest.html"].Execute(w, data); err != nil {
		slog.Error("execute template", "error", err)
	}
}
