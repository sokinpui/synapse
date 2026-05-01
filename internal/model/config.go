package model

import "errors"

// Config defines the generation configuration for a model.
// All fields are optional.
type Config struct {
	Temperature  *float32 `json:"temperature,omitempty"`
	TopP         *float32 `json:"top_p,omitempty"`
	TopK         *float32 `json:"top_k,omitempty"`
	OutputLength int32    `json:"output_length,omitempty"`
}

// Custom errors for the library.
var (
	ErrModelNotFound = errors.New("model not found in registry")
	ErrGeneration    = errors.New("error during text generation")
	ErrConfiguration = errors.New("failed to initialize client, please check configuration")
)
