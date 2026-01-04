package srv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestHandleDeleteQuote(t *testing.T) {
	t.Run("returns 401 when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes/1/delete", nil)
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		server.HandleDeleteQuote(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes/abc/delete", nil)
		req.SetPathValue("id", "abc")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleDeleteQuote(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 404 for non-existent quote", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes/99999/delete", nil)
		req.SetPathValue("id", "99999")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleDeleteQuote(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("returns 403 when user cannot manage channel", func(t *testing.T) {
		server := testServer(t)
		channel := "somechannel"
		addTestQuote(t, server, "Quote to delete", nil, &channel)

		// Get the quote ID
		q := dbgen.New(server.DB)
		quotes, _ := q.ListAllQuotes(context.Background())
		quoteID := quotes[0].ID

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotes/%d/delete", quoteID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", quoteID))
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "notowner@test.com")
		w := httptest.NewRecorder()

		server.HandleDeleteQuote(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("admin can delete any quote", func(t *testing.T) {
		server := testServer(t)
		channel := "anychannel"
		addTestQuote(t, server, "Admin delete test", nil, &channel)

		q := dbgen.New(server.DB)
		quotes, _ := q.ListAllQuotes(context.Background())
		quoteID := quotes[0].ID

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotes/%d/delete", quoteID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", quoteID))
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleDeleteQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}

		// Verify quote was deleted
		_, err := q.GetQuoteByID(context.Background(), quoteID)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected quote to be deleted, got err: %v", err)
		}
	})

	t.Run("channel owner can delete quote from their channel", func(t *testing.T) {
		server := testServer(t)
		channel := "ownerchannel"
		addTestQuote(t, server, "Owner delete test", nil, &channel)

		// Add channel owner
		q := dbgen.New(server.DB)
		_ = q.AddChannelOwner(context.Background(), dbgen.AddChannelOwnerParams{
			Channel:   channel,
			UserEmail: "owner@test.com",
		})

		quotes, _ := q.ListAllQuotes(context.Background())
		quoteID := quotes[0].ID

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotes/%d/delete", quoteID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", quoteID))
		req.Header.Set("X-ExeDev-UserID", "owner123")
		req.Header.Set("X-ExeDev-Email", "owner@test.com")
		w := httptest.NewRecorder()

		server.HandleDeleteQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}
	})
}

func TestHandleSubmitSuggestion(t *testing.T) {
	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/suggestions", strings.NewReader("not json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.HandleSubmitSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 400 when text is empty", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/suggestions", strings.NewReader(`{"text":"","channel":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.HandleSubmitSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Text is required") {
			t.Errorf("expected 'Text is required', got: %s", w.Body.String())
		}
	})

	t.Run("returns 400 when channel is empty", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/suggestions", strings.NewReader(`{"text":"test quote","channel":""}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.HandleSubmitSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Channel is required") {
			t.Errorf("expected 'Channel is required', got: %s", w.Body.String())
		}
	})

	t.Run("returns 400 when text too long", func(t *testing.T) {
		server := testServer(t)
		longText := strings.Repeat("a", 501)
		body := fmt.Sprintf(`{"text":"%s","channel":"test"}`, longText)
		req := httptest.NewRequest(http.MethodPost, "/api/suggestions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.HandleSubmitSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "too long") {
			t.Errorf("expected 'too long' error, got: %s", w.Body.String())
		}
	})

	t.Run("creates suggestion successfully", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/suggestions", strings.NewReader(`{"text":"Great quote!","channel":"testchannel"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.HandleSubmitSuggestion(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Suggestion submitted") {
			t.Errorf("expected success message, got: %s", w.Body.String())
		}

		// Verify suggestion was created
		q := dbgen.New(server.DB)
		suggestions, err := q.ListPendingSuggestions(context.Background())
		if err != nil {
			t.Fatalf("failed to list suggestions: %v", err)
		}
		if len(suggestions) != 1 {
			t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
		}
		if suggestions[0].Text != "Great quote!" {
			t.Errorf("expected text 'Great quote!', got %s", suggestions[0].Text)
		}
	})

	t.Run("returns JSON response", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/suggestions", strings.NewReader(`{"text":"JSON test","channel":"ch"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.HandleSubmitSuggestion(w, req)

		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json, got %s", ct)
		}
	})

	t.Run("tracks submitter email when authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/api/suggestions", strings.NewReader(`{"text":"Auth quote","channel":"ch"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-ExeDev-Email", "submitter@test.com")
		w := httptest.NewRecorder()

		server.HandleSubmitSuggestion(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", w.Code)
		}

		// Verify submitter was recorded
		q := dbgen.New(server.DB)
		suggestions, _ := q.ListPendingSuggestions(context.Background())
		if len(suggestions) == 0 {
			t.Fatal("expected suggestion")
		}
		if suggestions[0].SubmittedByUser == nil || *suggestions[0].SubmittedByUser != "submitter@test.com" {
			t.Errorf("expected submitter email, got %v", suggestions[0].SubmittedByUser)
		}
	})
}

// addTestSuggestion adds a suggestion to the test database
func addTestSuggestion(t *testing.T, s *Server, text, channel string) int64 {
	t.Helper()
	q := dbgen.New(s.DB)
	err := q.CreateSuggestion(context.Background(), dbgen.CreateSuggestionParams{
		Text:          text,
		Channel:       channel,
		SubmittedByIp: "127.0.0.1",
		SubmittedAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to create suggestion: %v", err)
	}
	// Get the ID
	suggestions, _ := q.ListPendingSuggestions(context.Background())
	for _, s := range suggestions {
		if s.Text == text {
			return s.ID
		}
	}
	t.Fatal("suggestion not found")
	return 0
}

func TestHandleApproveSuggestion(t *testing.T) {
	t.Run("returns 401 when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/suggestions/1/approve", nil)
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		server.HandleApproveSuggestion(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/suggestions/abc/approve", nil)
		req.SetPathValue("id", "abc")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleApproveSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 404 for non-existent suggestion", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/suggestions/99999/approve", nil)
		req.SetPathValue("id", "99999")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleApproveSuggestion(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("returns 403 when user cannot manage channel", func(t *testing.T) {
		server := testServer(t)
		sugID := addTestSuggestion(t, server, "Suggestion to approve", "somechannel")

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/suggestions/%d/approve", sugID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", sugID))
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "notowner@test.com")
		w := httptest.NewRecorder()

		server.HandleApproveSuggestion(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("admin can approve suggestion and creates quote", func(t *testing.T) {
		server := testServer(t)
		sugID := addTestSuggestion(t, server, "Admin approved suggestion", "testchannel")

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/suggestions/%d/approve", sugID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", sugID))
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleApproveSuggestion(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}

		// Verify quote was created
		q := dbgen.New(server.DB)
		quotes, _ := q.ListAllQuotes(context.Background())
		found := false
		for _, quote := range quotes {
			if quote.Text == "Admin approved suggestion" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected quote to be created from suggestion")
		}
	})

	t.Run("channel owner can approve suggestion for their channel", func(t *testing.T) {
		server := testServer(t)
		channel := "ownerchannel"
		sugID := addTestSuggestion(t, server, "Owner approved", channel)

		// Add channel owner
		q := dbgen.New(server.DB)
		_ = q.AddChannelOwner(context.Background(), dbgen.AddChannelOwnerParams{
			Channel:   channel,
			UserEmail: "owner@test.com",
		})

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/suggestions/%d/approve", sugID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", sugID))
		req.Header.Set("X-ExeDev-UserID", "owner123")
		req.Header.Set("X-ExeDev-Email", "owner@test.com")
		w := httptest.NewRecorder()

		server.HandleApproveSuggestion(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", w.Code)
		}
	})
}

func TestHandleBotSuggestion(t *testing.T) {
	t.Run("returns 400 when no channel header", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/suggest?text=test+quote", nil)
		w := httptest.NewRecorder()

		server.HandleBotSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "channel") {
			t.Errorf("expected channel error, got: %s", w.Body.String())
		}
	})

	t.Run("returns 400 when no text", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/suggest", nil)
		req.Header.Set("Nightbot-Channel", "name=testchannel&displayName=Test&provider=twitch&providerId=123")
		w := httptest.NewRecorder()

		server.HandleBotSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Usage") {
			t.Errorf("expected usage message, got: %s", w.Body.String())
		}
	})

	t.Run("returns 400 when text too short", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/suggest?text=ab", nil)
		req.Header.Set("Nightbot-Channel", "name=testchannel&displayName=Test&provider=twitch&providerId=123")
		w := httptest.NewRecorder()

		server.HandleBotSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "too short") {
			t.Errorf("expected 'too short', got: %s", w.Body.String())
		}
	})

	t.Run("creates suggestion with Nightbot header", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/suggest?text=Bot+suggested+quote", nil)
		req.Header.Set("Nightbot-Channel", "name=botchannel&displayName=BotChannel&provider=twitch&providerId=123")
		w := httptest.NewRecorder()

		server.HandleBotSuggestion(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "submitted") {
			t.Errorf("expected success message, got: %s", w.Body.String())
		}

		// Verify suggestion was created with correct channel
		q := dbgen.New(server.DB)
		suggestions, _ := q.ListPendingSuggestionsByChannel(context.Background(), "botchannel")
		if len(suggestions) != 1 {
			t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
		}
		if suggestions[0].Text != "Bot suggested quote" {
			t.Errorf("expected 'Bot suggested quote', got %s", suggestions[0].Text)
		}
	})

	t.Run("creates suggestion with channel query param", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/suggest?text=Query+param+quote&channel=querychannel", nil)
		w := httptest.NewRecorder()

		server.HandleBotSuggestion(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

func TestHandleGetQuote(t *testing.T) {
	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/quote/abc", nil)
		req.SetPathValue("id", "abc")
		w := httptest.NewRecorder()

		server.HandleGetQuote(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 404 for non-existent quote", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/quote/99999", nil)
		req.SetPathValue("id", "99999")
		w := httptest.NewRecorder()

		server.HandleGetQuote(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("returns quote by ID", func(t *testing.T) {
		server := testServer(t)
		addTestQuote(t, server, "Quote by ID test", nil, nil)

		q := dbgen.New(server.DB)
		quotes, _ := q.ListAllQuotes(context.Background())
		quoteID := quotes[0].ID

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quote/%d", quoteID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", quoteID))
		w := httptest.NewRecorder()

		server.HandleGetQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Quote by ID test") {
			t.Errorf("expected quote text, got: %s", w.Body.String())
		}
	})

	t.Run("returns JSON when Accept header requests it", func(t *testing.T) {
		server := testServer(t)
		addTestQuote(t, server, "JSON ID test", nil, nil)

		q := dbgen.New(server.DB)
		quotes, _ := q.ListAllQuotes(context.Background())
		quoteID := quotes[0].ID

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/quote/%d", quoteID), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", quoteID))
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		server.HandleGetQuote(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json, got %s", ct)
		}
	})
}

func TestHandleEditQuote(t *testing.T) {
	t.Run("redirects to login when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes/1/edit", strings.NewReader("text=edited"))
		req.SetPathValue("id", "1")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.HandleEditQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "login") {
			t.Errorf("expected redirect to login, got: %s", loc)
		}
	})

	t.Run("returns 404 for non-existent quote", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/quotes/99999/edit", strings.NewReader("text=edited"))
		req.SetPathValue("id", "99999")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleEditQuote(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("admin can edit any quote", func(t *testing.T) {
		server := testServer(t)
		channel := "editchannel"
		addTestQuote(t, server, "Original text", nil, &channel)

		q := dbgen.New(server.DB)
		quotes, _ := q.ListAllQuotes(context.Background())
		quoteID := quotes[0].ID

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotes/%d/edit", quoteID), 
			strings.NewReader("text=Edited+text&channel=editchannel"))
		req.SetPathValue("id", fmt.Sprintf("%d", quoteID))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleEditQuote(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303, got %d", w.Code)
		}

		// Verify quote was updated
		updated, _ := q.GetQuoteByID(context.Background(), quoteID)
		if updated.Text != "Edited text" {
			t.Errorf("expected 'Edited text', got %s", updated.Text)
		}
	})

	t.Run("returns 403 when user cannot manage channel", func(t *testing.T) {
		server := testServer(t)
		channel := "otherchannel"
		addTestQuote(t, server, "Protected quote", nil, &channel)

		q := dbgen.New(server.DB)
		quotes, _ := q.ListAllQuotes(context.Background())
		quoteID := quotes[0].ID

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotes/%d/edit", quoteID), 
			strings.NewReader("text=Hacked"))
		req.SetPathValue("id", fmt.Sprintf("%d", quoteID))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "hacker@test.com")
		w := httptest.NewRecorder()

		server.HandleEditQuote(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})
}

func TestHandleListAllQuotes(t *testing.T) {
	t.Run("returns empty array when no quotes", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/quotes", nil)
		w := httptest.NewRecorder()

		server.HandleListAllQuotes(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if w.Body.String() != "[]\n" {
			t.Errorf("expected empty array, got: %s", w.Body.String())
		}
	})

	t.Run("returns JSON array of quotes", func(t *testing.T) {
		server := testServer(t)
		addTestQuote(t, server, "Quote 1", nil, nil)
		addTestQuote(t, server, "Quote 2", nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/quotes", nil)
		w := httptest.NewRecorder()

		server.HandleListAllQuotes(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json, got %s", ct)
		}
		if !strings.Contains(w.Body.String(), "Quote 1") || !strings.Contains(w.Body.String(), "Quote 2") {
			t.Errorf("expected both quotes, got: %s", w.Body.String())
		}
	})
}

func TestHandleListSuggestions(t *testing.T) {
	t.Run("redirects when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/suggestions", nil)
		w := httptest.NewRecorder()

		server.HandleListSuggestions(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303, got %d", w.Code)
		}
	})

	t.Run("returns 403 for non-admin non-owner", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodGet, "/suggestions", nil)
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "nobody@test.com")
		w := httptest.NewRecorder()

		server.HandleListSuggestions(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("admin can list all suggestions", func(t *testing.T) {
		server := testServer(t)
		addTestSuggestion(t, server, "Test suggestion", "testchannel")

		req := httptest.NewRequest(http.MethodGet, "/suggestions", nil)
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleListSuggestions(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Test suggestion") {
			t.Errorf("expected suggestion in response")
		}
	})

	t.Run("channel owner can list their channel suggestions", func(t *testing.T) {
		server := testServer(t)
		q := dbgen.New(server.DB)
		_ = q.AddChannelOwner(context.Background(), dbgen.AddChannelOwnerParams{
			Channel:   "ownedchannel",
			UserEmail: "owner@test.com",
			InvitedBy: "admin@test.com",
		})
		addTestSuggestion(t, server, "Owned channel suggestion", "ownedchannel")

		req := httptest.NewRequest(http.MethodGet, "/suggestions", nil)
		req.Header.Set("X-ExeDev-UserID", "owner123")
		req.Header.Set("X-ExeDev-Email", "owner@test.com")
		w := httptest.NewRecorder()

		server.HandleListSuggestions(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

func TestHandleRejectSuggestion(t *testing.T) {
	t.Run("returns 401 when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/suggestions/1/reject", nil)
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()

		server.HandleRejectSuggestion(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/suggestions/abc/reject", nil)
		req.SetPathValue("id", "abc")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleRejectSuggestion(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 404 for non-existent suggestion", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/suggestions/99999/reject", nil)
		req.SetPathValue("id", "99999")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleRejectSuggestion(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("returns 403 when user cannot manage channel", func(t *testing.T) {
		server := testServer(t)
		id := addTestSuggestion(t, server, "Protected suggestion", "otherchannel")

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/suggestions/%d/reject", id), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", id))
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "nobody@test.com")
		w := httptest.NewRecorder()

		server.HandleRejectSuggestion(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("admin can reject any suggestion", func(t *testing.T) {
		server := testServer(t)
		id := addTestSuggestion(t, server, "To be rejected", "anychannel")

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/suggestions/%d/reject", id), nil)
		req.SetPathValue("id", fmt.Sprintf("%d", id))
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleRejectSuggestion(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303, got %d", w.Code)
		}

		// Verify suggestion was rejected
		q := dbgen.New(server.DB)
		suggestion, _ := q.GetSuggestionByID(context.Background(), id)
		if suggestion.Status != "rejected" {
			t.Errorf("expected rejected status, got %s", suggestion.Status)
		}
	})
}

func TestHandleAddChannelOwner(t *testing.T) {
	t.Run("returns 401 when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/admin/owners", strings.NewReader("channel=test&email=user@test.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.HandleAddChannelOwner(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns 403 for non-admin", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/admin/owners", strings.NewReader("channel=test&email=user@test.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "user@test.com")
		w := httptest.NewRecorder()

		server.HandleAddChannelOwner(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("redirects with error when channel or email missing", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/admin/owners", strings.NewReader("channel=test"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleAddChannelOwner(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("expected error in redirect, got %s", loc)
		}
	})

	t.Run("admin can add channel owner", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/admin/owners", strings.NewReader("channel=newchannel&email=newowner@test.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleAddChannelOwner(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303, got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "success=") {
			t.Errorf("expected success in redirect, got %s", loc)
		}

		// Verify owner was added
		q := dbgen.New(server.DB)
		channels, _ := q.GetChannelsByOwner(context.Background(), "newowner@test.com")
		if len(channels) != 1 || channels[0] != "newchannel" {
			t.Errorf("expected newchannel in owned channels, got %v", channels)
		}
	})
}

func TestHandleRemoveChannelOwner(t *testing.T) {
	t.Run("returns 401 when not authenticated", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/admin/owners/remove", strings.NewReader("channel=test&email=user@test.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		server.HandleRemoveChannelOwner(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns 403 for non-admin", func(t *testing.T) {
		server := testServer(t)
		req := httptest.NewRequest(http.MethodPost, "/admin/owners/remove", strings.NewReader("channel=test&email=user@test.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "user@test.com")
		w := httptest.NewRecorder()

		server.HandleRemoveChannelOwner(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("admin can remove channel owner", func(t *testing.T) {
		server := testServer(t)
		q := dbgen.New(server.DB)

		// First add an owner
		_ = q.AddChannelOwner(context.Background(), dbgen.AddChannelOwnerParams{
			Channel:   "removechannel",
			UserEmail: "toremove@test.com",
			InvitedBy: "admin@test.com",
		})

		req := httptest.NewRequest(http.MethodPost, "/admin/owners/remove", strings.NewReader("channel=removechannel&email=toremove@test.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-ExeDev-UserID", "admin123")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")
		w := httptest.NewRecorder()

		server.HandleRemoveChannelOwner(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("expected 303, got %d", w.Code)
		}

		// Verify owner was removed
		channels, _ := q.GetChannelsByOwner(context.Background(), "toremove@test.com")
		if len(channels) != 0 {
			t.Errorf("expected no channels, got %v", channels)
		}
	})
}
