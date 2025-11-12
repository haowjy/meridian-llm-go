// anthropic-basic demonstrates non-streaming (blocking) responses with the Anthropic provider.
// This example requires an ANTHROPIC_API_KEY environment variable.
//
// Usage (Option 1 - .env file):
//
//	# Create a .env file in your project root with:
//	# ANTHROPIC_API_KEY=sk-ant-...
//	go run examples/anthropic-basic/main.go
//
// Usage (Option 2 - export):
//
//	export ANTHROPIC_API_KEY="sk-ant-..."
//	go run examples/anthropic-basic/main.go
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
	fmt.Println("=== Anthropic Basic (Non-Streaming) Example ===")
	fmt.Println("Demonstrating blocking response with Claude\n")

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
	fmt.Printf("Provider: %s\n", provider.Name())

	// Build request
	req := &llmprovider.GenerateRequest{
		Model: "claude-haiku-4-5-20251001", // Fast, cost-effective model
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType:   llmprovider.BlockTypeText,
						Sequence:    0,
						TextContent: helpers.StrPtr("Explain what a variable is in programming in 2-3 sentences."),
					},
				},
			},
		},
		Params: &llmprovider.RequestParams{
			MaxTokens:   helpers.IntPtr(150),
			Temperature: helpers.FloatPtr(0.7),
		},
	}

	// Generate response (blocks until complete)
	fmt.Println("\nGenerating response from Claude...\n")

	resp, err := provider.GenerateResponse(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to generate response: %v", err)
	}

	// Print response
	fmt.Println("Response:")
	fmt.Println("---")
	for _, block := range resp.Blocks {
		if block.TextContent != nil {
			fmt.Println(*block.TextContent)
		}
	}
	fmt.Println("---\n")

	// Print metadata
	fmt.Printf("✓ Response complete\n")
	fmt.Printf("  Model: %s\n", resp.Model)
	fmt.Printf("  Input tokens: %d\n", resp.InputTokens)
	fmt.Printf("  Output tokens: %d\n", resp.OutputTokens)
	fmt.Printf("  Stop reason: %s\n", resp.StopReason)

	// Print any extra metadata
	if len(resp.ResponseMetadata) > 0 {
		fmt.Println("\n  Additional metadata:")
		for key, value := range resp.ResponseMetadata {
			fmt.Printf("    %s: %v\n", key, value)
		}
	}

	fmt.Println("\n=== Example Complete ===")
}
