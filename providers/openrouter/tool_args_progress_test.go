package openrouter

import "testing"

func TestToolArgsProgress_WhitespaceOutsideStringIsNotMeaningful(t *testing.T) {
	var p toolArgsProgress

	// Start outside string (default)
	if meaningful := p.Apply("   \n\r\t"); meaningful {
		t.Fatalf("meaningful = %v, want false", meaningful)
	}
	if p.inString {
		t.Fatalf("inString = %v, want false", p.inString)
	}
}

func TestToolArgsProgress_WhitespaceInsideStringIsMeaningful(t *testing.T) {
	var p toolArgsProgress

	// Enter a JSON string (opening quote)
	if meaningful := p.Apply(`{"file_text":"`); !meaningful {
		t.Fatalf("meaningful = %v, want true (non-whitespace)", meaningful)
	}
	if !p.inString {
		t.Fatalf("inString = %v, want true", p.inString)
	}

	// Whitespace inside a JSON string is semantically meaningful (content).
	if meaningful := p.Apply("   \n"); !meaningful {
		t.Fatalf("meaningful = %v, want true", meaningful)
	}
	if !p.inString {
		t.Fatalf("inString = %v, want true", p.inString)
	}
}

func TestToolArgsProgress_WhitespaceAfterClosingStringIsNotMeaningful(t *testing.T) {
	var p toolArgsProgress

	_ = p.Apply(`{"x":"hello`) // enter string
	if !p.inString {
		t.Fatalf("inString = %v, want true", p.inString)
	}

	// Close string and object
	if meaningful := p.Apply(`"}`); !meaningful {
		t.Fatalf("meaningful = %v, want true", meaningful)
	}
	if p.inString {
		t.Fatalf("inString = %v, want false", p.inString)
	}

	// Now whitespace outside string is not meaningful.
	if meaningful := p.Apply("\n\n   "); meaningful {
		t.Fatalf("meaningful = %v, want false", meaningful)
	}
}
