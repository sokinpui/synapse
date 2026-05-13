package task

import (
	"encoding/json"
)

type GenerationTask struct {
	TaskID    string          `json:"task_id"`
	ModelCode string          `json:"model_code"`
	Endpoint  string          `json:"endpoint"`
	Stream    bool            `json:"stream"`
	Payload   json.RawMessage `json:"payload"`
}
