package utils

import (
	"testing"
)

func TestGenerateShortCode(t *testing.T) {
	code1 := GenerateShortCode(8)
	code2 := GenerateShortCode(8)

	if len(code1) != 8 {
		t.Errorf("Expected length 8, got %d", len(code1))
	}
	if len(code2) != 8 {
		t.Errorf("Expected length 8, got %d", len(code2))
	}
	if code1 == code2 {
		t.Errorf("Expected unique short codes, got duplicates: %s", code1)
	}
}
