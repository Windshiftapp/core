package agentskills

import _ "embed"

// TestCaseStandardBody is the portable workspace skill body.
//
//go:embed packs/test-case-standard/SKILL.md
var TestCaseStandardBody string

// TestCaseStandardPage is the starter standard to publish as a workspace page.
//
//go:embed packs/test-case-standard/standard.md
var TestCaseStandardPage string

// TestCaseStandardSetup describes binding setup and a reproducible worked example.
//
//go:embed packs/test-case-standard/setup.md
var TestCaseStandardSetup string
