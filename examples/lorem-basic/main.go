// lorem-basic demonstrates non-streaming (blocking) responses with the Lorem mock provider.
// This example requires NO API KEY and blocks for 10 seconds before returning a complete response.
//
// Usage:
//
//	go run examples/lorem-basic/main.go
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
	fmt.Println("=== Lorem Basic (Non-Streaming) Example ===")
	fmt.Println("Demonstrating blocking response with mock provider (no API key required)\n")

	// Create Lorem provider (no API key needed)
	provider := lorem.NewProvider()
	fmt.Printf("Provider: %s\n", provider.Name())

	// Build request
	req := &llmprovider.GenerateRequest{
		Model: "lorem-fast",
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

	// Generate response (blocks for 10 seconds)
	fmt.Println("\nGenerating response (this will take 10 seconds)...")

	resp, err := provider.GenerateResponse(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to generate response: %v", err)
	}

	// Print response
	fmt.Println("\nResponse:")
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

	fmt.Println("\n=== Example Complete ===")
}
