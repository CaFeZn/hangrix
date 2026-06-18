package service

import (
	"encoding/json"
	"strconv"
)

type sessionIDStepEnv struct {
	Env map[string]string `json:"env"`
}

func extractAgentRunSessionID(raw []byte) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var steps []sessionIDStepEnv
	if err := json.Unmarshal(raw, &steps); err != nil {
		return 0, false
	}
	for _, step := range steps {
		if step.Env == nil {
			continue
		}
		if v := step.Env["HANGRIX_SESSION_ID"]; v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil && id > 0 {
				return id, true
			}
		}
	}
	return 0, false
}
