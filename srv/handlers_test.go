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

func TestHandleAddQuote(t *testing.T) {
	t.Run("redirects to login when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader("text=test+quote"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "login") {
			t.Errorf("expected redirect to login, got: %s", loc)
		}
	})

	t.Run("returns 403 when user cannot manage channel", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader("text=test+quote&channel=somechannel"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "notowner@test.com")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("admin can add quote to any channel", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader("text=Admin+added+quote&channel=anychannel"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "success") {
			t.Errorf("expected redirect with success, got: %s", loc)
		}
	})

	t.Run("channel owner can add quote to their channel", func(t *testing.T) {
		server := testServer(t)
		// Add channel owner
		q := dbgen.New(server.DB)
		err := q.AddChannelOwner(context.Background(), dbgen.AddChannelOwnerParams{
			Channel:   "mychannel",
			UserEmail: "owner@test.com",
		})
		if err != nil {
			t.Fatalf("failed to add channel owner: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader("text=Owner+quote&channel=mychannel"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "owner123")
		req.Header.Set("X-ExeDev-Email", "owner@test.com")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "success") {
			t.Errorf("expected redirect with success, got: %s", loc)
		}
	})

	t.Run("non-admin cannot add global quote (no channel)", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader("text=Global+quote"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "anyuser")
		req.Header.Set("X-ExeDev-Email", "anyone@test.com")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		// Non-admins cannot add global quotes (empty channel)
		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403 forbidden, got %d", w.Code)
		}
	})

	t.Run("admin can add global quote (no channel)", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader("text=Global+quote+by+admin"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "success") {
			t.Errorf("expected redirect with success, got: %s", loc)
		}
	})

	t.Run("validates empty text", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader("text="))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "error") {
			t.Errorf("expected redirect with error, got: %s", loc)
		}
	})

	t.Run("stores all fields correctly", func(t *testing.T) {
		server := testServer(t)
		formData := "text=Full+quote&author=TestAuthor&civilization=English&opponent_civ=French"
		req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader(formData))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleAddQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}

		// Verify quote was stored
		q := dbgen.New(server.DB)
		quotes, err := q.ListAllQuotes(context.Background())
		if err != nil {
			t.Fatalf("failed to list quotes: %v", err)
		}
		if len(quotes) == 0 {
			t.Fatal("expected at least one quote")
		}
		quote := quotes[0]
		if quote.Text != "Full quote" {
			t.Errorf("expected text 'Full quote', got %s", quote.Text)
		}
		if quote.Author == nil || *quote.Author != "TestAuthor" {
			t.Errorf("expected author 'TestAuthor', got %v", quote.Author)
		}
		if quote.Civilization == nil || *quote.Civilization != "English" {
			t.Errorf("expected civilization 'English', got %v", quote.Civilization)
		}
		if quote.OpponentCiv == nil || *quote.OpponentCiv != "French" {
			t.Errorf("expected opponent_civ 'French', got %v", quote.OpponentCiv)
		}
	})
}
