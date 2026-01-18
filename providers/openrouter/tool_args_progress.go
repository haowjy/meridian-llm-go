package openrouter

import "unicode"

// toolArgsProgress tracks whether we're currently inside a JSON string while streaming
// tool-call arguments. This lets us distinguish:
//   - whitespace that is semantically meaningful (inside a JSON string, e.g. file_text),
//   - whitespace that is semantically irrelevant (between JSON tokens), which should not
//     count as progress for stall detection.
//
// It is intentionally minimal and reusable across Chat Completions and Responses APIs.
type toolArgsProgress struct {
	inString bool
	escape   bool
}

// Apply updates internal state based on the incoming delta and returns whether the delta
// should be considered "meaningful progress" for the tool args stream.
//
// Rules:
//   - If we're inside a JSON string, any delta (including whitespace) is meaningful.
//   - If we're outside a JSON string, only deltas containing at least one non-whitespace rune
//     are meaningful (pure whitespace does not advance JSON parsing).
func (p *toolArgsProgress) Apply(delta string) (meaningful bool) {
	if delta == "" {
		return false
	}

	meaningful = p.inString || containsNonWhitespace(delta)

	// Update JSON string/escape state.
	//
	// Notes:
	// - We only treat backslash escapes while inside a string.
	// - Quotes toggle string state unless escaped.
	for _, r := range delta {
		if p.escape {
			p.escape = false
			continue
		}
		if p.inString && r == '\\' {
			p.escape = true
			continue
		}
		if r == '"' {
			p.inString = !p.inString
		}
	}

	return meaningful
}

func containsNonWhitespace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
