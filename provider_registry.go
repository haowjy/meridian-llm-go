package llmprovider

// ProviderID represents a unique provider identifier.
// Using a typed constant prevents typos and provides compile-time safety.
type ProviderID string

// Known provider identifiers
const (
	// ProviderAnthropic is Anthropic's Claude API
	ProviderAnthropic ProviderID = "anthropic"

	// ProviderOpenAI is OpenAI's GPT API
	ProviderOpenAI ProviderID = "openai"

	// ProviderGoogle is Google's Gemini API
	ProviderGoogle ProviderID = "google"

	// ProviderLorem is the mock Lorem provider for testing
	ProviderLorem ProviderID = "lorem"
)

// String returns the string representation of the provider ID
func (p ProviderID) String() string {
	return string(p)
}

// IsValid returns true if the provider ID is a known provider
func (p ProviderID) IsValid() bool {
	switch p {
	case ProviderAnthropic, ProviderOpenAI, ProviderGoogle, ProviderLorem:
		return true
	default:
		return false
	}
}
