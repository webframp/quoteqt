package srv

// ChangelogEntry represents a single changelog entry.
type ChangelogEntry struct {
	Date    string // YYYY-MM-DD format
	Version string // optional version tag
	Changes []string
}

// Changelog contains all changelog entries, newest first.
var Changelog = []ChangelogEntry{
	{
		Date:    "2024-12-30",
		Version: "",
		Changes: []string{
			"Add configurable rate limits via environment variables",
			"Refactor to use errors.Is() for error comparisons",
			"Extract auth helper functions for cleaner code",
			"Add consistent security event logging",
			"Replace hardcoded colors with CSS variables",
		},
	},
	{
		Date:    "2024-12-29",
		Version: "",
		Changes: []string{
			"Add matchup tips feature for civ vs civ advice",
			"Improve API documentation with Swagger UI",
			"Add channel-based quote filtering",
		},
	},
	{
		Date:    "2024-12-28",
		Version: "",
		Changes: []string{
			"Initial release",
			"Quote management with civilization filtering",
			"Nightbot and Moobot integration",
			"Quote suggestions system",
			"Channel owner permissions",
		},
	},
}
