package srv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webframp/quoteqt/db/dbgen"
)

// testServer creates a test server with a fresh database
func testServer(t *testing.T) *Server {
	t.Helper()
	tempDB := filepath.Join(t.TempDir(), "test.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname", []string{"admin@test.com"})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return server
}

// addTestQuote adds a quote to the test database
func addTestQuote(t *testing.T, s *Server, text string, civ, channel *string) {
	t.Helper()
	q := dbgen.New(s.DB)
	err := q.CreateQuote(context.Background(), dbgen.CreateQuoteParams{
		Text:         text,
		Civilization: civ,
		Channel:      channel,
	})
	if err != nil {
		t.Fatalf("failed to create quote: %v", err)
	}
}

// addTestCiv adds a civilization to the test database (ignores if already exists)
func addTestCiv(t *testing.T, s *Server, name, shortname string) {
	t.Helper()
	q := dbgen.New(s.DB)
	_ = q.CreateCiv(context.Background(), dbgen.CreateCivParams{
		Name:      name,
		Shortname: &shortname,
	})
	// Ignore error - civ may already exist from migrations
}

func TestHandleRandomQuote(t *testing.T) {
	t.Run("returns 200 with message when no quotes", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "No quotes available") {
			t.Errorf("expected 'No quotes available' message, got: %s", w.Body.String())
		}
	})

	t.Run("returns quote when available", func(t *testing.T) {
		server := testServer(t)
		addTestQuote(t, server, "Test quote text", nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Test quote text") {
			t.Errorf("expected quote text in response, got: %s", w.Body.String())
		}
	})

	t.Run("filters by civ full name", func(t *testing.T) {
		server := testServer(t)
		hre := "Holy Roman Empire"
		french := "French"
		addTestQuote(t, server, "HRE quote", &hre, nil)
		addTestQuote(t, server, "French quote", &french, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quote?civ=Holy+Roman+Empire", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "HRE quote") {
			t.Errorf("expected HRE quote, got: %s", body)
		}
	})

	t.Run("filters by civ shortname", func(t *testing.T) {
		server := testServer(t)
		addTestCiv(t, server, "Holy Roman Empire", "hre")
		hre := "Holy Roman Empire"
		addTestQuote(t, server, "HRE shortname quote", &hre, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quote?civ=hre", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "HRE shortname quote") {
			t.Errorf("expected HRE quote, got: %s", body)
		}
	})

	t.Run("returns 200 with message for unknown civ", func(t *testing.T) {
		server := testServer(t)
		addTestQuote(t, server, "Some quote", nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quote?civ=unknownciv", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "No quotes available") {
			t.Errorf("expected no quotes message, got: %s", w.Body.String())
		}
	})

	t.Run("filters by channel via Nightbot header", func(t *testing.T) {
		server := testServer(t)
		channel := "testchannel"
		addTestQuote(t, server, "Channel specific quote", nil, &channel)
		addTestQuote(t, server, "Global quote", nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
		req.Header.Set("Nightbot-Channel", "name=testchannel&displayName=TestChannel&provider=twitch&providerId=123")
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		// Should return either channel-specific or global quote
		body := w.Body.String()
		if !strings.Contains(body, "quote") {
			t.Errorf("expected a quote, got: %s", body)
		}
	})

	t.Run("filters by channel via query param", func(t *testing.T) {
		server := testServer(t)
		channel := "mychannel"
		addTestQuote(t, server, "My channel quote", nil, &channel)

		req := httptest.NewRequest(http.MethodGet, "/api/quote?channel=mychannel", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("returns JSON when Accept header requests it", func(t *testing.T) {
		server := testServer(t)
		addTestQuote(t, server, "JSON test quote", nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json, got %s", ct)
		}
		if !strings.Contains(w.Body.String(), `"text"`) {
			t.Errorf("expected JSON with text field, got: %s", w.Body.String())
		}
	})

	t.Run("returns plain text by default", func(t *testing.T) {
		server := testServer(t)
		addTestQuote(t, server, "Plain text quote", nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
		w := httptest.NewRecorder()

		server.HandleRandomQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/plain") {
			t.Errorf("expected text/plain, got %s", ct)
		}
	})
}

// addTestMatchupQuote adds a matchup quote to the test database
func addTestMatchupQuote(t *testing.T, s *Server, text string, civ, opponentCiv string, channel *string) {
	t.Helper()
	q := dbgen.New(s.DB)
	err := q.CreateQuote(context.Background(), dbgen.CreateQuoteParams{
		Text:         text,
		Civilization: &civ,
		OpponentCiv:  &opponentCiv,
		Channel:      channel,
	})
	if err != nil {
		t.Fatalf("failed to create matchup quote: %v", err)
	}
}

func TestHandleMatchup(t *testing.T) {
	t.Run("returns 400 when missing civ param", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/matchup?vs=french", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Usage:") {
			t.Errorf("expected usage message, got: %s", w.Body.String())
		}
	})

	t.Run("returns 400 when missing vs param", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/matchup?civ=hre", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 400 when no params", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/matchup", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 200 with message when no matchup tips", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/matchup?civ=hre&vs=french", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "No tips") {
			t.Errorf("expected 'No tips' message, got: %s", w.Body.String())
		}
	})

	t.Run("returns matchup tip when available", func(t *testing.T) {
		server := testServer(t)
		addTestMatchupQuote(t, server, "HRE vs French tip", "Holy Roman Empire", "French", nil)

		req := httptest.NewRequest(http.MethodGet, "/api/matchup?civ=Holy+Roman+Empire&vs=French", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "HRE vs French tip") {
			t.Errorf("expected matchup tip, got: %s", w.Body.String())
		}
	})

	t.Run("resolves civ shortnames", func(t *testing.T) {
		server := testServer(t)
		// Civs already exist from migrations
		addTestMatchupQuote(t, server, "Shortname matchup tip", "Holy Roman Empire", "French", nil)

		req := httptest.NewRequest(http.MethodGet, "/api/matchup?civ=hre&vs=french", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Shortname matchup tip") {
			t.Errorf("expected matchup tip, got: %s", w.Body.String())
		}
	})

	t.Run("supports Nightbot querystring format", func(t *testing.T) {
		server := testServer(t)
		addTestMatchupQuote(t, server, "Nightbot format tip", "Holy Roman Empire", "French", nil)

		// Nightbot sends: /api/matchup?hre french (space-separated)
		req := httptest.NewRequest(http.MethodGet, "/api/matchup?hre%20french", nil)
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Nightbot format tip") {
			t.Errorf("expected matchup tip, got: %s", w.Body.String())
		}
	})

	t.Run("filters by channel", func(t *testing.T) {
		server := testServer(t)
		channel := "teststreamer"
		addTestMatchupQuote(t, server, "Channel specific tip", "Holy Roman Empire", "French", &channel)

		req := httptest.NewRequest(http.MethodGet, "/api/matchup?civ=hre&vs=french", nil)
		req.Header.Set("Nightbot-Channel", "name=teststreamer&displayName=TestStreamer&provider=twitch&providerId=123")
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Channel specific tip") {
			t.Errorf("expected channel tip, got: %s", w.Body.String())
		}
	})

	t.Run("returns JSON when Accept header requests it", func(t *testing.T) {
		server := testServer(t)
		addTestMatchupQuote(t, server, "JSON matchup tip", "Holy Roman Empire", "French", nil)

		req := httptest.NewRequest(http.MethodGet, "/api/matchup?civ=hre&vs=french", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		server.HandleMatchup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json, got %s", ct)
		}
		if !strings.Contains(w.Body.String(), `"opponent_civ"`) {
			t.Errorf("expected JSON with opponent_civ field, got: %s", w.Body.String())
		}
	})
}
