// Package helpers provides utility functions for the llmprovider examples.
package helpers

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// StrPtr returns a pointer to the given string.
// Useful for creating optional string parameters.
func StrPtr(s string) *string {
	return &s
}

// IntPtr returns a pointer to the given int.
// Useful for creating optional int parameters.
func IntPtr(i int) *int {
	return &i
}

// FloatPtr returns a pointer to the given float64.
// Useful for creating optional float parameters.
func FloatPtr(f float64) *float64 {
	return &f
}

// BoolPtr returns a pointer to the given bool.
// Useful for creating optional bool parameters.
func BoolPtr(b bool) *bool {
	return &b
}

// LoadEnv searches for a .env file starting from the current directory
// and walking up the directory tree. It loads the first .env file found.
// If no .env file is found, it silently continues (using system env vars).
//
// This allows examples to work from any location in the workspace:
//   - go run meridian-llm-go/examples/anthropic-basic/main.go (from workspace root)
//   - go run examples/anthropic-basic/main.go (from meridian-llm-go/)
//   - go run anthropic-basic/main.go (from examples/)
func LoadEnv() {
	// Get current working directory
	dir, err := os.Getwd()
	if err != nil {
		// Silently continue - will use system env vars
		return
	}

	// Walk up directory tree looking for .env
	for {
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			// Found .env file, load it
			_ = godotenv.Load(envPath)
			return
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root, stop
			return
		}
		dir = parent
	}
}
