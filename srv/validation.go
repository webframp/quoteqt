package srv

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Field length limits
const (
	MaxQuoteTextLen   = 1000
	MaxAuthorLen      = 100
	MaxCivNameLen     = 100
	MaxShortnameLen   = 50
	MaxDLCLen         = 100
)

// ValidationError represents a validation failure
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateLength checks if a string exceeds the max length (in runes, not bytes)
func ValidateLength(field, value string, maxLen int) error {
	if utf8.RuneCountInString(value) > maxLen {
		return ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be %d characters or less", maxLen),
		}
	}
	return nil
}

// ValidateRequired checks if a string is non-empty after trimming
func ValidateRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return ValidationError{
			Field:   field,
			Message: "is required",
		}
	}
	return nil
}

// ValidateQuoteText validates quote text field
func ValidateQuoteText(text string) error {
	if err := ValidateRequired("Quote text", text); err != nil {
		return err
	}
	return ValidateLength("Quote text", text, MaxQuoteTextLen)
}

// ValidateAuthor validates author field (optional)
func ValidateAuthor(author string) error {
	if author == "" {
		return nil
	}
	return ValidateLength("Author", author, MaxAuthorLen)
}

// ValidateCivName validates civilization name field
func ValidateCivName(name string) error {
	if err := ValidateRequired("Name", name); err != nil {
		return err
	}
	return ValidateLength("Name", name, MaxCivNameLen)
}

// ValidateShortname validates shortname field (optional)
func ValidateShortname(shortname string) error {
	if shortname == "" {
		return nil
	}
	return ValidateLength("Shortname", shortname, MaxShortnameLen)
}

// ValidateDLC validates DLC field (optional)
func ValidateDLC(dlc string) error {
	if dlc == "" {
		return nil
	}
	return ValidateLength("DLC", dlc, MaxDLCLen)
}

// MaxRequestBodySize is the maximum allowed request body size (5MB)
// Needs to be large enough for Nightbot command imports
const MaxRequestBodySize = 5 * 1024 * 1024

// LimitRequestBody wraps a handler to limit request body size
func LimitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
		next.ServeHTTP(w, r)
	})
}
