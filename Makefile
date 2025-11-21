.PHONY: examples clean test run-anthropic-basic run-anthropic-streaming run-anthropic-thinking run-lorem-basic run-lorem-streaming run-openrouter-streaming

# Build all example binaries
examples:
	@echo "Building all examples..."
	@cd examples/anthropic-basic && go build -o ../../anthropic-basic
	@cd examples/anthropic-streaming && go build -o ../../anthropic-streaming
	@cd examples/anthropic-thinking && go build -o ../../anthropic-thinking
	@cd examples/lorem-basic && go build -o ../../lorem-basic
	@cd examples/lorem-streaming && go build -o ../../lorem-streaming
	@cd examples/openrouter-streaming && go build -o ../../openrouter-streaming
	@echo "Examples built successfully!"

# Clean example binaries
clean:
	@echo "Cleaning example binaries..."
	@rm -f anthropic-basic anthropic-streaming anthropic-thinking lorem-basic lorem-streaming openrouter-streaming
	@echo "Cleaned!"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run individual examples (requires ANTHROPIC_API_KEY for anthropic examples)
run-anthropic-basic: examples
	@./anthropic-basic

run-anthropic-streaming: examples
	@./anthropic-streaming

run-anthropic-thinking: examples
	@./anthropic-thinking

run-lorem-basic: examples
	@./lorem-basic

run-lorem-streaming: examples
	@./lorem-streaming

run-openrouter-streaming: examples
	@./openrouter-streaming

# Help
help:
	@echo "Available targets:"
	@echo "  make examples               - Build all example binaries"
	@echo "  make clean                  - Remove all example binaries"
	@echo "  make test                   - Run all tests"
	@echo "  make run-anthropic-basic    - Build and run Anthropic basic example"
	@echo "  make run-anthropic-streaming - Build and run Anthropic streaming example"
	@echo "  make run-anthropic-thinking - Build and run Anthropic thinking example"
	@echo "  make run-lorem-basic        - Build and run Lorem basic example"
	@echo "  make run-lorem-streaming    - Build and run Lorem streaming example"
	@echo "  make run-openrouter-streaming - Build and run OpenRouter streaming example"
	@echo ""
	@echo "Note: Anthropic examples require ANTHROPIC_API_KEY environment variable"
	@echo "Note: OpenRouter examples require OPENROUTER_API_KEY environment variable"
