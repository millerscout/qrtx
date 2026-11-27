// Command sdd manages spec-driven development changes under sdd/changes/.
package main

import (
	"fmt"
	"os"

	"github.com/millerscout/qrtx"
	"github.com/millerscout/qrtx/cli"
)

func main() {
	if root, err := qrtx.RepoRoot(); err == nil {
		qrtx.SetCodeIndex(qrtx.TextIndex{Root: root})
	}
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "qrtx: %v\n", err)
		os.Exit(1)
	}
}
