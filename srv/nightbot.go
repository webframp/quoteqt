package srv

import (
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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

// AddNightbotAttributes adds Nightbot header data as span attributes for observability
func AddNightbotAttributes(r *http.Request) {
	span := trace.SpanFromContext(r.Context())
	if !span.IsRecording() {
		return
	}

	// Parse and add channel attributes
	if channel := ParseNightbotChannel(r.Header.Get("Nightbot-Channel")); channel != nil {
		span.SetAttributes(
			attribute.String("nightbot.channel.name", channel.Name),
			attribute.String("nightbot.channel.display_name", channel.DisplayName),
			attribute.String("nightbot.channel.provider", channel.Provider),
			attribute.String("nightbot.channel.provider_id", channel.ProviderID),
		)
	}

	// Parse and add user attributes
	if user := ParseNightbotUser(r.Header.Get("Nightbot-User")); user != nil {
		span.SetAttributes(
			attribute.String("nightbot.user.name", user.Name),
			attribute.String("nightbot.user.display_name", user.DisplayName),
			attribute.String("nightbot.user.provider", user.Provider),
			attribute.String("nightbot.user.user_level", user.UserLevel),
		)
	}
}
