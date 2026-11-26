package sdd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SetTrackerConfig writes c as the `tracker:` block of .sdd.yaml, keeping the
// file's other keys and comments. It leaves the file untouched and returns
// changed=false when the block already holds the same values. Unstable until v1.
func SetTrackerConfig(repoRoot string, c TrackerConfig) (bool, error) {
	path := filepath.Join(repoRoot, RepoConfigFile)
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	block := map[string]any{"backend": c.Backend}
	if len(c.Config) > 0 {
		block["config"] = c.Config
	}
	if len(c.StatusMap) > 0 {
		block["status_map"] = c.StatusMap
	}
	if c.Mirror != "" {
		block["mirror"] = c.Mirror
	}
	if c.Comments != "" {
		block["comments"] = c.Comments
	}
	if c.Timeout > 0 {
		block["timeout"] = c.Timeout
	}
	var want yaml.Node
	if err := want.Encode(block); err != nil {
		return false, err
	}
	var doc yaml.Node
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return false, fmt.Errorf("%s: %w", RepoConfigFile, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return false, fmt.Errorf("%s: top level is not a mapping", RepoConfigFile)
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != "tracker" {
			continue
		}
		var have, next any
		_ = m.Content[i+1].Decode(&have)
		_ = want.Decode(&next)
		a, _ := yaml.Marshal(have)
		b, _ := yaml.Marshal(next)
		if bytes.Equal(a, b) {
			return false, nil
		}
		m.Content[i+1] = &want
		return true, writeYAMLDoc(path, &doc)
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tracker"}, &want)
	return true, writeYAMLDoc(path, &doc)
}

func writeYAMLDoc(path string, doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
