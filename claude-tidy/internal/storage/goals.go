package storage

import (
	"encoding/json"
	"os"
	"time"

	"claude-tidy/internal/config"
)

// SessionGoal stores a user-defined goal for a session.
type SessionGoal struct {
	Goal        string `json:"goal"`
	CreatedAt   string `json:"created_at"`
	ProjectPath string `json:"project_path"`
}

// GoalsData is the top-level structure for goals.json.
type GoalsData struct {
	Sessions map[string]SessionGoal `json:"sessions"`
}

// LoadGoals reads ~/.claude-tidy/goals.json.
func LoadGoals() (*GoalsData, error) {
	data, err := os.ReadFile(config.GoalsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return &GoalsData{Sessions: make(map[string]SessionGoal)}, nil
		}
		return nil, err
	}

	var goals GoalsData
	if err := json.Unmarshal(data, &goals); err != nil {
		return &GoalsData{Sessions: make(map[string]SessionGoal)}, nil
	}

	if goals.Sessions == nil {
		goals.Sessions = make(map[string]SessionGoal)
	}

	return &goals, nil
}

// SaveGoal saves a goal for a session ID.
func SaveGoal(sessionID, goal, projectPath string) error {
	if err := config.EnsureTidyDir(); err != nil {
		return err
	}

	goals, err := LoadGoals()
	if err != nil {
		goals = &GoalsData{Sessions: make(map[string]SessionGoal)}
	}

	goals.Sessions[sessionID] = SessionGoal{
		Goal:        goal,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		ProjectPath: projectPath,
	}

	data, err := json.MarshalIndent(goals, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(config.GoalsFile(), data, 0644)
}
