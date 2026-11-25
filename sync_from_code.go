package sdd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SyncPackageFromCode updates contract export signatures from Go AST.
func SyncPackageFromCode(repoRoot, relPkg string) error {
	relPkg = strings.Trim(relPkg, "/")
	dir := filepath.Join(repoRoot, relPkg)
	contractPath := filepath.Join(dir, contractFileName)
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return err
	}
	contract, err := ParseContractMarkdown(string(raw), contractPath)
	if err != nil {
		return err
	}
	exports, err := loadPackageExports(dir)
	if err != nil {
		return err
	}
	byName := map[string]exportSymbol{}
	for _, e := range exports {
		byName[e.Name] = e
	}
	for i := range contract.Exports {
		name := contract.Exports[i].Name
		if got, ok := byName[name]; ok {
			contract.Exports[i].Signature = got.Signature
			if contract.Exports[i].Kind == "" {
				contract.Exports[i].Kind = got.Kind
			}
		}
	}
	updated, err := replaceSDDBlock(string(raw), contract)
	if err != nil {
		return err
	}
	return os.WriteFile(contractPath, []byte(updated), 0o644)
}

// SyncDriftedPackages syncs all packages with drift for repoRoot.
func SyncDriftedPackages(repoRoot string, packages []string) error {
	if len(packages) == 0 {
		var err error
		if packages, err = DefaultVerifyPackages(repoRoot); err != nil {
			return err
		}
	}
	var drifted []string
	for _, pkg := range packages {
		issues, err := VerifyPackage(repoRoot, pkg)
		if err != nil {
			return err
		}
		if len(issues) > 0 {
			drifted = append(drifted, pkg)
		}
	}
	for _, pkg := range drifted {
		if err := SyncPackageFromCode(repoRoot, pkg); err != nil {
			return err
		}
	}
	return nil
}

func replaceSDDBlock(markdown string, contract *Contract) (string, error) {
	block, err := extractSDDBlock(markdown)
	if err != nil {
		return "", err
	}
	_ = block
	const open = "```sdd"
	const close = "```"
	start := strings.Index(markdown, open)
	if start < 0 {
		return "", fmt.Errorf("missing block")
	}
	rest := markdown[start+len(open):]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	end := strings.Index(rest, close)
	if end < 0 {
		return "", fmt.Errorf("unclosed block")
	}
	yamlBody, err := marshalContractYAML(contract)
	if err != nil {
		return "", err
	}
	newBlock := open + "\n" + yamlBody + "\n" + close
	return markdown[:start] + newBlock + rest[end+len(close):], nil
}

func marshalContractYAML(c *Contract) (string, error) {
	var b strings.Builder
	b.WriteString("package: ")
	b.WriteString(c.Package)
	b.WriteString("\nexports:\n")
	for _, e := range c.Exports {
		b.WriteString("  - name: ")
		b.WriteString(e.Name)
		b.WriteString("\n    kind: ")
		b.WriteString(e.Kind)
		b.WriteString("\n    signature: ")
		b.WriteString(fmt.Sprintf("%q", e.Signature))
		b.WriteString("\n")
	}
	b.WriteString("scenarios:\n")
	for _, sc := range c.Scenarios {
		b.WriteString("  - id: ")
		b.WriteString(sc.ID)
		b.WriteString("\n    when: ")
		b.WriteString(fmt.Sprintf("%q", sc.When))
		b.WriteString("\n    then: ")
		b.WriteString(fmt.Sprintf("%q", sc.Then))
		b.WriteString("\n")
	}
	return b.String(), nil
}
