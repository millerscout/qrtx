// Command sdd manages spec-driven development changes under sdd/changes/.
package main

import (
	"fmt"
	"os"

	"github.com/millerscout/sdd"
	"github.com/millerscout/sdd/cli"
)

func main() {
	if root, err := sdd.RepoRoot(); err == nil {
		sdd.SetCodeIndex(sdd.TextIndex{Root: root})
	}
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "sdd: %v\n", err)
		os.Exit(1)
	}
}
