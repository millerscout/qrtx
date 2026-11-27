// Command qrtx-tracker-github is the qrtx tracker plugin for GitHub Issues.
// qrtx runs it as `qrtx-tracker-github <op>` with one sdd.adapter/v1 request
// on stdin. See docs/providers.md.
package main

import (
	"github.com/millerscout/qrtx/providers/github"
	"github.com/millerscout/qrtx/providers/kit"
)

func main() { kit.Main(kit.Tracker(&github.Provider{})) }
