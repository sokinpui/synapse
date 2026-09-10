package main

import (
	"encoding/json"
	"errors"
)

var (
	ErrModelNotFound = errors.New("model not found in registry")
	ErrGeneration    = errors.New("error during text generation")
	ErrConfiguration = errors.New("failed to initialize client, please check configuration")
)

type GenerationTask struct {
	TaskID    string          `json:"task_id"`
	ModelCode string          `json:"model_code"`
	Endpoint  string          `json:"endpoint"`
	Stream    bool            `json:"stream"`
	Payload   json.RawMessage `json:"payload"`
}

type Result struct {
	Raw     json.RawMessage
	IsError bool
	IsDone  bool
}

type ModelListJSON struct {
	Object string      `json:"object"`
	Data   []ModelJSON `json:"data"`
}

type ModelJSON struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
