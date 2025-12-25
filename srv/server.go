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

	if text == "" {
		http.Redirect(w, r, "/quotes?error=Quote+text+is+required", http.StatusSeeOther)
		return
	}

	q := dbgen.New(s.DB)
	var authorPtr *string
	if author != "" {
		authorPtr = &author
	}

	err := q.CreateQuote(r.Context(), dbgen.CreateQuoteParams{
		UserID:    userID,
		Text:      text,
		Author:    authorPtr,
		CreatedAt: time.Now(),
	})
	if err != nil {
		slog.Error("create quote", "error", err)
		http.Redirect(w, r, "/quotes?error=Failed+to+save+quote", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/quotes?success=Quote+added!", http.StatusSeeOther)
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
	ID        int64   `json:"id"`
	Text      string  `json:"text"`
	Author    *string `json:"author,omitempty"`
	CreatedAt string  `json:"created_at"`
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
			ID:        quote.ID,
			Text:      quote.Text,
			Author:    quote.Author,
			CreatedAt: quote.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) HandleRandomQuote(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	quote, err := q.GetRandomQuote(r.Context())
	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, "No quotes available.")
			return
		}
		slog.Error("get random quote", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if quote.Author != nil && *quote.Author != "" {
		fmt.Fprintf(w, "%s\n— %s\n", quote.Text, *quote.Author)
	} else {
		fmt.Fprintln(w, quote.Text)
	}
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
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}
