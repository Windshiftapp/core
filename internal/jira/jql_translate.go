package jira

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	jqlOrderByPattern      = regexp.MustCompile(`(?i)\s+ORDER\s+BY\s+.+$`)
	jqlSimpleClausePattern = regexp.MustCompile(`(?is)^\s*([A-Za-z][A-Za-z0-9_ -]*?)\s*(=|!=|<>|~|NOT\s+IN|IN)\s*(.+?)\s*$`)
	jqlOuterParensPattern  = regexp.MustCompile(`^\((.*)\)$`)
	jqlWhitespacePattern   = regexp.MustCompile(`\s+`)
	jqlIdentifierSafe      = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
)

// JQLTranslation is the outcome of translating a Jira JQL expression. QL is the
// Windshift selector; Unsupported lists clauses that could not be translated.
// MembershipComplete is false when an unsupported clause changes which issues
// the expression matches, so callers must not execute QL as the full predicate.
// An unsupported ORDER BY alone leaves membership complete.
type JQLTranslation struct {
	QL                 string
	Unsupported        []string
	MembershipComplete bool
}

// TranslateJQLToWindshiftQL translates a Jira JQL expression into Windshift QL
// scoped to workspaceID. Clauses that cannot be translated are returned in
// unsupported; callers must preserve the original JQL and keep the imported
// selector inactive rather than widening it.
func TranslateJQLToWindshiftQL(jql string, workspaceID int) (ql string, unsupported []string) {
	translation := TranslateJQL(jql, workspaceID)
	return translation.QL, translation.Unsupported
}

// TranslateJQL translates a Jira JQL expression and reports whether the
// translated selector preserves the source predicate tree.
func TranslateJQL(jql string, workspaceID int) JQLTranslation {
	result := JQLTranslation{
		QL:                 fmt.Sprintf("workspace_id = %d", workspaceID),
		MembershipComplete: true,
	}
	jql = strings.TrimSpace(jql)
	if jql == "" {
		return result
	}
	if match := jqlOrderByPattern.FindString(jql); match != "" {
		result.Unsupported = append(result.Unsupported, strings.TrimSpace(match))
		jql = strings.TrimSpace(jqlOrderByPattern.ReplaceAllString(jql, ""))
	}
	if jql == "" {
		return result
	}
	expr, err := parseJQLExpression(jql)
	if err != nil {
		// Preserve the whole expression when the grammar is not understood;
		// callers surface this as an unsupported clause rather than guessing.
		result.Unsupported = append(result.Unsupported, jql)
		result.MembershipComplete = false
		return result
	}
	translated, clauseUnsupported := translateJQLBoolean(expr)
	result.Unsupported = append(result.Unsupported, clauseUnsupported...)
	if translated == "" {
		if len(clauseUnsupported) > 0 {
			result.MembershipComplete = false
		}
		return result
	}
	result.QL = result.QL + " AND (" + translated + ")"
	return result
}

// UnsupportedJQLSelector returns a workspace-scoped selector that matches
// nothing. Importers use it when a Jira predicate tree could not be fully
// translated, keeping the imported collection non-executable instead of
// widening it to the whole workspace.
func UnsupportedJQLSelector(workspaceID int) string {
	return fmt.Sprintf("workspace_id = %d AND 1 = 0", workspaceID)
}

// JQLProjectScope summarizes the project predicate of a JQL expression. A
// saved filter's project clause decides which workspace it belongs to, but the
// clause itself is represented by workspace_id in the emitted QL, so callers
// must reason about the predicate separately from translation.
type JQLProjectScope struct {
	// HasClause is true when the expression names at least one project.
	HasClause bool
	// Include holds project keys the predicate requires (project = / IN).
	Include []string
	// Exclude holds project keys the predicate rejects (project != / NOT IN).
	Exclude []string
	// Analyzed is false when the expression or a project clause could not be
	// parsed. Callers must treat the scope as unknown rather than absent.
	Analyzed bool
}

// AnalyzeJQLProjectScope extracts the project predicate from a JQL expression.
func AnalyzeJQLProjectScope(jql string) JQLProjectScope {
	scope := JQLProjectScope{Analyzed: true}
	jql = strings.TrimSpace(jqlOrderByPattern.ReplaceAllString(strings.TrimSpace(jql), ""))
	if jql == "" {
		return scope
	}
	expr, err := parseJQLExpression(jql)
	if err != nil {
		scope.Analyzed = false
		return scope
	}
	if !collectJQLProjectScope(expr, false, &scope) {
		scope.Analyzed = false
	}
	return scope
}

func collectJQLProjectScope(expr jqlBooleanExpr, negated bool, scope *JQLProjectScope) bool {
	switch node := expr.(type) {
	case jqlLeafExpr:
		keys, leafNegated, ok, isProject := parseJQLProjectLeaf(node.clause)
		if !isProject {
			return true
		}
		if !ok {
			return false
		}
		scope.HasClause = true
		if leafNegated != negated {
			scope.Exclude = append(scope.Exclude, keys...)
		} else {
			scope.Include = append(scope.Include, keys...)
		}
		return true
	case jqlNotExpr:
		return collectJQLProjectScope(node.operand, !negated, scope)
	case jqlBinaryExpr:
		return collectJQLProjectScope(node.left, negated, scope) &&
			collectJQLProjectScope(node.right, negated, scope)
	default:
		return true
	}
}

// parseJQLProjectLeaf reports whether a leaf clause is a project predicate and,
// when it is, which keys it requires or rejects.
func parseJQLProjectLeaf(clause string) (keys []string, negated, ok, isProject bool) {
	clause = strings.TrimSpace(clause)
	if m := jqlOuterParensPattern.FindStringSubmatch(clause); len(m) == 2 {
		clause = strings.TrimSpace(m[1])
	}
	m := jqlSimpleClausePattern.FindStringSubmatch(clause)
	if len(m) != 4 {
		// A malformed project clause must not be treated as absent, or the
		// caller would widen the selector to the whole workspace.
		if jqlClauseMentionsField(clause, "project") {
			return nil, false, false, true
		}
		return nil, false, false, false
	}
	if normalizeJQLField(m[1]) != "project" {
		return nil, false, false, false
	}
	op := normalizeJQLOperator(m[2])
	rawValue := strings.TrimSpace(m[3])
	switch op {
	case "=", "IN":
		negated = false
	case "!=", "NOT IN":
		negated = true
	default:
		return nil, false, false, true
	}
	if op == "IN" || op == "NOT IN" {
		values, listOK := parseJQLValueList(rawValue)
		if !listOK {
			return nil, false, false, true
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			keys = append(keys, value)
		}
		return keys, negated, len(keys) > 0, true
	}
	value := unquoteJQLValue(rawValue)
	if strings.TrimSpace(value) == "" {
		return nil, false, false, true
	}
	return []string{value}, negated, true, true
}

// JQLProjectScopeDecision classifies a filter's project predicate against the
// Jira project keys imported into one workspace.
type JQLProjectScopeDecision int

const (
	// JQLScopeWorkspaceWide means the predicate does not restrict membership
	// beyond the workspace, so a workspace-scoped selector is equivalent.
	JQLScopeWorkspaceWide JQLProjectScopeDecision = iota
	// JQLScopeUnrelated means the predicate names only projects that are not
	// imported into this workspace, so the filter does not belong here.
	JQLScopeUnrelated
	// JQLScopeNarrowed means the predicate restricts membership to a strict
	// subset of the workspace's projects, which workspace-scoped QL cannot
	// express without widening or guessing.
	JQLScopeNarrowed
	// JQLScopeIndeterminate means the project predicate could not be analyzed.
	JQLScopeIndeterminate
)

// DecideJQLProjectScope classifies a filter's project predicate against the
// Jira project keys imported into one workspace. workspaceProjects must list at
// least the current project; an empty list is treated as indeterminate.
func DecideJQLProjectScope(scope JQLProjectScope, workspaceProjects []string) JQLProjectScopeDecision {
	if !scope.Analyzed {
		return JQLScopeIndeterminate
	}
	inWorkspace := make(map[string]struct{}, len(workspaceProjects))
	for _, key := range workspaceProjects {
		inWorkspace[normalizeJQLProjectKey(key)] = struct{}{}
	}
	if !scope.HasClause {
		// No project predicate: the filter spans every project, so each
		// workspace keeps its own slice.
		return JQLScopeWorkspaceWide
	}
	if len(inWorkspace) == 0 {
		return JQLScopeIndeterminate
	}
	for _, key := range scope.Exclude {
		if _, ok := inWorkspace[normalizeJQLProjectKey(key)]; ok {
			// The filter rejects a project that lives in this workspace.
			return JQLScopeNarrowed
		}
	}
	referenced := make(map[string]struct{}, len(scope.Include))
	for _, key := range scope.Include {
		referenced[normalizeJQLProjectKey(key)] = struct{}{}
	}
	if len(referenced) == 0 {
		// Only exclusions, and none apply to this workspace.
		return JQLScopeWorkspaceWide
	}
	anyIncluded := false
	for key := range inWorkspace {
		if _, ok := referenced[key]; ok {
			anyIncluded = true
			break
		}
	}
	if !anyIncluded {
		return JQLScopeUnrelated
	}
	for key := range inWorkspace {
		if _, ok := referenced[key]; !ok {
			// The workspace holds a project the filter does not name, so the
			// workspace scope alone would widen the selector.
			return JQLScopeNarrowed
		}
	}
	return JQLScopeWorkspaceWide
}

func normalizeJQLProjectKey(key string) string {
	return strings.ToUpper(strings.TrimSpace(key))
}

// jqlClauseMentionsField reports whether a clause begins with the given field,
// even when the rest of the clause does not match the simple-clause grammar.
func jqlClauseMentionsField(clause, field string) bool {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return false
	}
	token := clause
	for i, r := range clause {
		if isJQLBoundary(r) || r == '=' || r == '!' || r == '<' || r == '>' || r == '~' {
			token = clause[:i]
			break
		}
	}
	return normalizeJQLField(token) == field
}

func translateJQLClause(clause string) (string, bool) {
	clause = strings.TrimSpace(clause)
	if m := jqlOuterParensPattern.FindStringSubmatch(clause); len(m) == 2 {
		clause = strings.TrimSpace(m[1])
	}
	if containsTopLevelJQLKeyword(clause, "OR") || containsTopLevelJQLKeyword(clause, "AND") {
		return "", false
	}
	m := jqlSimpleClausePattern.FindStringSubmatch(clause)
	if len(m) != 4 {
		return "", false
	}
	field := normalizeJQLField(m[1])
	op := normalizeJQLOperator(m[2])
	rawValue := strings.TrimSpace(m[3])

	if field == "project" {
		if op == "=" || op == "IN" {
			// Represented by workspace_id in every imported collection. A
			// tautology keeps an OR group from silently narrowing.
			return "1 = 1", true
		}
		return "", false
	}
	qlField, valueMapper, ok := jqlFieldMapping(field)
	if !ok {
		return "", false
	}
	values, listOK := parseJQLValueList(rawValue)
	if op == "IN" || op == "NOT IN" {
		if !listOK || len(values) == 0 {
			return "", false
		}
		mapped := make([]string, 0, len(values))
		for _, value := range values {
			if mv, ok := valueMapper(value); ok {
				mapped = append(mapped, quoteQLValue(mv))
			}
		}
		if len(mapped) == 0 {
			return "", false
		}
		return fmt.Sprintf("%s %s (%s)", qlField, op, strings.Join(mapped, ", ")), true
	}
	if listOK {
		return "", false
	}
	value := unquoteJQLValue(rawValue)
	mapped, ok := valueMapper(value)
	if !ok {
		return "", false
	}
	if op == "~" {
		return fmt.Sprintf("%s ~ %s", qlField, quoteQLValue(mapped)), true
	}
	return fmt.Sprintf("%s %s %s", qlField, op, quoteQLValue(mapped)), true
}

func normalizeJQLField(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	field = strings.ReplaceAll(field, " ", "")
	field = strings.ReplaceAll(field, "_", "")
	return field
}

func normalizeJQLOperator(op string) string {
	op = strings.ToUpper(jqlWhitespacePattern.ReplaceAllString(strings.TrimSpace(op), " "))
	if op == "<>" {
		return "!="
	}
	return op
}

func jqlFieldMapping(field string) (qlField string, valueMapper func(string) (string, bool), ok bool) {
	identity := func(v string) (string, bool) { return strings.TrimSpace(v), strings.TrimSpace(v) != "" }
	switch field {
	case "status":
		return "status", identity, true
	case "statuscategory":
		return "status_category", identity, true
	case "priority":
		return "priority", func(v string) (string, bool) { return SuggestPriorityMapping(v), strings.TrimSpace(v) != "" }, true
	case "issuetype", "type":
		return "itemtypename", identity, true
	case "requesttype", "customerrequesttype", "requesttypename":
		return "requesttypename", identity, true
	case "organization", "organizations", "organisation", "customerorganization", "customerorganisation":
		return "customerorganisation", identity, true
	case "summary":
		return "title", identity, true
	case "description":
		return "description", identity, true
	case "labels", "label":
		return "labels", identity, true
	case "fixversion", "fixversions", "fixversion/s":
		return "milestonename", identity, true
	case "component", "components":
		return "labels", func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", false
			}
			return "component:" + v, true
		}, true
	case "affectedversion", "affectedversions", "affectedversion/s":
		return "labels", func(v string) (string, bool) {
			v = strings.TrimSpace(v)
			if v == "" {
				return "", false
			}
			return "affects:" + v, true
		}, true
	case "key", "issuekey":
		return "key", identity, true
	}
	return "", nil, false
}

func parseJQLValueList(raw string) ([]string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "(") || !strings.HasSuffix(raw, ")") {
		return nil, false
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return nil, true
	}
	var values []string
	var current strings.Builder
	quote := rune(0)
	for _, r := range inner {
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			current.WriteRune(r)
		case ',':
			values = append(values, unquoteJQLValue(current.String()))
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	values = append(values, unquoteJQLValue(current.String()))
	return values, true
}

func unquoteJQLValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

func quoteQLValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if jqlIdentifierSafe.MatchString(value) && !strings.Contains(value, ":") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func isJQLBoundary(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '(' || r == ')'
}

func hasJQLKeywordAt(runes []rune, idx int, keyword string) bool {
	if idx+len(keyword) > len(runes) {
		return false
	}
	for j, r := range keyword {
		if !strings.EqualFold(string(runes[idx+j]), string(r)) {
			return false
		}
	}
	beforeOK := idx == 0 || isJQLBoundary(runes[idx-1])
	afterIdx := idx + len(keyword)
	afterOK := afterIdx >= len(runes) || isJQLBoundary(runes[afterIdx])
	return beforeOK && afterOK
}

func containsTopLevelJQLKeyword(value, keyword string) bool {
	quote := rune(0)
	depth := 0
	runes := []rune(value)
	for i, r := range runes {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && hasJQLKeywordAt(runes, i, keyword) {
				return true
			}
		}
	}
	return false
}

// JQL boolean parsing
// ---------------------------------------------------------------------------
//
// Jira saved filters routinely nest AND/OR groups. The original translator only
// understood a flat top-level AND list and rejected anything nested or ORed.
// This parser turns a JQL expression into a small tree so the translator can
// preserve grouping and boolean intent in the emitted Windshift QL.

type jqlBooleanExpr interface{}

type jqlLeafExpr struct {
	clause string
}

type jqlBinaryExpr struct {
	op          string
	left, right jqlBooleanExpr
}

type jqlNotExpr struct {
	operand jqlBooleanExpr
}

type jqlExprParser struct {
	runes []rune
	pos   int
}

func parseJQLExpression(jql string) (jqlBooleanExpr, error) {
	parser := &jqlExprParser{runes: []rune(jql)}
	expr, err := parser.parseOr()
	if err != nil {
		return nil, err
	}
	parser.skipSpaces()
	if parser.pos < len(parser.runes) {
		return nil, fmt.Errorf("unexpected token near %q", parser.remainingPreview())
	}
	return expr, nil
}

func (p *jqlExprParser) parseOr() (jqlBooleanExpr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peekKeyword("OR") {
		p.consumeKeyword("OR")
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = jqlBinaryExpr{op: "OR", left: left, right: right}
	}
	return left, nil
}

func (p *jqlExprParser) parseAnd() (jqlBooleanExpr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.peekKeyword("AND") {
		p.consumeKeyword("AND")
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = jqlBinaryExpr{op: "AND", left: left, right: right}
	}
	return left, nil
}

func (p *jqlExprParser) parseNot() (jqlBooleanExpr, error) {
	// A prefix NOT is the only unary operator here; `field NOT IN (...)` is a
	// leaf because NOT is only special when a primary is expected.
	if p.peekKeyword("NOT") {
		p.consumeKeyword("NOT")
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return jqlNotExpr{operand: operand}, nil
	}
	return p.parsePrimary()
}

func (p *jqlExprParser) parsePrimary() (jqlBooleanExpr, error) {
	p.skipSpaces()
	if p.pos >= len(p.runes) {
		return nil, fmt.Errorf("unexpected end of JQL expression")
	}
	if p.runes[p.pos] == '(' {
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.pos >= len(p.runes) || p.runes[p.pos] != ')' {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return inner, nil
	}
	clause := p.readClause()
	if strings.TrimSpace(clause) == "" {
		return nil, fmt.Errorf("empty JQL clause")
	}
	return jqlLeafExpr{clause: clause}, nil
}

func (p *jqlExprParser) readClause() string {
	start := p.pos
	quote := rune(0)
	depth := 0
	for p.pos < len(p.runes) {
		r := p.runes[p.pos]
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			p.pos++
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			p.pos++
		case '(':
			depth++
			p.pos++
		case ')':
			if depth == 0 {
				return strings.TrimSpace(string(p.runes[start:p.pos]))
			}
			depth--
			p.pos++
		default:
			if depth == 0 && (p.keywordAt(p.pos, "AND") || p.keywordAt(p.pos, "OR")) {
				return strings.TrimSpace(string(p.runes[start:p.pos]))
			}
			p.pos++
		}
	}
	return strings.TrimSpace(string(p.runes[start:p.pos]))
}

func (p *jqlExprParser) skipSpaces() {
	for p.pos < len(p.runes) {
		switch p.runes[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *jqlExprParser) keywordAt(idx int, keyword string) bool {
	if idx+len(keyword) > len(p.runes) {
		return false
	}
	for j, r := range keyword {
		if !strings.EqualFold(string(p.runes[idx+j]), string(r)) {
			return false
		}
	}
	beforeOK := idx == 0 || isJQLBoundary(p.runes[idx-1])
	afterIdx := idx + len(keyword)
	afterOK := afterIdx >= len(p.runes) || isJQLBoundary(p.runes[afterIdx])
	return beforeOK && afterOK
}

func (p *jqlExprParser) peekKeyword(keyword string) bool {
	p.skipSpaces()
	return p.keywordAt(p.pos, keyword)
}

func (p *jqlExprParser) consumeKeyword(keyword string) {
	p.skipSpaces()
	p.pos += len(keyword)
}

func (p *jqlExprParser) remainingPreview() string {
	if p.pos >= len(p.runes) {
		return ""
	}
	end := p.pos + 20
	if end > len(p.runes) {
		end = len(p.runes)
	}
	return string(p.runes[p.pos:end])
}

// translateJQLBoolean renders a parsed expression as Windshift QL. A single
// unsupported leaf invalidates the whole expression: returning a partial QL
// would silently broaden the selector, so the caller keeps the original JQL
// and marks the goal needs-attention instead.
func translateJQLBoolean(expr jqlBooleanExpr) (ql string, unsupported []string) {
	switch node := expr.(type) {
	case jqlLeafExpr:
		translated, ok := translateJQLClause(node.clause)
		if !ok {
			return "", []string{strings.TrimSpace(node.clause)}
		}
		return translated, nil
	case jqlNotExpr:
		inner, unsupported := translateJQLBoolean(node.operand)
		if len(unsupported) > 0 {
			return "", unsupported
		}
		return "NOT (" + inner + ")", nil
	case jqlBinaryExpr:
		left, leftUnsupported := translateJQLBoolean(node.left)
		right, rightUnsupported := translateJQLBoolean(node.right)
		combined := make([]string, 0, len(leftUnsupported)+len(rightUnsupported))
		combined = append(combined, leftUnsupported...)
		combined = append(combined, rightUnsupported...)
		if len(combined) > 0 {
			return "", combined
		}
		switch {
		case left == "" && right == "":
			return "", nil
		case left == "":
			return right, nil
		case right == "":
			return left, nil
		default:
			return "(" + left + " " + node.op + " " + right + ")", nil
		}
	default:
		return "", nil
	}
}
