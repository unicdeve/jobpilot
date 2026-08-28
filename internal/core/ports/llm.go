package ports

import (
	"context"
)

type Request struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float64
	MaxTokens    int
}

type Response struct {
	Content string
}

type LLMProvider interface {
	Generate(ctx context.Context, req Request) (Response, error)
	TestConnectivity(ctx context.Context) error
}
