package model

import (
	"encoding/json"
	"errors"
)

// Request encapsulates all input data for a generation task.
type Request struct {
	TaskID  string
	Endpoint string
	Payload json.RawMessage
}

type Result struct {
	// Raw contains the raw JSON fragment or full response
	Raw json.RawMessage
	// IsError indicates if the result represents a provider error
	IsError bool
	// IsDone indicates the end of a stream
	IsDone bool
}

// Custom errors for the library.
var (
	ErrModelNotFound = errors.New("model not found in registry")
	ErrGeneration    = errors.New("error during text generation")
	ErrConfiguration = errors.New("failed to initialize client, please check configuration")
)
