package srv

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webframp/quoteqt/db/dbgen"
)

func TestServerSetupAndHandlers(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_server.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname", []string{"admin@test.com"})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	t.Run("root endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		server.HandleRoot(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "AoE4") {
			t.Errorf("expected page to contain AoE4, got body: %s", body[:200])
		}
	})

	t.Run("browse endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/browse", nil)
		w := httptest.NewRecorder()

		server.HandleQuotesPublic(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("quotes endpoint requires auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/quotes", nil)
		w := httptest.NewRecorder()

		server.HandleQuotes(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected redirect 303, got %d", w.Code)
		}
	})

	t.Run("random quote endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		// Should return 200 or 404 (no quotes in test db)
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}

		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/plain") {
			t.Errorf("expected text/plain, got %s", ct)
		}
	})

	t.Run("matchup endpoint requires both params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/matchup?civ=hre", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestQuotesToViews(t *testing.T) {
	author := "Test Author"
	civ := "English"
	opp := "French"

	input := []dbgen.Quote{
		{ID: 1, Text: "Test quote", Author: &author, Civilization: &civ, OpponentCiv: &opp},
		{ID: 2, Text: "No author", Author: nil, Civilization: nil, OpponentCiv: nil},
	}

	result := quotesToViews(input)

	if len(result) != 2 {
		t.Fatalf("expected 2 views, got %d", len(result))
	}

	if result[0].Author != "Test Author" {
		t.Errorf("expected author 'Test Author', got '%s'", result[0].Author)
	}
	if result[0].Civilization != "English" {
		t.Errorf("expected civ 'English', got '%s'", result[0].Civilization)
	}
	if result[0].OpponentCiv != "French" {
		t.Errorf("expected opponent 'French', got '%s'", result[0].OpponentCiv)
	}

	if result[1].Author != "" {
		t.Errorf("expected empty author, got '%s'", result[1].Author)
	}
}

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"1 minute ago", now.Add(-1 * time.Minute), "1 minute ago"},
		{"5 minutes ago", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"59 minutes ago", now.Add(-59 * time.Minute), "59 minutes ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1 hour ago"},
		{"2 hours ago", now.Add(-2 * time.Hour), "2 hours ago"},
		{"23 hours ago", now.Add(-23 * time.Hour), "23 hours ago"},
		{"yesterday", now.Add(-25 * time.Hour), "yesterday"},
		{"2 days ago", now.Add(-50 * time.Hour), "2 days ago"},
		{"6 days ago", now.Add(-6 * 24 * time.Hour), "6 days ago"},
		{"old date", now.Add(-30 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour).Format("Jan 2, 2006")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimeAgo(tt.time)
			if result != tt.expected {
				t.Errorf("formatTimeAgo(%v) = %q, want %q", tt.time, result, tt.expected)
			}
		})
	}
}
