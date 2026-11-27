package qrtx

import (
	"path/filepath"
	"testing"
)

func TestParseContractMarkdown(t *testing.T) {
	md := "## Contract\n\n```sdd\npackage: demo\nexports:\n  - name: Foo\n    kind: func\n    signature: \"func Foo()\"\nscenarios:\n  - id: ok\n    when: x\n    then: y\n```\n"
	c, err := ParseContractMarkdown(md, "test.md")
	if err != nil {
		t.Fatal(err)
	}
	if c.Package != "demo" || len(c.Exports) != 1 {
		t.Fatalf("contract=%+v", c)
	}
}

func TestParseContractMarkdownMissingBlock(t *testing.T) {
	_, err := ParseContractMarkdown("# hi\n", "x.md")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseContractFileFixture(t *testing.T) {
	path := filepath.Join("testdata", "pkgok", "contract.sdd.md")
	c, err := ParseContractFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Package != "pkgok" {
		t.Fatalf("package=%s", c.Package)
	}
}
