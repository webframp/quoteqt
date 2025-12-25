package srv

import (
	"strings"
	"testing"
)

func TestValidateQuoteText(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{"valid short", "Hello world", false},
		{"valid max length", strings.Repeat("a", MaxQuoteTextLen), false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"too long", strings.Repeat("a", MaxQuoteTextLen+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuoteText(tt.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuoteText() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAuthor(t *testing.T) {
	tests := []struct {
		name    string
		author  string
		wantErr bool
	}{
		{"valid", "John Doe", false},
		{"empty (optional)", "", false},
		{"max length", strings.Repeat("a", MaxAuthorLen), false},
		{"too long", strings.Repeat("a", MaxAuthorLen+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAuthor(tt.author)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAuthor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCivName(t *testing.T) {
	tests := []struct {
		name    string
		civName string
		wantErr bool
	}{
		{"valid", "Holy Roman Empire", false},
		{"empty", "", true},
		{"max length", strings.Repeat("a", MaxCivNameLen), false},
		{"too long", strings.Repeat("a", MaxCivNameLen+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCivName(tt.civName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCivName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLength_Unicode(t *testing.T) {
	// Test that we count runes, not bytes
	// "日本語" is 3 runes but 9 bytes
	err := ValidateLength("test", "日本語", 3)
	if err != nil {
		t.Errorf("Should allow 3 unicode characters within limit of 3: %v", err)
	}

	err = ValidateLength("test", "日本語", 2)
	if err == nil {
		t.Error("Should reject 3 unicode characters when limit is 2")
	}
}
