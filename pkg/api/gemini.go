package api

import (
	"context"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const modelName = "gemini-1.5-flash"
const modelResponseMIMEType = "application/json"

// GeminiClient wraps the generative model client
type GeminiClient struct {
	Ctx   context.Context
	model *genai.GenerativeModel
}

// NewGeminiClient initializes a new Gemini client
func NewGeminiClient(ctx context.Context) (*GeminiClient, error) {
	apiKey := os.Getenv("GEMINI_API_TOKEN")
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	model := client.GenerativeModel(modelName)
	model.ResponseMIMEType = modelResponseMIMEType

	// Specify the schema for the model
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeArray,
		Items: &genai.Schema{
			Type: genai.TypeString,
		},
	}

	return &GeminiClient{
		Ctx:   ctx,
		model: model,
	}, nil
}

// Model returns the generative model
func (c *GeminiClient) Model() *genai.GenerativeModel {
	return c.model
}

// Close closes the Gemini client
func (c *GeminiClient) Close() {
	// Implement any necessary cleanup here
}
