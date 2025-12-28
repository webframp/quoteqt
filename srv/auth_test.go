package srv

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"srv.exe.dev/db/dbgen"
)

func setupTestServer(t *testing.T, adminEmails []string) *Server {
	t.Helper()
	tempDB := filepath.Join(t.TempDir(), "test_auth.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname", adminEmails)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return server
}

func TestIsAdmin_MatchesConfiguredEmails(t *testing.T) {
	server := setupTestServer(t, []string{"admin@example.com", "superuser@test.com"})

	tests := []struct {
		email string
		want  bool
	}{
		{"admin@example.com", true},
		{"superuser@test.com", true},
		{"notadmin@example.com", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := server.isAdmin(tt.email); got != tt.want {
				t.Errorf("isAdmin(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsAdmin_CaseInsensitive(t *testing.T) {
	server := setupTestServer(t, []string{"Admin@Example.COM"})

	tests := []struct {
		email string
		want  bool
	}{
		{"admin@example.com", true},
		{"ADMIN@EXAMPLE.COM", true},
		{"Admin@Example.COM", true},
		{"aDmIn@eXaMpLe.CoM", true},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := server.isAdmin(tt.email); got != tt.want {
				t.Errorf("isAdmin(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsAdmin_TrimsWhitespace(t *testing.T) {
	server := setupTestServer(t, []string{"admin@example.com"})

	tests := []struct {
		email string
		want  bool
	}{
		{" admin@example.com", true},
		{"admin@example.com ", true},
		{"  admin@example.com  ", true},
		{"\tadmin@example.com\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := server.isAdmin(tt.email); got != tt.want {
				t.Errorf("isAdmin(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsAdmin_NoAdminsConfigured(t *testing.T) {
	server := setupTestServer(t, []string{})

	if server.isAdmin("anyone@example.com") {
		t.Error("should return false when no admins configured")
	}
}

func TestCanManageChannel_AdminCanManageAny(t *testing.T) {
	server := setupTestServer(t, []string{"admin@example.com"})
	ctx := context.Background()

	// Admin should be able to manage any channel
	channels := []string{"beastyqt", "randomchannel", "anychannel"}
	for _, ch := range channels {
		if !server.canManageChannel(ctx, "admin@example.com", ch) {
			t.Errorf("admin should be able to manage channel %q", ch)
		}
	}
}

func TestCanManageChannel_NonOwnerCannotManage(t *testing.T) {
	server := setupTestServer(t, []string{"admin@example.com"})
	ctx := context.Background()

	// Non-admin, non-owner should not be able to manage
	if server.canManageChannel(ctx, "random@example.com", "somechannel") {
		t.Error("non-owner should not be able to manage channel")
	}
}

func TestCanManageChannel_OwnerCanManageOwnChannel(t *testing.T) {
	server := setupTestServer(t, []string{"admin@example.com"})
	ctx := context.Background()

	// Add a channel owner
	q := dbgen.New(server.DB)
	err := q.AddChannelOwner(ctx, dbgen.AddChannelOwnerParams{
		Channel:   "beastyqt",
		UserEmail: "streamer@example.com",
	})
	if err != nil {
		t.Fatalf("failed to add channel owner: %v", err)
	}

	// Owner should be able to manage their channel
	if !server.canManageChannel(ctx, "streamer@example.com", "beastyqt") {
		t.Error("channel owner should be able to manage their channel")
	}

	// Owner should NOT be able to manage other channels
	if server.canManageChannel(ctx, "streamer@example.com", "otherchannel") {
		t.Error("channel owner should not be able to manage other channels")
	}
}

func TestCanManageChannel_OwnerCaseInsensitive(t *testing.T) {
	server := setupTestServer(t, []string{})
	ctx := context.Background()

	// Add a channel owner with lowercase
	q := dbgen.New(server.DB)
	err := q.AddChannelOwner(ctx, dbgen.AddChannelOwnerParams{
		Channel:   "beastyqt",
		UserEmail: "streamer@example.com",
	})
	if err != nil {
		t.Fatalf("failed to add channel owner: %v", err)
	}

	// Should match regardless of channel case
	tests := []struct {
		channel string
		want    bool
	}{
		{"beastyqt", true},
		{"BEASTYQT", true},
		{"BeastyQT", true},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			if got := server.canManageChannel(ctx, "streamer@example.com", tt.channel); got != tt.want {
				t.Errorf("canManageChannel(%q) = %v, want %v", tt.channel, got, tt.want)
			}
		})
	}
}

func TestCanManageChannel_MultipleChannelOwner(t *testing.T) {
	server := setupTestServer(t, []string{})
	ctx := context.Background()

	// Add owner for multiple channels
	q := dbgen.New(server.DB)
	for _, ch := range []string{"channel1", "channel2", "channel3"} {
		err := q.AddChannelOwner(ctx, dbgen.AddChannelOwnerParams{
			Channel:   ch,
			UserEmail: "multiowner@example.com",
		})
		if err != nil {
			t.Fatalf("failed to add channel owner: %v", err)
		}
	}

	// Should be able to manage all owned channels
	for _, ch := range []string{"channel1", "channel2", "channel3"} {
		if !server.canManageChannel(ctx, "multiowner@example.com", ch) {
			t.Errorf("should be able to manage owned channel %q", ch)
		}
	}

	// Should NOT be able to manage unowned channel
	if server.canManageChannel(ctx, "multiowner@example.com", "channel4") {
		t.Error("should not be able to manage unowned channel")
	}
}

func TestGetOwnedChannels_ReturnsCorrectChannels(t *testing.T) {
	server := setupTestServer(t, []string{})
	ctx := context.Background()

	// Add owner for multiple channels
	q := dbgen.New(server.DB)
	expected := []string{"alpha", "beta", "gamma"}
	for _, ch := range expected {
		err := q.AddChannelOwner(ctx, dbgen.AddChannelOwnerParams{
			Channel:   ch,
			UserEmail: "owner@example.com",
		})
		if err != nil {
			t.Fatalf("failed to add channel owner: %v", err)
		}
	}

	channels, err := server.getOwnedChannels(ctx, "owner@example.com")
	if err != nil {
		t.Fatalf("getOwnedChannels failed: %v", err)
	}

	if len(channels) != len(expected) {
		t.Errorf("expected %d channels, got %d", len(expected), len(channels))
	}

	// Check all expected channels are present
	channelSet := make(map[string]bool)
	for _, ch := range channels {
		channelSet[ch] = true
	}
	for _, ch := range expected {
		if !channelSet[ch] {
			t.Errorf("expected channel %q not found", ch)
		}
	}
}

func TestGetOwnedChannels_EmptyForNonOwner(t *testing.T) {
	server := setupTestServer(t, []string{})
	ctx := context.Background()

	channels, err := server.getOwnedChannels(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("getOwnedChannels failed: %v", err)
	}

	if len(channels) != 0 {
		t.Errorf("expected 0 channels for non-owner, got %d", len(channels))
	}
}

func TestGetOwnedChannels_EmailNormalization(t *testing.T) {
	server := setupTestServer(t, []string{})
	ctx := context.Background()

	// Add owner with lowercase email
	q := dbgen.New(server.DB)
	err := q.AddChannelOwner(ctx, dbgen.AddChannelOwnerParams{
		Channel:   "testchannel",
		UserEmail: "owner@example.com",
	})
	if err != nil {
		t.Fatalf("failed to add channel owner: %v", err)
	}

	// Should find channels with different email casing
	tests := []string{
		"owner@example.com",
		"OWNER@EXAMPLE.COM",
		"Owner@Example.Com",
		" owner@example.com ",
	}

	for _, email := range tests {
		t.Run(email, func(t *testing.T) {
			channels, err := server.getOwnedChannels(ctx, email)
			if err != nil {
				t.Fatalf("getOwnedChannels failed: %v", err)
			}
			if len(channels) != 1 {
				t.Errorf("expected 1 channel for %q, got %d", email, len(channels))
			}
		})
	}
}
