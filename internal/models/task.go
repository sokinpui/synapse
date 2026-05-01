package models

import "github.com/sokinpui/synapse.go/internal/model"

type GenerationTask struct {
	TaskID    string        `json:"task_id"`
	Prompt    string        `json:"prompt"`
	ModelCode string        `json:"model_code"`
	Stream    bool          `json:"stream"`
	Config    *model.Config `json:"config,omitempty"`
	Images    [][]byte      `json:"images,omitempty"`
}
