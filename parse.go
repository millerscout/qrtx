package sdd

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const contractFileName = "contract.sdd.md"

// Contract holds machine-readable package contract from contract.sdd.md.
type Contract struct {
	Package   string             `yaml:"package"`
	Exports   []ContractExport   `yaml:"exports"`
	Scenarios []ContractScenario `yaml:"scenarios"`
}

type ContractExport struct {
	Name      string `yaml:"name"`
	Kind      string `yaml:"kind"`
	Signature string `yaml:"signature"`
}

type ContractScenario struct {
	ID   string `yaml:"id"`
	When string `yaml:"when"`
	Then string `yaml:"then"`
}

// ParseContractFile reads and parses contract.sdd.md at path.
func ParseContractFile(path string) (*Contract, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseContractMarkdown(string(raw), path)
}

// ParseContractMarkdown extracts the ```sdd fenced YAML block.
func ParseContractMarkdown(markdown string, pathForErr string) (*Contract, error) {
	block, err := extractSDDBlock(markdown)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pathForErr, err)
	}
	var c Contract
	if err := yaml.Unmarshal([]byte(block), &c); err != nil {
		return nil, fmt.Errorf("%s: invalid contract yaml: %w", pathForErr, err)
	}
	if strings.TrimSpace(c.Package) == "" {
		return nil, fmt.Errorf("%s: contract missing package", pathForErr)
	}
	if len(c.Exports) == 0 {
		return nil, fmt.Errorf("%s: contract missing exports", pathForErr)
	}
	if len(c.Scenarios) == 0 {
		return nil, fmt.Errorf("%s: contract missing scenarios", pathForErr)
	}
	for _, sc := range c.Scenarios {
		if strings.TrimSpace(sc.When) == "" || strings.TrimSpace(sc.Then) == "" {
			return nil, fmt.Errorf("%s: scenario %q missing when/then", pathForErr, sc.ID)
		}
	}
	return &c, nil
}

func extractSDDBlock(markdown string) (string, error) {
	const open = "```sdd"
	const close = "```"
	start := strings.Index(markdown, open)
	if start < 0 {
		return "", fmt.Errorf("missing ```sdd contract block")
	}
	rest := markdown[start+len(open):]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	}
	end := strings.Index(rest, close)
	if end < 0 {
		return "", fmt.Errorf("unclosed ```sdd block")
	}
	block := strings.TrimSpace(rest[:end])
	if block == "" {
		return "", fmt.Errorf("empty ```sdd block")
	}
	return block, nil
}
