//go:build integration

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

var baseURL = "http://localhost:8000"

func init() {
	if u := os.Getenv("API_BASE_URL"); u != "" {
		baseURL = u
	}
}

// TestAPIQuoteRandom tests GET /api/quote returns plain text
func TestAPIQuoteRandom(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/quote")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 or 404 (no quotes)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
	}

	// Content-Type should be plain text
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %s", ct)
	}
}

// TestAPIQuoteWithCiv tests GET /api/quote?civ=X with shortname
func TestAPIQuoteWithCiv(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/quote?civ=hre")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %s", ct)
	}
}

// TestAPIQuotesJSON tests GET /api/quotes returns JSON array
func TestAPIQuotesJSON(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/quotes")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %s", ct)
	}

	// Should parse as JSON array
	var quotes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&quotes); err != nil {
		t.Errorf("failed to parse JSON: %v", err)
	}

	// Verify expected fields exist in response (if quotes exist)
	if len(quotes) > 0 {
		q := quotes[0]
		requiredFields := []string{"id", "text", "created_at"}
		for _, field := range requiredFields {
			if _, ok := q[field]; !ok {
				t.Errorf("missing required field: %s", field)
			}
		}
	}
}

// TestAPIMatchup tests GET /api/matchup?civ=X&vs=Y
func TestAPIMatchup(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/matchup?civ=hre&vs=french")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %s", ct)
	}
}

// TestAPIMatchupMissingParams tests GET /api/matchup with missing params
func TestAPIMatchupMissingParams(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/api/matchup"},
		{"missing vs", "/api/matchup?civ=hre"},
		{"missing civ", "/api/matchup?vs=french"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(baseURL + tt.url)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

// TestPublicPagesAccessible tests that public pages return 200
func TestPublicPagesAccessible(t *testing.T) {
	pages := []string{"/", "/browse"}

	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			resp, err := http.Get(baseURL + page)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Errorf("expected text/html content-type, got %s", ct)
			}
		})
	}
}

// TestAuthenticatedPagesRedirect tests that auth pages redirect without auth
func TestAuthenticatedPagesRedirect(t *testing.T) {
	pages := []string{"/quotes", "/civs"}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			resp, err := client.Get(baseURL + page)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusSeeOther {
				t.Errorf("expected 303 redirect, got %d", resp.StatusCode)
			}

			loc := resp.Header.Get("Location")
			if !strings.Contains(loc, "/__exe.dev/login") {
				t.Errorf("expected redirect to login, got %s", loc)
			}
		})
	}
}

// TestCivShortnames tests that common civ shortnames resolve correctly
func TestCivShortnames(t *testing.T) {
	shortnames := []string{"hre", "delhi", "rus", "french", "english", "mongols", "zhuxi"}

	for _, sn := range shortnames {
		t.Run(sn, func(t *testing.T) {
			resp, err := http.Get(baseURL + "/api/quote?civ=" + url.QueryEscape(sn))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should not return 500 (shortname should resolve)
			if resp.StatusCode == http.StatusInternalServerError {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("server error for shortname %s: %s", sn, body)
			}
		})
	}
}
