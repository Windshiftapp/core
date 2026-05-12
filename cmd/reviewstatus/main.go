package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	markerRe = regexp.MustCompile(`//\s*last review:\s*ser(?:\s*,\s*(\d{6}))?(?:\s*,\s*([A-Z][A-Z0-9_]*)\s*:\s*([^\r\n]*))?`)

	skipDirs = map[string]struct{}{
		"vendor":       {},
		"node_modules": {},
		"dist":         {},
		"bin":          {},
		"frontend":     {},
		"data":         {},
		"deploy":       {},
		"ethereal":     {},
		"logbook":      {},
		"docs":         {},
		"plugins":      {},
		"plugin-src":   {},
	}

	tagSeverity = map[string]int{"FIXME": 0, "TODO": 1, "NOTE": 2}
	tagClass    = map[string]string{"FIXME": "danger", "TODO": "warning", "NOTE": "info"}
)

func classifyTag(tag string) (class string, severity int) {
	if c, ok := tagClass[tag]; ok {
		return c, tagSeverity[tag]
	}
	return "generic", 3
}

type annotation struct {
	Line int
	Tag  string
	Note string
}

type parseError struct {
	Line int
	Raw  string
}

type fileResult struct {
	Path        string
	Lines       int
	Markers     int
	Oldest      time.Time
	Newest      time.Time
	Annotations []annotation
	TagCounts   map[string]int
	ParseErrors []parseError
}

func parseDDMMYY(s string) (time.Time, error) {
	if len(s) != 6 {
		return time.Time{}, fmt.Errorf("expected 6 digits")
	}
	dd, e1 := strconv.Atoi(s[0:2])
	mm, e2 := strconv.Atoi(s[2:4])
	yy, e3 := strconv.Atoi(s[4:6])
	if e1 != nil || e2 != nil || e3 != nil {
		return time.Time{}, fmt.Errorf("non-digit")
	}
	if mm < 1 || mm > 12 || dd < 1 || dd > 31 {
		return time.Time{}, fmt.Errorf("out of range")
	}
	t := time.Date(2000+yy, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	if t.Day() != dd || int(t.Month()) != mm {
		return time.Time{}, fmt.Errorf("invalid day/month combination")
	}
	return t, nil
}

func scanFile(absPath, relPath string) fileResult {
	// #nosec G304 -- dev CLI binary, not a network-exposed handler; the person running it is already the trust boundary for the local filesystem
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fileResult{Path: relPath, TagCounts: map[string]int{}}
	}
	lines := strings.Split(string(data), "\n")
	res := fileResult{
		Path:      relPath,
		Lines:     len(lines),
		TagCounts: map[string]int{},
	}
	for i, line := range lines {
		m := markerRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		res.Markers++
		rawDate := m[1]
		if rawDate == "" {
			res.ParseErrors = append(res.ParseErrors, parseError{Line: i + 1, Raw: "(missing date)"})
		} else {
			t, perr := parseDDMMYY(rawDate)
			if perr != nil {
				res.ParseErrors = append(res.ParseErrors, parseError{Line: i + 1, Raw: rawDate})
			} else {
				if res.Oldest.IsZero() || t.Before(res.Oldest) {
					res.Oldest = t
				}
				if t.After(res.Newest) {
					res.Newest = t
				}
			}
		}
		if tag := m[2]; tag != "" {
			res.Annotations = append(res.Annotations, annotation{
				Line: i + 1,
				Tag:  tag,
				Note: strings.TrimSpace(m[3]),
			})
			res.TagCounts[tag]++
		}
	}
	return res
}

func walk(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path == root {
				return nil
			}
			if _, skip := skipDirs[name]; skip {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

type annotationRow struct {
	Tag      string
	Class    string
	Path     string
	Line     int
	Note     string
	Severity int
}

type tagBadge struct {
	Tag   string
	Class string
	Count int
}

type reviewedRow struct {
	Path       string
	Oldest     string
	Newest     string
	DaysOld    int
	Markers    int
	Tags       []tagBadge
	StaleClass string
}

type unreviewedRow struct {
	Path  string
	Lines int
}

type parseErrorRow struct {
	Path string
	Line int
	Raw  string
}

type report struct {
	GeneratedAt string
	Total       int
	Reviewed    int
	Unreviewed  int
	Coverage    string
	FixmeCount  int
	TodoCount   int
	NoteCount   int
	OtherCount  int
	Annotations []annotationRow
	Unreviewed2 []unreviewedRow
	ReviewedL   []reviewedRow
	ParseErrors []parseErrorRow
}

func staleClass(days int) string {
	switch {
	case days <= 30:
		return "fresh"
	case days <= 90:
		return "aging"
	default:
		return "stale"
	}
}

func buildReport(results []fileResult, now time.Time) report {
	rep := report{GeneratedAt: now.Format("2006-01-02 15:04 MST")}

	for _, r := range results {
		rep.Total++
		reviewed := !r.Oldest.IsZero()
		if reviewed {
			rep.Reviewed++
			days := int(now.Sub(r.Oldest).Hours() / 24)
			var tags []tagBadge
			for _, t := range []string{"FIXME", "TODO", "NOTE"} {
				if c := r.TagCounts[t]; c > 0 {
					tags = append(tags, tagBadge{Tag: t, Class: tagClass[t], Count: c})
				}
			}
			var otherKeys []string
			for k := range r.TagCounts {
				if _, known := tagClass[k]; !known {
					otherKeys = append(otherKeys, k)
				}
			}
			sort.Strings(otherKeys)
			for _, k := range otherKeys {
				tags = append(tags, tagBadge{Tag: k, Class: "generic", Count: r.TagCounts[k]})
			}
			rep.ReviewedL = append(rep.ReviewedL, reviewedRow{
				Path:       r.Path,
				Oldest:     r.Oldest.Format("2006-01-02"),
				Newest:     r.Newest.Format("2006-01-02"),
				DaysOld:    days,
				Markers:    r.Markers,
				Tags:       tags,
				StaleClass: staleClass(days),
			})
		} else {
			rep.Unreviewed++
			rep.Unreviewed2 = append(rep.Unreviewed2, unreviewedRow{Path: r.Path, Lines: r.Lines})
		}

		for _, a := range r.Annotations {
			class, severity := classifyTag(a.Tag)
			rep.Annotations = append(rep.Annotations, annotationRow{
				Tag:      a.Tag,
				Class:    class,
				Path:     r.Path,
				Line:     a.Line,
				Note:     a.Note,
				Severity: severity,
			})
		}
		for _, pe := range r.ParseErrors {
			rep.ParseErrors = append(rep.ParseErrors, parseErrorRow{Path: r.Path, Line: pe.Line, Raw: pe.Raw})
		}

		rep.FixmeCount += r.TagCounts["FIXME"]
		rep.TodoCount += r.TagCounts["TODO"]
		rep.NoteCount += r.TagCounts["NOTE"]
		for k, c := range r.TagCounts {
			if _, known := tagClass[k]; !known {
				rep.OtherCount += c
			}
		}
	}

	if rep.Total > 0 {
		rep.Coverage = fmt.Sprintf("%.1f%%", float64(rep.Reviewed)*100/float64(rep.Total))
	} else {
		rep.Coverage = "0.0%"
	}

	sort.Slice(rep.Annotations, func(i, j int) bool {
		a, b := rep.Annotations[i], rep.Annotations[j]
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	sort.Slice(rep.Unreviewed2, func(i, j int) bool { return rep.Unreviewed2[i].Path < rep.Unreviewed2[j].Path })
	sort.Slice(rep.ReviewedL, func(i, j int) bool {
		a, b := rep.ReviewedL[i], rep.ReviewedL[j]
		if a.Oldest != b.Oldest {
			return a.Oldest < b.Oldest
		}
		return a.Path < b.Path
	})
	sort.Slice(rep.ParseErrors, func(i, j int) bool {
		a, b := rep.ParseErrors[i], rep.ParseErrors[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	return rep
}

const tmplSrc = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Review Status</title>
<style>
:root {
  --ds-surface:          #fafbfc;
  --ds-surface-raised:   #ffffff;
  --ds-surface-hovered:  #f5f6f7;
  --ds-surface-selected: #e3f2fd;
  --ds-text:             #172b4d;
  --ds-text-subtle:      #6b778c;
  --ds-text-link:        #0052cc;
  --ds-interactive:      #2874bb;
  --ds-success:          #16a34a;
  --ds-warning:          #ca8a04;
  --ds-danger:           #dc2626;
  --ds-border:           #dfe1e6;

  --ds-status-success-bg: #dcfce7; --ds-status-success-fg: #166534; --ds-status-success-bd: #86efac;
  --ds-status-warning-bg: #fef3c7; --ds-status-warning-fg: #92400e; --ds-status-warning-bd: #fcd34d;
  --ds-status-danger-bg:  #fee2e2; --ds-status-danger-fg:  #991b1b; --ds-status-danger-bd:  #fca5a5;
  --ds-status-info-bg:    #dbeafe; --ds-status-info-fg:    #1e40af; --ds-status-info-bd:    #93c5fd;

  --radius-md: 4px; --radius-lg: 6px; --radius-xl: 8px;
  --space-1: 4px; --space-2: 8px; --space-3: 12px; --space-4: 16px; --space-6: 24px; --space-8: 32px;

  --font-sans: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  --font-mono: Monaco, Menlo, "Ubuntu Mono", monospace;
}
@media (prefers-color-scheme: dark) {
  :root {
    --ds-surface:        #1d2125;
    --ds-surface-raised: #22272b;
    --ds-surface-hovered:#2c333a;
    --ds-text:           #b6c2cf;
    --ds-text-subtle:    #8b98a9;
    --ds-text-link:      #579dff;
    --ds-interactive:    #3b82f6;
    --ds-border:         #2c333a;
    --ds-status-info-bg:    #1e3a5f; --ds-status-info-fg:    #93c5fd; --ds-status-info-bd:    #2e5a8a;
    --ds-status-warning-bg: #422006; --ds-status-warning-fg: #fcd34d; --ds-status-warning-bd: #713f12;
    --ds-status-danger-bg:  #450a0a; --ds-status-danger-fg:  #fca5a5; --ds-status-danger-bd:  #7f1d1d;
    --ds-status-success-bg: #052e16; --ds-status-success-fg: #86efac; --ds-status-success-bd: #166534;
  }
}
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; }
body {
  font-family: var(--font-sans);
  font-size: 13px;
  line-height: 1.5;
  color: var(--ds-text);
  background: var(--ds-surface);
}
.container { max-width: 1280px; margin: 0 auto; padding: var(--space-8) var(--space-6); }
h1 { font-size: 24px; font-weight: 600; margin: 0 0 var(--space-2) 0; letter-spacing: -0.01em; }
h2 { font-size: 16px; font-weight: 600; margin: var(--space-8) 0 var(--space-3) 0; letter-spacing: -0.005em; }
.meta { color: var(--ds-text-subtle); margin-bottom: var(--space-6); }

.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: var(--space-3); margin-bottom: var(--space-6); }
.card {
  background: var(--ds-surface-raised);
  border: 1px solid var(--ds-border);
  border-radius: var(--radius-lg);
  padding: var(--space-4);
}
.card .label { color: var(--ds-text-subtle); font-size: 12px; text-transform: uppercase; letter-spacing: 0.04em; font-weight: 500; }
.card .value { font-size: 24px; font-weight: 600; margin-top: var(--space-1); }
.card.accent .value { color: var(--ds-interactive); }
.card.info .value    { color: var(--ds-status-info-fg); }
.card.warning .value { color: var(--ds-status-warning-fg); }
.card.danger .value  { color: var(--ds-status-danger-fg); }
.card.success .value { color: var(--ds-success); }

table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  background: var(--ds-surface-raised);
  border: 1px solid var(--ds-border);
  border-radius: var(--radius-lg);
  overflow: hidden;
}
thead th {
  position: sticky; top: 0;
  background: var(--ds-surface-hovered);
  text-align: left;
  font-weight: 600;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ds-text-subtle);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--ds-border);
}
tbody td {
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--ds-border);
  vertical-align: top;
}
tbody tr:last-child td { border-bottom: none; }
tbody tr:hover { background: var(--ds-surface-hovered); }
.mono { font-family: var(--font-mono); font-size: 12px; }
.subtle { color: var(--ds-text-subtle); }
.num { text-align: right; font-variant-numeric: tabular-nums; }

.tag {
  display: inline-block;
  font-family: var(--font-sans);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  padding: 2px var(--space-2);
  border-radius: var(--radius-md);
  border: 1px solid transparent;
  margin-right: var(--space-1);
}
.tag--info    { background: var(--ds-status-info-bg);    color: var(--ds-status-info-fg);    border-color: var(--ds-status-info-bd); }
.tag--warning { background: var(--ds-status-warning-bg); color: var(--ds-status-warning-fg); border-color: var(--ds-status-warning-bd); }
.tag--danger  { background: var(--ds-status-danger-bg);  color: var(--ds-status-danger-fg);  border-color: var(--ds-status-danger-bd); }
.tag--generic { background: var(--ds-surface-hovered);   color: var(--ds-text-subtle);       border-color: var(--ds-border); }
.tag .count { opacity: 0.75; margin-left: 4px; font-weight: 500; }

tr.row-fresh td:first-child { border-left: 3px solid var(--ds-success); }
tr.row-aging td:first-child { border-left: 3px solid var(--ds-warning); }
tr.row-stale td:first-child { border-left: 3px solid var(--ds-danger); }
tr.row-unreviewed td:first-child { border-left: 3px solid var(--ds-danger); }

.empty { padding: var(--space-6); text-align: center; color: var(--ds-text-subtle); background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-radius: var(--radius-lg); }

.note-text { white-space: pre-wrap; word-break: break-word; }
.file-link { color: var(--ds-text-link); text-decoration: none; }
.file-link:hover { text-decoration: underline; }
</style>
</head>
<body>
<div class="container">
  <h1>Review Status</h1>
  <div class="meta">Generated {{.GeneratedAt}}</div>

  <div class="cards">
    <div class="card"><div class="label">Total files</div><div class="value">{{.Total}}</div></div>
    <div class="card success"><div class="label">Reviewed</div><div class="value">{{.Reviewed}}</div></div>
    <div class="card danger"><div class="label">Unreviewed</div><div class="value">{{.Unreviewed}}</div></div>
    <div class="card accent"><div class="label">Coverage</div><div class="value">{{.Coverage}}</div></div>
    <div class="card danger"><div class="label">FIXMEs</div><div class="value">{{.FixmeCount}}</div></div>
    <div class="card warning"><div class="label">TODOs</div><div class="value">{{.TodoCount}}</div></div>
    <div class="card info"><div class="label">NOTEs</div><div class="value">{{.NoteCount}}</div></div>
    {{if .OtherCount}}<div class="card"><div class="label">Other tags</div><div class="value">{{.OtherCount}}</div></div>{{end}}
  </div>

  <h2>Annotations</h2>
  {{if .Annotations}}
  <table>
    <thead><tr><th style="width:90px">Tag</th><th>Location</th><th>Note</th></tr></thead>
    <tbody>
    {{range .Annotations}}
      <tr>
        <td><span class="tag tag--{{.Class}}">{{.Tag}}</span></td>
        <td class="mono">{{.Path}}:{{.Line}}</td>
        <td class="note-text">{{.Note}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}
  <div class="empty">No annotations yet.</div>
  {{end}}

  <h2>Unreviewed files <span class="subtle">({{.Unreviewed}})</span></h2>
  {{if .Unreviewed2}}
  <table>
    <thead><tr><th>Path</th><th class="num" style="width:100px">Lines</th></tr></thead>
    <tbody>
    {{range .Unreviewed2}}
      <tr class="row-unreviewed">
        <td class="mono">{{.Path}}</td>
        <td class="num subtle">{{.Lines}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}
  <div class="empty">Everything is reviewed. Nice work.</div>
  {{end}}

  <h2>Reviewed files <span class="subtle">({{.Reviewed}})</span></h2>
  {{if .ReviewedL}}
  <table>
    <thead><tr>
      <th>Path</th>
      <th style="width:120px">Oldest</th>
      <th style="width:120px">Newest</th>
      <th class="num" style="width:80px">Days</th>
      <th class="num" style="width:90px">Markers</th>
      <th style="width:200px">Tags</th>
    </tr></thead>
    <tbody>
    {{range .ReviewedL}}
      <tr class="row-{{.StaleClass}}">
        <td class="mono">{{.Path}}</td>
        <td class="subtle">{{.Oldest}}</td>
        <td class="subtle">{{.Newest}}</td>
        <td class="num subtle">{{.DaysOld}}</td>
        <td class="num">{{.Markers}}</td>
        <td>
          {{range .Tags}}<span class="tag tag--{{.Class}}">{{.Tag}}<span class="count">×{{.Count}}</span></span>{{end}}
        </td>
      </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}
  <div class="empty">No reviewed files yet.</div>
  {{end}}

  {{if .ParseErrors}}
  <h2>Parse errors</h2>
  <table>
    <thead><tr><th>Location</th><th>Raw date</th></tr></thead>
    <tbody>
    {{range .ParseErrors}}
      <tr>
        <td class="mono">{{.Path}}:{{.Line}}</td>
        <td class="mono">{{.Raw}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
  {{end}}
</div>
</body>
</html>
`

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd: %v", err)
	}

	files, err := walk(root)
	if err != nil {
		log.Fatalf("walk: %v", err)
	}

	workers := runtime.NumCPU()
	in := make(chan string, workers)
	out := make(chan fileResult, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range in {
				out <- scanFile(filepath.Join(root, rel), rel)
			}
		}()
	}
	go func() {
		for _, f := range files {
			in <- f
		}
		close(in)
		wg.Wait()
		close(out)
	}()

	results := make([]fileResult, 0, len(files))
	for r := range out {
		results = append(results, r)
	}

	rep := buildReport(results, time.Now())

	tmpl, err := template.New("report").Parse(tmplSrc)
	if err != nil {
		log.Fatalf("template parse: %v", err)
	}

	outPath := filepath.Join(root, "docs", "review_status.html")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		log.Fatalf("mkdir docs: %v", err)
	}
	// #nosec G304 -- dev CLI binary, not a network-exposed handler; the relative path is hardcoded and joined with the caller-supplied repo root
	f, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	if err := tmpl.Execute(f, rep); err != nil {
		f.Close()
		log.Fatalf("render: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("close output: %v", err)
	}

	fmt.Printf("reviewstatus: %d/%d files reviewed (%s), %d FIXMEs, %d TODOs, %d NOTEs → docs/review_status.html\n",
		rep.Reviewed, rep.Total, rep.Coverage, rep.FixmeCount, rep.TodoCount, rep.NoteCount)
}
