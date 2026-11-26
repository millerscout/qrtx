// Command sdd-tracker-jira is the sdd tracker plugin for Jira Cloud.
// sdd runs it as `sdd-tracker-jira <op>` with one sdd.adapter/v1 request on
// stdin. See docs/providers.md.
package main

import (
	"github.com/millerscout/sdd/providers/jira"
	"github.com/millerscout/sdd/providers/kit"
)

func main() { kit.Main(kit.Tracker(&jira.Provider{})) }
