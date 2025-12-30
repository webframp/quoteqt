package srv

import (
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// BotSource identifies which bot sent the request
type BotSource string

const (
	BotSourceNightbot BotSource = "nightbot"
	BotSourceMoobot   BotSource = "moobot"
	BotSourceQuery    BotSource = "query"
	BotSourceNone     BotSource = ""
)

// BotChannel contains channel information extracted from bot headers
type BotChannel struct {
	Name   string
	Source BotSource
}

// GetBotChannel extracts the channel name from bot headers or query param.
// Priority: Nightbot header > Moobot header > ?channel= query param
func GetBotChannel(r *http.Request) *BotChannel {
	// Check Nightbot header first
	if nb := ParseNightbotChannel(r.Header.Get("Nightbot-Channel")); nb != nil && nb.Name != "" {
		return &BotChannel{Name: nb.Name, Source: BotSourceNightbot}
	}

	// Check Moobot header
	if moobotChannel := r.Header.Get("Moobot-channel-name"); moobotChannel != "" {
		return &BotChannel{Name: strings.ToLower(moobotChannel), Source: BotSourceMoobot}
	}

	// Fall back to query param
	if ch := r.URL.Query().Get("channel"); ch != "" {
		return &BotChannel{Name: ch, Source: BotSourceQuery}
	}

	return nil
}

// NightbotChannel represents parsed Nightbot-Channel header data
type NightbotChannel struct {
	Name        string
	DisplayName string
	Provider    string
	ProviderID  string
}

// ParseNightbotChannel parses the Nightbot-Channel header
// Format: name=night&displayName=Night&provider=twitch&providerId=11785491
func ParseNightbotChannel(header string) *NightbotChannel {
	if header == "" {
		return nil
	}
	values, err := url.ParseQuery(header)
	if err != nil {
		return nil
	}
	return &NightbotChannel{
		Name:        values.Get("name"),
		DisplayName: values.Get("displayName"),
		Provider:    values.Get("provider"),
		ProviderID:  values.Get("providerId"),
	}
}

// NightbotUser represents parsed Nightbot-User header data
type NightbotUser struct {
	Name        string
	DisplayName string
	Provider    string
	ProviderID  string
	UserLevel   string
}

// ParseNightbotUser parses the Nightbot-User header
func ParseNightbotUser(header string) *NightbotUser {
	if header == "" {
		return nil
	}
	values, err := url.ParseQuery(header)
	if err != nil {
		return nil
	}
	return &NightbotUser{
		Name:        values.Get("name"),
		DisplayName: values.Get("displayName"),
		Provider:    values.Get("provider"),
		ProviderID:  values.Get("providerId"),
		UserLevel:   values.Get("userLevel"),
	}
}

// AddBotAttributes adds bot header data as span attributes for observability
func AddBotAttributes(r *http.Request) {
	span := trace.SpanFromContext(r.Context())
	if !span.IsRecording() {
		return
	}

	// Check for Nightbot headers
	if channel := ParseNightbotChannel(r.Header.Get("Nightbot-Channel")); channel != nil {
		span.SetAttributes(
			attribute.String("bot.source", "nightbot"),
			attribute.String("bot.channel.name", channel.Name),
			attribute.String("bot.channel.display_name", channel.DisplayName),
			attribute.String("bot.channel.provider", channel.Provider),
			attribute.String("bot.channel.provider_id", channel.ProviderID),
		)

		// Also add user attributes if present
		if user := ParseNightbotUser(r.Header.Get("Nightbot-User")); user != nil {
			span.SetAttributes(
				attribute.String("bot.user.name", user.Name),
				attribute.String("bot.user.display_name", user.DisplayName),
				attribute.String("bot.user.provider", user.Provider),
				attribute.String("bot.user.user_level", user.UserLevel),
			)
		}
		return
	}

	// Check for Moobot headers
	if moobotChannel := r.Header.Get("Moobot-channel-name"); moobotChannel != "" {
		span.SetAttributes(
			attribute.String("bot.source", "moobot"),
			attribute.String("bot.channel.name", moobotChannel),
		)

		// Add Moobot user info if present
		if userName := r.Header.Get("Moobot-user-name"); userName != "" {
			span.SetAttributes(attribute.String("bot.user.name", userName))
		}
		if userID := r.Header.Get("Moobot-user-id"); userID != "" {
			span.SetAttributes(attribute.String("bot.user.id", userID))
		}
	}
}

// AddNightbotAttributes is an alias for AddBotAttributes for backwards compatibility
func AddNightbotAttributes(r *http.Request) {
	AddBotAttributes(r)
}

// GetBotUser extracts the username from bot headers.
// Returns the display name if available, otherwise the name.
// Returns empty string if no bot user info found.
func GetBotUser(r *http.Request) string {
	// Check Nightbot user header
	if user := ParseNightbotUser(r.Header.Get("Nightbot-User")); user != nil {
		if user.DisplayName != "" {
			return user.DisplayName
		}
		if user.Name != "" {
			return user.Name
		}
	}

	// Check Moobot user header
	if userName := r.Header.Get("Moobot-user-name"); userName != "" {
		return userName
	}

	return ""
}
