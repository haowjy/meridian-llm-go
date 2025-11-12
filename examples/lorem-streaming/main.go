// lorem-streaming demonstrates streaming responses with the Lorem mock provider.
// This example requires NO API KEY and streams lorem ipsum text in real-time.
//
// Usage:
//
//	go run examples/lorem-streaming/main.go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/haowjy/meridian-llm-go"
	"github.com/haowjy/meridian-llm-go/examples/helpers"
	"github.com/haowjy/meridian-llm-go/providers/lorem"
)

func main() {
	fmt.Println("=== Lorem Streaming Example ===")
	fmt.Println("Demonstrating real-time streaming with mock provider (no API key required)\n")

	// Create Lorem provider (no API key needed)
	provider := lorem.NewProvider()
	fmt.Printf("Provider: %s\n\n", provider.Name())

	// Build request
	req := &llmprovider.GenerateRequest{
		Model: "lorem-fast", // Options: lorem-fast, lorem-slow, lorem-medium
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType:   llmprovider.BlockTypeText,
						Sequence:    0,
						TextContent: helpers.StrPtr("Hello, tell me something interesting!"),
					},
				},
			},
		},
		Params: &llmprovider.RequestParams{
			MaxTokens: helpers.IntPtr(100), // ~100 words
		},
	}

	// Start streaming
	fmt.Println("Streaming response:")
	fmt.Println("---")

	eventChan, err := provider.StreamResponse(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err)
	}

	// Read streaming events
	for event := range eventChan {
		// Handle errors
		if event.Error != nil {
			log.Fatalf("Streaming error: %v", event.Error)
		}

		// Handle deltas (incremental content)
		if event.Delta != nil {
			if event.Delta.TextDelta != nil {
				// Print text as it arrives (no newline - continuous stream)
				fmt.Print(*event.Delta.TextDelta)
			}
		}

		// Handle final metadata
		if event.Metadata != nil {
			fmt.Println("\n---\n")
			fmt.Printf("✓ Streaming complete\n")
			fmt.Printf("  Model: %s\n", event.Metadata.Model)
			fmt.Printf("  Input tokens: %d\n", event.Metadata.InputTokens)
			fmt.Printf("  Output tokens: %d\n", event.Metadata.OutputTokens)
			fmt.Printf("  Stop reason: %s\n", event.Metadata.StopReason)
		}
	}

	fmt.Println("\n=== Example Complete ===")
}
