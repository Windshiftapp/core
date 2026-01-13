// Package main provides a test report generator that reads Go test JSON output
// and generates a self-contained HTML report.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"sort"
	"strings"
	"time"
)

// TestEvent represents a single event from go test -json output
type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
}

// TestResult holds the aggregated result for a single test
type TestResult struct {
	Name     string
	Status   string // pass, fail, skip
	Duration float64
	Output   []string
	Subtests []*TestResult
}

// PackageResult holds results for a package
type PackageResult struct {
	Name     string
	Status   string
	Duration float64
	Tests    map[string]*TestResult
}

// Report holds the complete test report data
type Report struct {
	Timestamp time.Time
	Packages  map[string]*PackageResult
	Passed    int
	Failed    int
	Skipped   int
	Total     int
	Duration  float64
}

func main() {
	report := &Report{
		Timestamp: time.Now(),
		Packages:  make(map[string]*PackageResult),
	}

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size for long output lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event TestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Skip non-JSON lines
		}

		processEvent(report, &event)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// Calculate totals
	calculateTotals(report)

	// Generate HTML
	generateHTML(report)
}

func processEvent(report *Report, event *TestEvent) {
	// Get or create package
	pkg, ok := report.Packages[event.Package]
	if !ok {
		pkg = &PackageResult{
			Name:  event.Package,
			Tests: make(map[string]*TestResult),
		}
		report.Packages[event.Package] = pkg
	}

	// Package-level event (no test name)
	if event.Test == "" {
		switch event.Action {
		case "pass", "fail", "skip":
			pkg.Status = event.Action
			pkg.Duration = event.Elapsed
		}
		return
	}

	// Parse test name to handle subtests (TestName/SubtestName)
	parts := strings.SplitN(event.Test, "/", 2)
	parentName := parts[0]

	// Get or create parent test
	parent, ok := pkg.Tests[parentName]
	if !ok {
		parent = &TestResult{
			Name:     parentName,
			Subtests: make([]*TestResult, 0),
		}
		pkg.Tests[parentName] = parent
	}

	// Determine which test to update
	var target *TestResult
	if len(parts) == 1 {
		// This is the parent test
		target = parent
	} else {
		// This is a subtest - find or create it
		subtestName := parts[1]
		found := false
		for _, st := range parent.Subtests {
			if st.Name == subtestName {
				target = st
				found = true
				break
			}
		}
		if !found {
			target = &TestResult{Name: subtestName}
			parent.Subtests = append(parent.Subtests, target)
		}
	}

	// Process the event
	switch event.Action {
	case "pass", "fail", "skip":
		target.Status = event.Action
		target.Duration = event.Elapsed
	case "output":
		target.Output = append(target.Output, event.Output)
	}
}

func calculateTotals(report *Report) {
	for _, pkg := range report.Packages {
		report.Duration += pkg.Duration

		for _, test := range pkg.Tests {
			// Count subtests if they exist, otherwise count the parent
			if len(test.Subtests) > 0 {
				for _, st := range test.Subtests {
					report.Total++
					switch st.Status {
					case "pass":
						report.Passed++
					case "fail":
						report.Failed++
					case "skip":
						report.Skipped++
					}
				}
			} else {
				report.Total++
				switch test.Status {
				case "pass":
					report.Passed++
				case "fail":
					report.Failed++
				case "skip":
					report.Skipped++
				}
			}
		}
	}
}

func generateHTML(report *Report) {
	fmt.Println(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Report</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.5;
            color: #333;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #fafafa;
        }
        header {
            background: #fff;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        }
        h1 { font-size: 24px; margin-bottom: 10px; }
        .timestamp { color: #666; font-size: 14px; margin-bottom: 15px; }
        .summary {
            display: flex;
            gap: 20px;
            flex-wrap: wrap;
        }
        .stat {
            padding: 8px 16px;
            border-radius: 4px;
            font-weight: 500;
        }
        .stat.passed { background: #d4edda; color: #155724; }
        .stat.failed { background: #f8d7da; color: #721c24; }
        .stat.skipped { background: #fff3cd; color: #856404; }
        .stat.duration { background: #e2e3e5; color: #383d41; }
        .stat.total { background: #cce5ff; color: #004085; }
        .package {
            background: #fff;
            border-radius: 8px;
            margin-bottom: 15px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .package-header {
            padding: 15px 20px;
            background: #f8f9fa;
            border-bottom: 1px solid #e9ecef;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .package-name { font-weight: 600; font-size: 16px; }
        .package-duration { color: #666; font-size: 14px; }
        .test {
            padding: 12px 20px;
            border-bottom: 1px solid #f0f0f0;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .test:last-child { border-bottom: none; }
        .test-info { display: flex; align-items: center; gap: 10px; }
        .test-name { font-family: monospace; font-size: 14px; }
        .test-duration { color: #666; font-size: 13px; }
        .badge {
            padding: 2px 8px;
            border-radius: 3px;
            font-size: 12px;
            font-weight: 500;
            text-transform: uppercase;
        }
        .badge.pass { background: #d4edda; color: #155724; }
        .badge.fail { background: #f8d7da; color: #721c24; }
        .badge.skip { background: #fff3cd; color: #856404; }
        .subtests {
            margin-left: 30px;
            border-left: 2px solid #e9ecef;
        }
        .subtest {
            padding: 8px 20px;
            border-bottom: 1px solid #f5f5f5;
            display: flex;
            justify-content: space-between;
            align-items: center;
            background: #fafafa;
        }
        .subtest:last-child { border-bottom: none; }
        .output {
            margin: 10px 20px 10px 50px;
            padding: 10px;
            background: #f8f9fa;
            border-radius: 4px;
            font-family: monospace;
            font-size: 12px;
            white-space: pre-wrap;
            word-break: break-all;
            color: #721c24;
            max-height: 300px;
            overflow-y: auto;
        }
        .collapsible {
            cursor: pointer;
        }
        .collapsible:hover {
            background: #f8f9fa;
        }
        .collapse-icon {
            margin-right: 8px;
            transition: transform 0.2s;
        }
        .collapsed .collapse-icon {
            transform: rotate(-90deg);
        }
        .collapsed + .subtests,
        .collapsed + .output {
            display: none;
        }
    </style>
</head>
<body>
    <header>
        <h1>Test Report</h1>`)

	fmt.Printf("        <div class=\"timestamp\">Generated: %s</div>\n", report.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Println(`        <div class="summary">`)
	fmt.Printf("            <span class=\"stat total\">%d total</span>\n", report.Total)
	fmt.Printf("            <span class=\"stat passed\">%d passed</span>\n", report.Passed)
	fmt.Printf("            <span class=\"stat failed\">%d failed</span>\n", report.Failed)
	fmt.Printf("            <span class=\"stat skipped\">%d skipped</span>\n", report.Skipped)
	fmt.Printf("            <span class=\"stat duration\">%.2fs</span>\n", report.Duration)
	fmt.Println(`        </div>
    </header>
    <main>`)

	// Sort packages for consistent output
	pkgNames := make([]string, 0, len(report.Packages))
	for name := range report.Packages {
		pkgNames = append(pkgNames, name)
	}
	sort.Strings(pkgNames)

	for _, pkgName := range pkgNames {
		pkg := report.Packages[pkgName]
		// Skip empty packages (no tests)
		if len(pkg.Tests) == 0 {
			continue
		}
		fmt.Println(`        <section class="package">`)
		fmt.Printf("            <div class=\"package-header\">\n")
		fmt.Printf("                <span class=\"package-name\">%s</span>\n", html.EscapeString(pkg.Name))
		fmt.Printf("                <span class=\"package-duration\">%.2fs</span>\n", pkg.Duration)
		fmt.Println(`            </div>`)

		// Sort tests for consistent output
		testNames := make([]string, 0, len(pkg.Tests))
		for name := range pkg.Tests {
			testNames = append(testNames, name)
		}
		sort.Strings(testNames)

		for _, testName := range testNames {
			test := pkg.Tests[testName]
			renderTest(test, false)
		}

		fmt.Println(`        </section>`)
	}

	fmt.Println(`    </main>
    <script>
        document.querySelectorAll('.collapsible').forEach(el => {
            el.addEventListener('click', () => {
                el.classList.toggle('collapsed');
            });
        });
    </script>
</body>
</html>`)
}

func renderTest(test *TestResult, isSubtest bool) {
	className := "test"
	if isSubtest {
		className = "subtest"
	}

	hasSubtests := len(test.Subtests) > 0
	hasFailed := test.Status == "fail"
	hasOutput := hasFailed && len(test.Output) > 0

	collapsible := ""
	collapseIcon := ""
	if hasSubtests || hasOutput {
		collapsible = " collapsible"
		collapseIcon = `<span class="collapse-icon">▼</span>`
	}

	fmt.Printf("            <div class=\"%s%s\">\n", className, collapsible)
	fmt.Printf("                <div class=\"test-info\">\n")
	fmt.Printf("                    %s<span class=\"test-name\">%s</span>\n", collapseIcon, html.EscapeString(test.Name))
	fmt.Printf("                </div>\n")
	fmt.Printf("                <div class=\"test-meta\">\n")
	if test.Duration > 0 {
		fmt.Printf("                    <span class=\"test-duration\">%.2fs</span>\n", test.Duration)
	}
	if test.Status != "" {
		fmt.Printf("                    <span class=\"badge %s\">%s</span>\n", test.Status, test.Status)
	}
	fmt.Printf("                </div>\n")
	fmt.Printf("            </div>\n")

	// Render failure output
	if hasOutput {
		fmt.Println(`            <div class="output">`)
		for _, line := range test.Output {
			// Only show meaningful output (skip test run/pass messages)
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "=== RUN") && !strings.HasPrefix(trimmed, "--- PASS") {
				fmt.Print(html.EscapeString(line))
			}
		}
		fmt.Println(`            </div>`)
	}

	// Render subtests
	if hasSubtests {
		fmt.Println(`            <div class="subtests">`)
		for _, st := range test.Subtests {
			renderTest(st, true)
		}
		fmt.Println(`            </div>`)
	}
}
