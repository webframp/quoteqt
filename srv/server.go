package srv

import (
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

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
}

type pageData struct {
	Hostname   string
	Now        string
	UserEmail  string
	UserID     string
	LoginURL   string
	LogoutURL  string
	Quotes     []dbgen.Quote
	Error      string
	Success    string
	QuoteCount int64
	Civs       []CivWithCount
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
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	return srv, nil
}

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	q := dbgen.New(s.DB)
	count, _ := q.CountQuotes(r.Context())

	data := pageData{
		Hostname:   s.Hostname,
		Now:        time.Now().Format(time.RFC3339),
		UserEmail:  userEmail,
		UserID:     userID,
		LoginURL:   loginURLForRequest(r),
		LogoutURL:  "/__exe.dev/logout",
		QuoteCount: count,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "index.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleQuotes(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))

	if userID == "" {
		http.Redirect(w, r, loginURLForRequest(r), http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	quotes, err := q.ListQuotesByUser(r.Context(), userID)
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
		Quotes:    quotes,
		Success:   r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "quotes.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAddQuote(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))

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

	if text == "" {
		http.Redirect(w, r, "/quotes?error=Quote+text+is+required", http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	var authorPtr, civPtr *string
	if author != "" {
		authorPtr = &author
	}
	if civ != "" {
		civPtr = &civ
	}

	err := q.CreateQuote(r.Context(), dbgen.CreateQuoteParams{
		UserID:       userID,
		Text:         text,
		Author:       authorPtr,
		Civilization: civPtr,
		CreatedAt:    time.Now(),
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
	civs, err := q.ListCivs(r.Context())
	if err != nil {
		slog.Error("list civs", "error", err)
	}

	civsWithCount := make([]CivWithCount, len(civs))
	for i, civ := range civs {
		count, _ := q.CountQuotesByCiv(r.Context(), &civ.Name)
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
			QuoteCount: count,
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

	if name == "" {
		http.Redirect(w, r, "/civs?error=Name+is+required", http.StatusSeeOther)
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

	if name == "" {
		http.Redirect(w, r, "/civs?error=Name+is+required", http.StatusSeeOther)
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
	err = q.DeleteCiv(r.Context(), id)
	if err != nil {
		slog.Error("delete civ", "error", err)
	}

	http.Redirect(w, r, "/civs?success=Civilization+deleted", http.StatusSeeOther)
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
	err = q.DeleteQuote(r.Context(), dbgen.DeleteQuoteParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		slog.Error("delete quote", "error", err)
	}

	http.Redirect(w, r, "/quotes?success=Quote+deleted", http.StatusSeeOther)
}

type QuoteResponse struct {
	ID           int64   `json:"id"`
	Text         string  `json:"text"`
	Author       *string `json:"author,omitempty"`
	Civilization *string `json:"civilization,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

func (s *Server) HandleListAllQuotes(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) HandleRandomQuote(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	civ := r.URL.Query().Get("civ")

	// Resolve shortname to full civ name
	if civ != "" {
		if resolved, err := q.ResolveCivName(r.Context(), dbgen.ResolveCivNameParams{
			Shortname: &civ,
			LOWER:     civ,
		}); err == nil {
			civ = resolved
		}
	}

	var quote dbgen.Quote
	var err error
	if civ != "" {
		quote, err = q.GetRandomQuoteByCiv(r.Context(), &civ)
	} else {
		quote, err = q.GetRandomQuote(r.Context())
	}

	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			if civ != "" {
				fmt.Fprintf(w, "No quotes available for %s.\n", civ)
			} else {
				fmt.Fprintln(w, "No quotes available.")
			}
			return
		}
		slog.Error("get random quote", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

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

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) error {
	path := filepath.Join(s.TemplatesDir, name)
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return fmt.Errorf("parse template %q: %w", name, err)
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
	mux.HandleFunc("GET /quotes", s.HandleQuotes)
	mux.HandleFunc("POST /quotes", s.HandleAddQuote)
	mux.HandleFunc("POST /quotes/{id}/delete", s.HandleDeleteQuote)
	mux.HandleFunc("GET /api/quote", s.HandleRandomQuote)
	mux.HandleFunc("GET /api/quotes", s.HandleListAllQuotes)
	mux.HandleFunc("GET /civs", s.HandleCivs)
	mux.HandleFunc("POST /civs", s.HandleAddCiv)
	mux.HandleFunc("POST /civs/{id}/edit", s.HandleEditCiv)
	mux.HandleFunc("POST /civs/{id}/delete", s.HandleDeleteCiv)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}
