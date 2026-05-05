// Package actiontemplates is an embedded read-only registry of action
// blueprints (trigger + nodes + edges) shipped with the binary. Workspaces
// instantiate a template via ActionTemplateService.ApplyToWorkspace, which
// snapshot-copies it into the workspace's actions table. There is no DB
// table for templates and no admin CRUD UI in v1 — editing a template means
// editing a YAML file in this package and shipping a release.
package actiontemplates

import (
	"embed"
	"fmt"
	"path"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"windshift/internal/models"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// Template is the on-disk shape of an action blueprint. The Apply path
// converts these into models.Action / models.ActionNode / models.ActionEdge
// rows in a workspace.
type Template struct {
	Key           string                   `yaml:"key"`
	Name          string                   `yaml:"name"`
	Description   string                   `yaml:"description"`
	Category      string                   `yaml:"category"`
	TriggerType   models.ActionTriggerType `yaml:"trigger_type"`
	TriggerConfig map[string]any           `yaml:"trigger_config"`
	Nodes         []TemplateNode           `yaml:"nodes"`
	Edges         []TemplateEdge           `yaml:"edges"`
}

// TemplateNode is a single node in the blueprint graph. The yaml `id` is a
// human-readable string (e.g. "trigger", "close") that edges reference; it
// is *not* the persisted node ID — Apply assigns DB IDs at instantiation
// time and rewrites edges to use them.
type TemplateNode struct {
	ID         string         `yaml:"id"`
	NodeType   string         `yaml:"node_type"`
	NodeConfig map[string]any `yaml:"node_config"`
	Position   TemplatePos    `yaml:"position"`
}

// TemplatePos is a node's canvas position. Optional; defaults to 0,0.
type TemplatePos struct {
	X float64 `yaml:"x"`
	Y float64 `yaml:"y"`
}

// TemplateEdge connects two TemplateNodes by their yaml `id`.
type TemplateEdge struct {
	SourceNodeID string `yaml:"source_node_id"`
	TargetNodeID string `yaml:"target_node_id"`
	EdgeType     string `yaml:"edge_type"`
}

var (
	loadOnce  sync.Once
	loaded    []Template
	loadByKey map[string]Template
	loadErr   error
)

// Registry returns all embedded templates, parsed and validated. Cached
// after the first call. Never panics; templates with parse or validation
// errors are excluded and surface via Errors().
func Registry() []Template {
	load()
	return loaded
}

// Get returns the template with the given key, or false if not found.
func Get(key string) (Template, bool) {
	load()
	t, ok := loadByKey[key]
	return t, ok
}

// LoadError returns the first parse/validation error encountered (if any).
// Used by tests and the startup self-check; the API returns whatever
// templates parsed successfully and skips the rest.
func LoadError() error { return loadErr }

func load() {
	loadOnce.Do(func() {
		entries, err := templateFS.ReadDir("templates")
		if err != nil {
			loadErr = fmt.Errorf("read embedded template dir: %w", err)
			return
		}

		loadByKey = make(map[string]Template, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".yaml") {
				continue
			}
			raw, err := templateFS.ReadFile(path.Join("templates", name))
			if err != nil {
				loadErr = fmt.Errorf("read %s: %w", name, err)
				continue
			}
			var t Template
			if err := yaml.Unmarshal(raw, &t); err != nil {
				loadErr = fmt.Errorf("parse %s: %w", name, err)
				continue
			}
			if err := validate(&t); err != nil {
				loadErr = fmt.Errorf("validate %s: %w", name, err)
				continue
			}
			if _, dup := loadByKey[t.Key]; dup {
				loadErr = fmt.Errorf("duplicate template key %q (in %s)", t.Key, name)
				continue
			}
			loadByKey[t.Key] = t
			loaded = append(loaded, t)
		}
	})
}

// validate enforces the v1 invariants: every edge references defined nodes;
// no edge cycles; iterator bodies don't re-converge with non-body nodes.
// The runtime trusts this so it doesn't have to re-validate per execution.
func validate(t *Template) error {
	if t.Key == "" {
		return fmt.Errorf("template missing key")
	}
	if t.Name == "" {
		return fmt.Errorf("template %q missing name", t.Key)
	}
	if len(t.Nodes) == 0 {
		return fmt.Errorf("template %q has no nodes", t.Key)
	}

	nodeIDs := make(map[string]string, len(t.Nodes)) // id -> node_type
	for _, n := range t.Nodes {
		if n.ID == "" {
			return fmt.Errorf("template %q has a node with empty id", t.Key)
		}
		if _, dup := nodeIDs[n.ID]; dup {
			return fmt.Errorf("template %q has duplicate node id %q", t.Key, n.ID)
		}
		nodeIDs[n.ID] = n.NodeType
	}

	for _, e := range t.Edges {
		if _, ok := nodeIDs[e.SourceNodeID]; !ok {
			return fmt.Errorf("template %q edge references unknown source node %q", t.Key, e.SourceNodeID)
		}
		if _, ok := nodeIDs[e.TargetNodeID]; !ok {
			return fmt.Errorf("template %q edge references unknown target node %q", t.Key, e.TargetNodeID)
		}
	}

	// Iterator body invariant: every node downstream of an iterator must have
	// all its incoming edges originating from iterator-body nodes (the
	// iterator itself, or another body node). Nodes outside the body must
	// not point into the body.
	for _, n := range t.Nodes {
		nodeType := models.ActionNodeType(n.NodeType)
		if !nodeType.IsIterator() {
			continue
		}
		body := bodyClosure(n.ID, t.Edges)
		for _, e := range t.Edges {
			if !body[e.TargetNodeID] {
				continue
			}
			// Target is in the body. Source must also be in the body, OR be
			// the iterator itself.
			if e.SourceNodeID == n.ID {
				continue
			}
			if !body[e.SourceNodeID] {
				return fmt.Errorf("template %q: iterator %q body node %q has incoming edge from outside-body node %q", t.Key, n.ID, e.TargetNodeID, e.SourceNodeID)
			}
		}
	}

	return nil
}

// bodyClosure mirrors action_engine_iterator.iteratorBodyNodes but operates
// on the YAML graph (string IDs) so we can validate before persisting.
func bodyClosure(iteratorID string, edges []TemplateEdge) map[string]bool {
	body := map[string]bool{}
	queue := []string{}
	for _, e := range edges {
		if e.SourceNodeID == iteratorID && !body[e.TargetNodeID] {
			body[e.TargetNodeID] = true
			queue = append(queue, e.TargetNodeID)
		}
	}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, e := range edges {
			if e.SourceNodeID == next && !body[e.TargetNodeID] {
				body[e.TargetNodeID] = true
				queue = append(queue, e.TargetNodeID)
			}
		}
	}
	return body
}
