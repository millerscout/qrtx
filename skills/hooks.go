package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// hookSpec is one sdd hook: the Claude Code event, its matcher (or ""), the
// sdd arguments, and whether it runs async.
type hookSpec struct {
	event   string
	matcher string
	args    string
	async   bool
}

var syncHooks = []hookSpec{
	{event: "SessionStart", args: "sync begin --hook"},
	{event: "UserPromptSubmit", args: "sync heartbeat --hook --quiet"},
	{event: "UserPromptSubmit", args: "sync inbox --hook --brief"},
	{event: "PostToolUse", matcher: "*", args: "sync heartbeat --hook --quiet", async: true},
	{event: "SessionEnd", args: "sync end --hook"},
}

// InstallHooks merges the sdd sync hooks into the Claude Code settings file
// at path, keeping every other key and hook. sddCmd is how the hooks call
// sdd ("sdd", or a quoted absolute path). It returns changed=false, leaving
// the file untouched, when every hook is already there.
func InstallHooks(path, sddCmd string) (bool, error) {
	settings := map[string]any{}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return false, fmt.Errorf("%s: %v", path, err)
		}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	for _, h := range syncHooks {
		command := sddCmd + " " + h.args
		groups, _ := hooks[h.event].([]any)
		if hasCommand(groups, command) {
			continue
		}
		entry := map[string]any{"type": "command", "command": command}
		if h.async {
			entry["async"] = true
		}
		group := map[string]any{"hooks": []any{entry}}
		if h.matcher != "" {
			group["matcher"] = h.matcher
		}
		hooks[h.event] = append(groups, group)
		changed = true
	}
	if !changed {
		return false, nil
	}
	settings["hooks"] = hooks
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}

func hasCommand(groups []any, command string) bool {
	for _, g := range groups {
		gm, _ := g.(map[string]any)
		list, _ := gm["hooks"].([]any)
		for _, h := range list {
			hm, _ := h.(map[string]any)
			if hm["command"] == command {
				return true
			}
		}
	}
	return false
}
