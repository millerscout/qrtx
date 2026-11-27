// Command qrtx-tracker-jira is the qrtx tracker plugin for Jira Cloud.
// qrtx runs it as `qrtx-tracker-jira <op>` with one sdd.adapter/v1 request on
// stdin. See docs/providers.md.
package main

import (
	"github.com/millerscout/qrtx/providers/jira"
	"github.com/millerscout/qrtx/providers/kit"
)

func main() { kit.Main(kit.Tracker(&jira.Provider{})) }
