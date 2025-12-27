package srv

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
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
	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	APILimiter   *RateLimiter
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
}

type QuoteView struct {
	ID           int64
	Text         string
	Author       string
	Civilization string
	OpponentCiv  string
	Channel      string
	CreatedBy    string
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

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
		// Rate limit: 30 requests per minute per IP, burst of 10
		APILimiter: NewRateLimiter(30, time.Minute, 10),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	if err := srv.loadTemplates(); err != nil {
		return nil, err
	}
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
	}
	return views
}

func (s *Server) HandleQuotes(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userID == "" {
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	quotes, err := q.ListAllQuotes(r.Context())
	if err != nil {
		slog.Error("list quotes", "error", err)
	}

	data := pageData{
		Hostname:  s.Hostname,
		Now:       time.Now().Format(time.RFC3339),
		UserEmail: userEmail,
		UserID:    userID,
		LoginURL:  loginURLForRequest(r),
		LogoutURL: "/__exe.dev/logout",
		Quotes:    quotesToViews(quotes),
		Success:   r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "quotes.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAddQuote(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userID == "" {
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

	if userID == "" {
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
		Hostname:  s.Hostname,
		Now:       time.Now().Format(time.RFC3339),
		UserEmail: userEmail,
		UserID:    userID,
		LoginURL:  loginURLForRequest(r),
		LogoutURL: "/__exe.dev/logout",
		Civs:      civsWithCount,
		Success:   r.URL.Query().Get("success"),
		Error:     r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "civs.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAddCiv(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	if userID == "" {
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
	err = q.DeleteQuoteByID(r.Context(), id)
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

	// Parse pagination params
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	count, _ := q.CountQuotes(r.Context())
	totalPages := int((count + defaultPageSize - 1) / defaultPageSize)
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * defaultPageSize
	quotes, err := q.ListQuotesPaginated(r.Context(), dbgen.ListQuotesPaginatedParams{
		Limit:  defaultPageSize,
		Offset: int64(offset),
	})
	if err != nil {
		slog.Error("list quotes paginated", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))

	data := pageData{
		Hostname:   s.Hostname,
		Now:        time.Now().Format(time.RFC3339),
		UserEmail:  userEmail,
		UserID:     userID,
		LoginURL:   loginURLForRequest(r),
		LogoutURL:  "/__exe.dev/logout",
		Quotes:     quotesToViews(quotes),
		QuoteCount: count,
		Page:       page,
		PageSize:   defaultPageSize,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "quotes_public.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) HandleMatchup(w http.ResponseWriter, r *http.Request) {
	AddNightbotAttributes(r)
	ctx := r.Context()

	q := dbgen.New(s.DB)
	playCiv := r.URL.Query().Get("civ")
	vsCiv := r.URL.Query().Get("vs")

	// Get channel from Nightbot header for multi-streamer support
	var channel string
	if nb := ParseNightbotChannel(r.Header.Get("Nightbot-Channel")); nb != nil {
		channel = nb.Name
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
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			// Return 200 so bots like Nightbot don't treat it as an error
			fmt.Fprintf(w, "No tips for %s vs %s yet.\n", playCiv, vsCiv)
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

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var parts []string
	parts = append(parts, quote.Text)
	if quote.Author != nil && *quote.Author != "" {
		parts = append(parts, fmt.Sprintf("— %s", *quote.Author))
	}
	fmt.Fprintln(w, strings.Join(parts, " "))
}

func (s *Server) HandleRandomQuote(w http.ResponseWriter, r *http.Request) {
	AddNightbotAttributes(r)
	ctx := r.Context()

	q := dbgen.New(s.DB)
	civ := r.URL.Query().Get("civ")

	// Get channel from Nightbot header for multi-streamer support
	var channel string
	if nb := ParseNightbotChannel(r.Header.Get("Nightbot-Channel")); nb != nil {
		channel = nb.Name
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
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			// Return 200 so bots like Nightbot don't treat it as an error
			if civ != "" {
				fmt.Fprintf(w, "No quotes available for %s.\n", civ)
			} else {
				fmt.Fprintln(w, "No quotes available.")
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
	span := trace.SpanFromContext(ctx)
	span.AddEvent("quote_served", trace.WithAttributes(
		attribute.Int64("quote.id", quote.ID),
		attribute.String("query_type", "quote"),
	))

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var parts []string
	parts = append(parts, quote.Text)
	if quote.Author != nil && *quote.Author != "" {
		parts = append(parts, fmt.Sprintf("— %s", *quote.Author))
	}
	if quote.Civilization != nil && *quote.Civilization != "" {
		parts = append(parts, fmt.Sprintf("[%s]", *quote.Civilization))
	}
	fmt.Fprintln(w, strings.Join(parts, " "))
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
	templateFiles := []string{"index.html", "quotes.html", "quotes_public.html", "civs.html"}
	for _, name := range templateFiles {
		path := filepath.Join(s.TemplatesDir, name)
		tmpl, err := template.New(name).Funcs(templateFuncs).ParseFiles(path)
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
	if err := db.RunMigrations(wdb); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.HandleRoot)
	mux.HandleFunc("GET /health", s.HandleHealth)
	mux.HandleFunc("GET /browse", s.HandleQuotesPublic)
	mux.HandleFunc("GET /quotes", s.HandleQuotes)
	mux.HandleFunc("POST /quotes", s.HandleAddQuote)
	mux.HandleFunc("POST /quotes/bulk", s.HandleBulkQuotes)
	mux.HandleFunc("POST /quotes/{id}/edit", s.HandleEditQuote)
	mux.HandleFunc("POST /quotes/{id}/delete", s.HandleDeleteQuote)
	mux.HandleFunc("GET /civs", s.HandleCivs)
	mux.HandleFunc("POST /civs", s.HandleAddCiv)
	mux.HandleFunc("POST /civs/{id}/edit", s.HandleEditCiv)
	mux.HandleFunc("POST /civs/{id}/delete", s.HandleDeleteCiv)
	mux.Handle("/static/", http.StripPrefix("/static/", StaticFileServer(s.StaticDir)))

	// API routes with rate limiting
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/quote", s.HandleRandomQuote)
	apiMux.HandleFunc("GET /api/quote/{id}", s.HandleGetQuote)
	apiMux.HandleFunc("GET /api/quotes", s.HandleListAllQuotes)
	apiMux.HandleFunc("GET /api/matchup", s.HandleMatchup)
	mux.Handle("/api/", s.APILimiter.Middleware(apiMux))

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: otelhttp.NewHandler(RequestLogger(Gzip(LimitRequestBody(mux))), "quotes"),
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
