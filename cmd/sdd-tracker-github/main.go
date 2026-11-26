// Command sdd-tracker-github is the sdd tracker plugin for GitHub Issues.
// sdd runs it as `sdd-tracker-github <op>` with one sdd.adapter/v1 request
// on stdin. See docs/providers.md.
package main

import (
	"github.com/millerscout/sdd/providers/github"
	"github.com/millerscout/sdd/providers/kit"
)

func main() { kit.Main(kit.Tracker(&github.Provider{})) }
