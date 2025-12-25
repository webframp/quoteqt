package srv

import "testing"

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
