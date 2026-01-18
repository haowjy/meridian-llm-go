// anthropic-thinking demonstrates streaming with extended thinking mode.
// This example requires an ANTHROPIC_API_KEY environment variable.
// It shows how Claude emits separate thinking and text blocks during streaming.
//
// Usage (Option 1 - .env file):
//
//	# Create a .env file in your project root with:
//	# ANTHROPIC_API_KEY=sk-ant-...
//	go run examples/anthropic-thinking/main.go
//
// Usage (Option 2 - export):
//
//	export ANTHROPIC_API_KEY="sk-ant-..."
//	go run examples/anthropic-thinking/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/haowjy/meridian-llm-go"
	"github.com/haowjy/meridian-llm-go/examples/helpers"
	"github.com/haowjy/meridian-llm-go/providers/anthropic"
)

func main() {
	fmt.Println("=== Anthropic Extended Thinking Example ===")
	fmt.Println("Demonstrating thinking mode with separate thinking/answer blocks")

	// Load .env file if present (searches up directory tree)
	helpers.LoadEnv()

	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required\n" +
			"Option 1: Create .env file with ANTHROPIC_API_KEY=sk-ant-...\n" +
			"Option 2: export ANTHROPIC_API_KEY=\"sk-ant-...\"")
	}

	// Create Anthropic provider
	provider, err := anthropic.NewProvider(apiKey)
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	fmt.Printf("Provider: %s\n\n", provider.Name())

	// Build request with thinking enabled
	req := &llmprovider.GenerateRequest{
		Model: "claude-sonnet-4-5-20250929", // Better model for complex reasoning
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType: llmprovider.BlockTypeText,
						Sequence:  0,
						TextContent: helpers.StrPtr(
							"Solve this logic puzzle: Three friends - Alice, Bob, and Charlie - " +
								"each have a different favorite color (red, blue, green). " +
								"Alice doesn't like red. Bob's favorite is not blue. " +
								"Charlie's favorite is green. " +
								"What is each person's favorite color?",
						),
					},
				},
			},
		},
		Params: &llmprovider.RequestParams{
			MaxTokens:       helpers.IntPtr(6000),
			Temperature:     helpers.FloatPtr(1.0),
			ThinkingEnabled: helpers.BoolPtr(true),
			ThinkingLevel:   helpers.StrPtr("medium"), // medium = 5k token budget
		},
	}

	// Start streaming
	fmt.Println("Streaming response with thinking...")

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

		// Handle AG-UI thinking block start
		if _, ok := event.GetThinkingStart(); ok {
			fmt.Println("🧠 THINKING BLOCK:")
			fmt.Println("---")
		}

		// Handle AG-UI thinking text content
		if evt, ok := event.GetThinkingTextMessageContent(); ok {
			fmt.Print(evt.Delta)
		}

		// Handle AG-UI text message start (answer block)
		if _, ok := event.GetTextMessageStart(); ok {
			fmt.Println("\n💬 ANSWER BLOCK:")
			fmt.Println("---")
		}

		// Handle AG-UI text message content
		if evt, ok := event.GetTextMessageContent(); ok {
			fmt.Print(evt.Delta)
		}

		// Handle final metadata
		if event.Metadata != nil {
			fmt.Println("\n---")
			fmt.Printf("✓ Streaming complete\n")
			fmt.Printf("  Model: %s\n", event.Metadata.Model)
			fmt.Printf("  Input tokens: %d\n", event.Metadata.InputTokens)
			fmt.Printf("  Output tokens: %d\n", event.Metadata.OutputTokens)
			fmt.Printf("  Stop reason: %s\n", event.Metadata.StopReason)

			// Print any extra metadata
			if len(event.Metadata.ResponseMetadata) > 0 {
				fmt.Println("\n  Additional metadata:")
				for key, value := range event.Metadata.ResponseMetadata {
					fmt.Printf("    %s: %v\n", key, value)
				}
			}
		}
	}

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\nNote: Thinking mode shows Claude's reasoning process in a separate block")
	fmt.Println("before providing the final answer. This is useful for complex problems that")
	fmt.Println("benefit from step-by-step reasoning.")
}
