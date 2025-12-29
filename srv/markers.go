package srv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// Marker types for grouping in Honeycomb UI
const (
	MarkerTypeDeploy        = "deploy"
	MarkerTypeMigration     = "migration"
	MarkerTypeConfigChange  = "config-change"
	MarkerTypeBulkOperation = "bulk-operation"
)

// Build-time variables (set via -ldflags)
var (
	Version   = "dev"
	CommitSHA = "unknown"
)

// Marker represents a Honeycomb marker
type Marker struct {
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time,omitempty"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
}

// MarkerClient handles communication with Honeycomb Markers API
type MarkerClient struct {
	apiKey  string
	dataset string
	client  *http.Client
}

// NewMarkerClient creates a new marker client from environment variables.
// Returns nil if HONEYCOMB_API_KEY is not set.
func NewMarkerClient() *MarkerClient {
	apiKey := os.Getenv("HONEYCOMB_API_KEY")
	if apiKey == "" {
		return nil
	}

	dataset := os.Getenv("OTEL_SERVICE_NAME")
	if dataset == "" {
		dataset = "quoteqt"
	}

	return &MarkerClient{
		apiKey:  apiKey,
		dataset: dataset,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateMarker sends a marker to Honeycomb.
// Logs errors but doesn't return them - markers are best-effort.
func (mc *MarkerClient) CreateMarker(m Marker) {
	if mc == nil {
		return
	}

	if m.StartTime == 0 {
		m.StartTime = time.Now().Unix()
	}

	body, err := json.Marshal(m)
	if err != nil {
		slog.Error("marshal marker", "error", err)
		return
	}

	url := fmt.Sprintf("https://api.honeycomb.io/1/markers/%s", mc.dataset)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		slog.Error("create marker request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Honeycomb-Team", mc.apiKey)

	resp, err := mc.client.Do(req)
	if err != nil {
		slog.Error("send marker", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		slog.Error("marker API error", "status", resp.StatusCode, "type", m.Type, "message", m.Message)
		return
	}

	slog.Info("marker created", "type", m.Type, "message", m.Message)
}

// CreateDeployMarker creates a deploy marker with version and commit info
func (mc *MarkerClient) CreateDeployMarker() {
	if mc == nil {
		return
	}

	message := fmt.Sprintf("Deploy %s", Version)
	if CommitSHA != "unknown" && CommitSHA != "" {
		message = fmt.Sprintf("Deploy %s (%s)", Version, CommitSHA[:minInt(7, len(CommitSHA))])
	}

	m := Marker{
		Message: message,
		Type:    MarkerTypeDeploy,
	}

	// Add GitHub commit URL if we have a commit SHA
	if CommitSHA != "unknown" && CommitSHA != "" {
		m.URL = fmt.Sprintf("https://github.com/webframp/quoteqt/commit/%s", CommitSHA)
	}

	mc.CreateMarker(m)
}

// CreateMigrationMarker creates a marker for a database migration
func (mc *MarkerClient) CreateMigrationMarker(filename string, startTime, endTime time.Time) {
	if mc == nil {
		return
	}

	mc.CreateMarker(Marker{
		StartTime: startTime.Unix(),
		EndTime:   endTime.Unix(),
		Message:   fmt.Sprintf("Migration: %s", filename),
		Type:      MarkerTypeMigration,
	})
}

// CreateConfigChangeMarker creates a marker for configuration changes
func (mc *MarkerClient) CreateConfigChangeMarker(message string) {
	if mc == nil {
		return
	}

	mc.CreateMarker(Marker{
		Message: message,
		Type:    MarkerTypeConfigChange,
	})
}

// CreateBulkOperationMarker creates a marker for bulk operations
func (mc *MarkerClient) CreateBulkOperationMarker(operation string, count int) {
	if mc == nil {
		return
	}

	mc.CreateMarker(Marker{
		Message: fmt.Sprintf("%s: %d items", operation, count),
		Type:    MarkerTypeBulkOperation,
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
