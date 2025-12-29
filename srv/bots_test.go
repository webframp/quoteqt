package srv

import (
	"net/http"
	"testing"
)

func TestParseNightbotChannel(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   *NightbotChannel
	}{
		{
			name:   "valid header",
			header: "name=night&displayName=Night&provider=twitch&providerId=11785491",
			want: &NightbotChannel{
				Name:        "night",
				DisplayName: "Night",
				Provider:    "twitch",
				ProviderID:  "11785491",
			},
		},
		{
			name:   "empty header",
			header: "",
			want:   nil,
		},
		{
			name:   "partial header",
			header: "name=streamer&provider=youtube",
			want: &NightbotChannel{
				Name:     "streamer",
				Provider: "youtube",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNightbotChannel(tt.header)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Errorf("expected %+v, got nil", tt.want)
				return
			}
			if got.Name != tt.want.Name || got.DisplayName != tt.want.DisplayName ||
				got.Provider != tt.want.Provider || got.ProviderID != tt.want.ProviderID {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseNightbotUser(t *testing.T) {
	header := "name=viewer&displayName=Viewer&provider=twitch&providerId=123&userLevel=owner"
	user := ParseNightbotUser(header)
	
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Name != "viewer" {
		t.Errorf("name = %q, want %q", user.Name, "viewer")
	}
	if user.UserLevel != "owner" {
		t.Errorf("userLevel = %q, want %q", user.UserLevel, "owner")
	}
}

func TestGetBotChannel(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		queryParam string
		wantName   string
		wantSource BotSource
		wantNil    bool
	}{
		{
			name:    "no headers or query",
			wantNil: true,
		},
		{
			name:       "nightbot header",
			headers:    map[string]string{"Nightbot-Channel": "name=beastyqt&provider=twitch"},
			wantName:   "beastyqt",
			wantSource: BotSourceNightbot,
		},
		{
			name:       "moobot header",
			headers:    map[string]string{"Moobot-channel-name": "SomeStreamer"},
			wantName:   "somestreamer", // lowercased
			wantSource: BotSourceMoobot,
		},
		{
			name:       "query param",
			queryParam: "testchannel",
			wantName:   "testchannel",
			wantSource: BotSourceQuery,
		},
		{
			name:       "nightbot takes precedence over moobot",
			headers:    map[string]string{"Nightbot-Channel": "name=nightbotch", "Moobot-channel-name": "moobotch"},
			wantName:   "nightbotch",
			wantSource: BotSourceNightbot,
		},
		{
			name:       "moobot takes precedence over query",
			headers:    map[string]string{"Moobot-channel-name": "MoobotChannel"},
			queryParam: "querychannel",
			wantName:   "moobotchannel",
			wantSource: BotSourceMoobot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "http://example.com/api/quote"
			if tt.queryParam != "" {
				url += "?channel=" + tt.queryParam
			}
			req, _ := http.NewRequest("GET", url, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := GetBotChannel(req)

			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tt.wantSource)
			}
		})
	}
}
