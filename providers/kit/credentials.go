package kit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/millerscout/qrtx/tracker"
	"gopkg.in/yaml.v3"
)

// Binding is one host's entry in the user-level credentials file. It names
// where the secret comes from, never the secret itself, and which repos or
// projects (scopes) the credential may be used for.
type Binding struct {
	TokenEnv     string   `yaml:"token_env,omitempty"`
	EmailEnv     string   `yaml:"email_env,omitempty"`
	TokenCommand string   `yaml:"token_command,omitempty"`
	Scopes       []string `yaml:"scopes,omitempty"`
}

// Creds is a resolved credential for one request.
type Creds struct {
	Token string
	Email string
}

// CredentialsPath is $QRTX_CREDENTIALS, else <user config dir>/qrtx/credentials.yaml
// ($XDG_CONFIG_HOME or ~/.config on Unix, %AppData% on Windows). When that
// file is missing but one exists under the tool's old name (<dir>/sdd/), the
// old file is used, so credentials bound before the rename keep working.
func CredentialsPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("QRTX_CREDENTIALS")); p != "" {
		return p, nil
	}
	var dir string
	switch x := os.Getenv("XDG_CONFIG_HOME"); {
	case runtime.GOOS == "windows":
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = d
	case x != "":
		dir = x
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	path := filepath.Join(dir, "qrtx", "credentials.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		legacy := filepath.Join(dir, "sdd", "credentials.yaml")
		if _, err := os.Stat(legacy); err == nil {
			return legacy, nil
		}
	}
	return path, nil
}

var bindingKeys = map[string]bool{"token_env": true, "email_env": true, "token_command": true, "scopes": true}
var secretKeys = map[string]bool{"token": true, "password": true, "secret": true, "api_token": true, "api_key": true}

// LoadBindings reads the credentials file. A missing file has no bindings.
func LoadBindings(path string) (map[string]Binding, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Binding{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Hosts map[string]map[string]any `yaml:"hosts"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	out := map[string]Binding{}
	for host, fields := range doc.Hosts {
		for k := range fields {
			if secretKeys[k] {
				return nil, fmt.Errorf("%s: host %s has %q: literal tokens are not allowed; use token_env or token_command", path, host, k)
			}
			if !bindingKeys[k] {
				return nil, fmt.Errorf("%s: host %s has unknown key %q", path, host, k)
			}
		}
		b, _ := yaml.Marshal(fields)
		var bind Binding
		if err := yaml.Unmarshal(b, &bind); err != nil {
			return nil, fmt.Errorf("%s: host %s: %v", path, host, err)
		}
		out[strings.ToLower(host)] = bind
	}
	return out, nil
}

func authErr(format string, a ...any) error {
	return &tracker.AdapterError{Code: tracker.ErrCodeAuth, Message: fmt.Sprintf(format, a...)}
}

func scopeAllowed(b Binding, scope string) bool {
	for _, s := range b.Scopes {
		if s == "*" || strings.EqualFold(s, scope) {
			return true
		}
	}
	return false
}

// Credentials resolves the credential bound to hostport (exact, port
// included when the URL has one) for scope (a repo or project). An unbound
// host, a scope outside the binding's scopes, or an empty source is an auth
// error naming what is missing and the file.
func Credentials(hostport, scope string) (Creds, error) {
	path, err := CredentialsPath()
	if err != nil {
		return Creds{}, authErr("cannot locate the credentials file: %v", err)
	}
	binds, err := LoadBindings(path)
	if err != nil {
		return Creds{}, authErr("%v", err)
	}
	host := strings.ToLower(hostport)
	b, ok := binds[host]
	if !ok {
		return Creds{}, authErr("host %s is not bound in %s; run `qrtx tracker setup` or add hosts.%s there", host, path, host)
	}
	if !scopeAllowed(b, scope) {
		return Creds{}, authErr("%s is not in the scopes bound to host %s in %s", scope, host, path)
	}
	var c Creds
	switch {
	case b.TokenEnv != "":
		c.Token = strings.TrimSpace(os.Getenv(b.TokenEnv))
		if c.Token == "" {
			return Creds{}, authErr("environment variable %s (bound to host %s in %s) is empty", b.TokenEnv, host, path)
		}
	case b.TokenCommand != "":
		tok, err := runTokenCommand(b.TokenCommand, host, scope)
		if err != nil {
			return Creds{}, authErr("token_command for host %s in %s: %v", host, path, err)
		}
		c.Token = tok
	default:
		return Creds{}, authErr("host %s in %s has no token_env or token_command", host, path)
	}
	if b.EmailEnv != "" {
		c.Email = strings.TrimSpace(os.Getenv(b.EmailEnv))
		if c.Email == "" {
			return Creds{}, authErr("environment variable %s (bound to host %s in %s) is empty", b.EmailEnv, host, path)
		}
	}
	return c, nil
}

func runTokenCommand(command, host, scope string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Env = append(os.Environ(), "QRTX_CRED_HOST="+host, "QRTX_CRED_SCOPE="+scope)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(errb.String()))
	}
	tok := strings.TrimSpace(out.String())
	if tok == "" {
		return "", fmt.Errorf("printed no token")
	}
	return tok, nil
}

// BindHost adds or merges a binding for host in the credentials file at path.
// Scopes are merged; the sources are replaced. It returns changed=false and
// leaves the file untouched when nothing would change.
func BindHost(path, host string, b Binding) (bool, error) {
	if b.TokenEnv == "" && b.TokenCommand == "" {
		return false, fmt.Errorf("binding for %s needs token_env or token_command", host)
	}
	if len(b.Scopes) == 0 {
		return false, fmt.Errorf("binding for %s needs at least one scope", host)
	}
	binds, err := LoadBindings(path)
	if err != nil {
		return false, err
	}
	host = strings.ToLower(host)
	merged := b
	if have, ok := binds[host]; ok {
		set := map[string]bool{}
		var scopes []string
		for _, s := range append(append([]string{}, have.Scopes...), b.Scopes...) {
			if !set[s] {
				set[s] = true
				scopes = append(scopes, s)
			}
		}
		merged.Scopes = scopes
		if have.TokenEnv == merged.TokenEnv && have.EmailEnv == merged.EmailEnv && have.TokenCommand == merged.TokenCommand && strings.Join(have.Scopes, "\x00") == strings.Join(merged.Scopes, "\x00") {
			return false, nil
		}
	}
	binds[host] = merged
	hosts := make([]string, 0, len(binds))
	for h := range binds {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	var buf bytes.Buffer
	buf.WriteString("# qrtx tracker credentials: where each host's token comes from, never the token.\n# Written by `qrtx tracker setup`; see docs/providers.md.\nhosts:\n")
	for _, h := range hosts {
		one, _ := yaml.Marshal(map[string]Binding{h: binds[h]})
		for _, line := range strings.Split(strings.TrimRight(string(one), "\n"), "\n") {
			buf.WriteString("  " + line + "\n")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}
