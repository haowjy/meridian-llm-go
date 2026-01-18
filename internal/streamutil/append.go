package streamutil

import (
	"strings"
)

// AppendWithOptionalSeparator appends next to b, inserting sep only when needed.
//
// This is useful for providers/models that emit adjacent chunks without any leading/trailing
// whitespace (e.g. reasoning summaries). It avoids producing glued output like:
//
//	"content.Evaluating ..."
//
// Rules:
// - never insert sep at start (when b is empty),
// - never insert sep if b already ends with whitespace,
// - never insert sep if next already starts with whitespace.
//
// Whitespace checks are intentionally conservative (space/newline/tab) to avoid changing
// content semantics across providers.
func AppendWithOptionalSeparator(b *strings.Builder, next, sep string) {
	if b == nil || next == "" {
		return
	}
	if b.Len() == 0 {
		b.WriteString(next)
		return
	}

	existing := b.String()
	if !hasTrailingWhitespace(existing) && !hasLeadingWhitespace(next) {
		b.WriteString(sep)
	}
	b.WriteString(next)
}

func hasTrailingWhitespace(s string) bool {
	return strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\t")
}

func hasLeadingWhitespace(s string) bool {
	return strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\n") || strings.HasPrefix(s, "\t")
}
