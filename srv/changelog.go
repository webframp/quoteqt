package srv

// ChangelogEntry represents a single changelog entry.
type ChangelogEntry struct {
	Date    string // YYYY-MM-DD format
	Version string // optional version tag
	Changes []string
}

// Changelog contains all changelog entries, newest first.
// Only include user-facing changes, not internal refactors.
var Changelog = []ChangelogEntry{
	{
		Date:    "2024-12-30",
		Version: "",
		Changes: []string{
			"Added changelog page",
		},
	},
	{
		Date:    "2024-12-29",
		Version: "",
		Changes: []string{
			"Added matchup tips - get advice for specific civ vs civ matchups via /api/matchup",
			"Added interactive API documentation at /api/",
			"Quotes can now be filtered by channel",
		},
	},
	{
		Date:    "2024-12-28",
		Version: "",
		Changes: []string{
			"Initial release",
			"Random quotes with optional civilization filtering",
			"Nightbot and Moobot integration for Twitch chat",
			"Community quote suggestions",
		},
	},
}
