// openrouter-streaming demonstrates streaming responses with the OpenRouter provider.
// This example requires an OPENROUTER_API_KEY environment variable.
//
// Usage (Option 1 - .env file):
//
//	# Create a .env file in your project root with:
//	# OPENROUTER_API_KEY=sk-or-...
//	go run examples/openrouter-streaming/main.go
//
// Usage (Option 2 - export):
//
//	export OPENROUTER_API_KEY="sk-or-..."
//	go run examples/openrouter-streaming/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/haowjy/meridian-llm-go"
	"github.com/haowjy/meridian-llm-go/examples/helpers"
	"github.com/haowjy/meridian-llm-go/providers/openrouter"
)

func main() {
	fmt.Println("=== OpenRouter Streaming Example ===")
	fmt.Println("Demonstrating streaming with thinking blocks\n")
	fmt.Println("NOTE: web_search currently disabled - custom implementation pending\n")

	// Load .env file if present (searches up directory tree)
	helpers.LoadEnv()

	// Get API key from environment
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required\n" +
			"Option 1: Create .env file with OPENROUTER_API_KEY=sk-or-...\n" +
			"Option 2: export OPENROUTER_API_KEY=\"sk-or-...\"")
	}

	// Create OpenRouter provider
	provider, err := openrouter.NewProvider(apiKey)
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	fmt.Printf("Provider: %s\n\n", provider.Name())

	// NOTE: web_search is currently blocked with OpenRouter pending custom implementation
	// TODO(search): Re-enable when custom web_search tool is implemented
	//
	// // Create search tool
	// searchTool, err := llmprovider.NewSearchTool()
	// if err != nil {
	// 	log.Fatalf("Failed to create search tool: %v", err)
	// }

	// Build request (without web search for now)
	req := &llmprovider.GenerateRequest{
		Model: "moonshotai/kimi-k2-thinking", // Thinking-enabled model
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType:   llmprovider.BlockTypeText,
						Sequence:    0,
						TextContent: helpers.StrPtr("Explain the key benefits of Go's goroutines compared to traditional threads."),
					},
				},
			},
		},
		Params: &llmprovider.RequestParams{
			MaxTokens: helpers.IntPtr(1000),
			// NOTE: web_search tool commented out until custom implementation is ready
			// Tools: []llmprovider.Tool{
			// 	*searchTool,
			// },
		},
	}

	// Start streaming
	fmt.Println("Streaming response:")
	fmt.Println("---")

	eventChan, err := provider.StreamResponse(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err)
	}

	// Track which block types we've seen
	var hasThinking bool
	var hasWebSearch bool
	var hasText bool

	// Read streaming events
	for event := range eventChan {
		// Handle errors
		if event.Error != nil {
			log.Fatalf("Streaming error: %v", event.Error)
		}

		// Handle complete blocks (for persistence)
		if event.Block != nil {
			switch event.Block.BlockType {
			case llmprovider.BlockTypeWebSearchResult:
				if !hasWebSearch {
					fmt.Println("\n🔍 WEB SEARCH RESULTS:\n---")
					hasWebSearch = true
				}
				// Print web search results
				if event.Block.Content != nil {
					if results, ok := event.Block.Content["results"].([]interface{}); ok {
						for i, result := range results {
							if r, ok := result.(map[string]interface{}); ok {
								fmt.Printf("%d. %s\n", i+1, r["title"])
								fmt.Printf("   %s\n", r["url"])
							}
						}
					}
				}
				fmt.Println("---\n")

			case llmprovider.BlockTypeThinking:
				if !hasThinking {
					fmt.Println("\n🧠 THINKING:\n---")
					hasThinking = true
				}
				if event.Block.TextContent != nil {
					fmt.Print(*event.Block.TextContent)
				}

			case llmprovider.BlockTypeText:
				if hasThinking {
					fmt.Println("\n---\n")
				}
				if !hasText {
					fmt.Println("💬 RESPONSE:\n---")
					hasText = true
				}
				if event.Block.TextContent != nil {
					fmt.Print(*event.Block.TextContent)
				}

			case llmprovider.BlockTypeToolUse:
				// Tool use blocks are handled internally
				fmt.Println("\n[Tool execution completed]")
			}
		}

		// Handle deltas (real-time streaming)
		if event.Delta != nil {
			if event.Delta.TextDelta != nil {
				// Print text as it arrives (no newline - continuous stream)
				fmt.Print(*event.Delta.TextDelta)
			}
		}

		// Handle final metadata
		if event.Metadata != nil {
			if hasText {
				fmt.Println("\n---\n")
			}
			fmt.Printf("✓ Streaming complete\n")
			fmt.Printf("  Model: %s\n", event.Metadata.Model)
			if event.Metadata.InputTokens > 0 {
				fmt.Printf("  Input tokens: %d\n", event.Metadata.InputTokens)
			}
			if event.Metadata.OutputTokens > 0 {
				fmt.Printf("  Output tokens: %d\n", event.Metadata.OutputTokens)
			}
			fmt.Printf("  Stop reason: %s\n", event.Metadata.StopReason)
		}
	}

	fmt.Println("\n=== Example Complete ===")
}
