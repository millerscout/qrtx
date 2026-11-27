package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookSpec is one qrtx hook: the Claude Code event, its matcher (or ""), the
// qrtx arguments, and whether it runs async.
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

// InstallHooks merges the qrtx sync hooks into the Claude Code settings file
// at path, keeping every other key and hook. qrtxCmd is how the hooks call
// qrtx ("qrtx", or a quoted absolute path). Hooks that still call the tool by
// its old name, sdd, are replaced. It returns changed=false, leaving the file
// untouched, when every hook is already there.
func InstallHooks(path, qrtxCmd string) (bool, error) {
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
		command := qrtxCmd + " " + h.args
		groups, _ := hooks[h.event].([]any)
		if kept, removed := dropLegacy(groups, h.args); removed {
			groups = kept
			hooks[h.event] = groups
			changed = true
		}
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

// dropLegacy removes hooks whose command runs the old qrtx binary (bare, or a
// path ending in /sdd, quoted or not) with args, and any group left empty.
func dropLegacy(groups []any, args string) ([]any, bool) {
	removed := false
	var out []any
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		list, _ := gm["hooks"].([]any)
		if !ok || list == nil {
			out = append(out, g)
			continue
		}
		var keep []any
		for _, h := range list {
			hm, _ := h.(map[string]any)
			if cmd, _ := hm["command"].(string); isLegacyCommand(cmd, args) {
				removed = true
				continue
			}
			keep = append(keep, h)
		}
		if len(keep) == 0 && len(list) > 0 {
			continue
		}
		gm["hooks"] = keep
		out = append(out, gm)
	}
	return out, removed
}

func isLegacyCommand(cmd, args string) bool {
	prog, ok := strings.CutSuffix(cmd, " "+args)
	if !ok {
		return false
	}
	prog = strings.Trim(prog, `"`)
	return prog == "sdd" || filepath.Base(prog) == "sdd"
}

// HasSyncHooks reports whether the settings file at path holds a hook that
// runs a qrtx sync hook command, through qrtx or the old sdd binary (bare or
// by path, quoted or not). A missing file holds none.
func HasSyncHooks(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		return false, fmt.Errorf("%s: %v", path, err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	for _, groups := range hooks {
		list, _ := groups.([]any)
		for _, g := range list {
			gm, _ := g.(map[string]any)
			entries, _ := gm["hooks"].([]any)
			for _, h := range entries {
				hm, _ := h.(map[string]any)
				if cmd, _ := hm["command"].(string); isSyncCommand(cmd) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func isSyncCommand(cmd string) bool {
	for _, h := range syncHooks {
		prog, ok := strings.CutSuffix(cmd, " "+h.args)
		if !ok {
			continue
		}
		base := filepath.Base(strings.Trim(prog, `"`))
		if base == "qrtx" || base == "qrtx.exe" || base == "sdd" {
			return true
		}
	}
	return false
}
