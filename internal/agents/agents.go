package agents

import "errors"

var ErrNotFound = errors.New("agent executable not found")

type Agent struct {
	Name        string
	Executable  string
	Description string
}

type Status struct {
	Name  string
	State string
	Path  string
}

type LookupFunc func(name string) (string, error)

func Supported() []Agent {
	return []Agent{
		{Name: "hermes", Executable: "hermes", Description: "Hermes Agent"},
		{Name: "openclaw", Executable: "openclaw", Description: "OpenClaw"},
		{Name: "claude", Executable: "claude", Description: "Claude Code"},
		{Name: "codex", Executable: "codex", Description: "OpenAI Codex"},
	}
}

func CheckAll(lookup LookupFunc) []Status {
	agents := Supported()
	statuses := make([]Status, 0, len(agents))
	for _, agent := range agents {
		path, err := lookup(agent.Executable)
		status := Status{Name: agent.Name, State: "installed", Path: path}
		if err != nil || path == "" {
			status.State = "missing"
			status.Path = ""
		}
		statuses = append(statuses, status)
	}
	return statuses
}
