package copilot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	copilot "github.com/github/copilot-sdk/go"
)

// Client wraps the GitHub Copilot SDK client
type Client struct {
	sdkClient *copilot.Client
	mu        sync.Mutex
}

// NewClient creates a new Copilot SDK client
func NewClient() (*Client, error) {
	// Check if Copilot CLI is available
	cliPath := os.Getenv("COPILOT_CLI_PATH")
	if cliPath == "" {
		cliPath = "copilot"
	}

	client := copilot.NewClient(&copilot.ClientOptions{
		CLIPath:  cliPath,
		LogLevel: "error",
	})

	// Start the client (spawns Copilot CLI in server mode)
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start Copilot CLI: %w. Please install copilot-cli: brew install copilot-cli", err)
	}

	return &Client{
		sdkClient: client,
	}, nil
}

// Chat sends a chat completion request using the Copilot SDK
func (c *Client) Chat(model string, prompt string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Map model name
	apiModel := mapModelName(model)
	ctx := context.Background()

	// Create a session with the specified model
	session, err := c.sdkClient.CreateSession(ctx, &copilot.SessionConfig{
		Model: apiModel,
		SystemMessage: &copilot.SystemMessageConfig{
			Mode: "append",
			Content: "You are a helpful code review assistant. Provide clear, actionable feedback on code changes. " +
				"Focus on security vulnerabilities, performance issues, bug risks, code style, and best practices.",
		},
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Disconnect()

	// Set up response collection
	var response strings.Builder
	done := make(chan bool)
	var sessionErr error

	// Subscribe to events
	unsubscribe := session.On(func(event copilot.SessionEvent) {
		switch event.Type {
		case "assistant.message":
			if event.Data.Content != nil {
				response.WriteString(*event.Data.Content)
			}
		case "session.idle":
			close(done)
		case "session.error":
			if event.Data.Content != nil {
				sessionErr = fmt.Errorf("session error: %s", *event.Data.Content)
			}
			close(done)
		}
	})
	defer unsubscribe()

	// Send the prompt
	_, err = session.Send(ctx, copilot.MessageOptions{
		Prompt: prompt,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// Wait for completion
	<-done

	if sessionErr != nil {
		return "", sessionErr
	}

	return response.String(), nil
}

// Close stops the Copilot CLI client
func (c *Client) Close() {
	if c.sdkClient != nil {
		c.sdkClient.Stop()
	}
}

// mapModelName maps user-friendly model names to Copilot model names
// All models available through Copilot CLI are supported
// See: https://docs.github.com/en/copilot/reference/ai-models/supported-models
func mapModelName(model string) string {
	switch strings.ToLower(model) {
	// OpenAI models
	case "gpt-4", "gpt4", "gpt-4o":
		return "gpt-4o"
	case "gpt-4o-mini", "gpt-4-mini":
		return "gpt-4o-mini"
	case "gpt-4.1":
		return "gpt-4.1"
	case "gpt-5":
		return "gpt-5"
	case "gpt-5-mini":
		return "gpt-5-mini"
	case "gpt-5.1":
		return "gpt-5.1"
	case "gpt-5.2":
		return "gpt-5.2"
	case "o1", "o1-preview":
		return "o1-preview"
	case "o1-mini":
		return "o1-mini"

	// Anthropic Claude models
	case "claude", "claude-sonnet", "claude-sonnet-4":
		return "claude-sonnet-4"
	case "claude-sonnet-4.5":
		return "claude-sonnet-4.5"
	case "claude-opus", "claude-opus-4.5":
		return "claude-opus-4.5"
	case "claude-haiku", "claude-haiku-4.5":
		return "claude-haiku-4.5"

	// Google Gemini models
	case "gemini", "gemini-2.5-pro":
		return "gemini-2.5-pro"
	case "gemini-3-flash":
		return "gemini-3-flash"
	case "gemini-3-pro":
		return "gemini-3-pro"

	// xAI Grok models
	case "grok", "grok-code-fast":
		return "grok-code-fast-1"

	default:
		return "gpt-4o-mini" // Default - good balance of quality and speed
	}
}
