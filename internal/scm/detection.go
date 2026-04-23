package scm

import (
	"fmt"
	"regexp"
	"strings"
)

// DetectionSource represents where an item key was detected
type DetectionSource string

const (
	DetectionSourceManual        DetectionSource = "manual"
	DetectionSourcePRTitle       DetectionSource = "pr_title"
	DetectionSourcePRBody        DetectionSource = "pr_body"
	DetectionSourceBranchName    DetectionSource = "branch_name"
	DetectionSourceCommitMessage DetectionSource = "commit_message"
)

// DetectedItemKey represents an item key found in text
type DetectedItemKey struct {
	Key    string          // The full key (e.g., "PROJ-123")
	Prefix string          // The workspace prefix (e.g., "PROJ")
	Number int             // The item number (e.g., 123)
	Source DetectionSource // Where it was detected
}

// ItemKeyDetector handles detection of item keys in text
type ItemKeyDetector struct {
	// defaultPattern matches workspace keys like PROJ-123, BUG-42, etc.
	// Pattern: 2-10 uppercase letters followed by dash and 1+ digits
	defaultPattern *regexp.Regexp
}

// NewItemKeyDetector creates a new item key detector
func NewItemKeyDetector() *ItemKeyDetector {
	return &ItemKeyDetector{
		// Default pattern: UPPERCASE_PREFIX-NUMBER
		// Examples: PROJ-123, BUG-42, TASK-1001
		defaultPattern: regexp.MustCompile(`\b([A-Z]{2,10})-(\d+)\b`),
	}
}

// DetectItemKeys extracts item keys from text using the default pattern
// Returns all unique item keys found in the text
func (d *ItemKeyDetector) DetectItemKeys(text string, source DetectionSource) []DetectedItemKey {
	return d.DetectItemKeysWithPattern(text, "", source)
}

// DetectItemKeysWithPattern extracts item keys using a custom pattern
// If pattern is empty, uses the default pattern
func (d *ItemKeyDetector) DetectItemKeysWithPattern(text, pattern string, source DetectionSource) []DetectedItemKey {
	var re *regexp.Regexp
	if pattern != "" {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			// Fall back to default pattern if custom pattern is invalid
			re = d.defaultPattern
		}
	} else {
		re = d.defaultPattern
	}

	matches := re.FindAllStringSubmatch(text, -1)
	if matches == nil {
		return nil
	}

	// Use a map to deduplicate
	seen := make(map[string]bool)
	var results []DetectedItemKey

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		key := match[0]
		if seen[key] {
			continue
		}
		seen[key] = true

		var number int
		_, _ = fmt.Sscanf(match[2], "%d", &number)

		results = append(results, DetectedItemKey{
			Key:    key,
			Prefix: match[1],
			Number: number,
			Source: source,
		})
	}

	return results
}

// DetectItemKeysForPrefix extracts item keys matching a specific workspace prefix
func (d *ItemKeyDetector) DetectItemKeysForPrefix(text, prefix string, source DetectionSource) []DetectedItemKey {
	if prefix == "" {
		return nil
	}

	// Create pattern for specific prefix
	pattern := fmt.Sprintf(`\b(%s)-(\d+)\b`, regexp.QuoteMeta(strings.ToUpper(prefix)))
	return d.DetectItemKeysWithPattern(text, pattern, source)
}

// DetectFromPullRequest extracts item keys from a pull request
// Searches in title, body, and head branch name
func (d *ItemKeyDetector) DetectFromPullRequest(pr *PullRequest, workspacePrefix string) []DetectedItemKey {
	var allKeys []DetectedItemKey
	seen := make(map[string]bool)

	sources := []struct {
		text   string
		source DetectionSource
	}{
		{pr.Title, DetectionSourcePRTitle},
		{pr.Body, DetectionSourcePRBody},
		{pr.HeadBranch, DetectionSourceBranchName},
	}

	for _, s := range sources {
		var keys []DetectedItemKey
		if workspacePrefix != "" {
			keys = d.DetectItemKeysForPrefix(s.text, workspacePrefix, s.source)
		} else {
			keys = d.DetectItemKeys(s.text, s.source)
		}

		for _, key := range keys {
			if !seen[key.Key] {
				seen[key.Key] = true
				allKeys = append(allKeys, key)
			}
		}
	}

	return allKeys
}

// DetectFromBranch extracts item keys from a branch name
func (d *ItemKeyDetector) DetectFromBranch(branch *Branch, workspacePrefix string) []DetectedItemKey {
	if workspacePrefix != "" {
		return d.DetectItemKeysForPrefix(branch.Name, workspacePrefix, DetectionSourceBranchName)
	}
	return d.DetectItemKeys(branch.Name, DetectionSourceBranchName)
}

// DetectFromCommit extracts item keys from a commit message
func (d *ItemKeyDetector) DetectFromCommit(commit *Commit, workspacePrefix string) []DetectedItemKey {
	if workspacePrefix != "" {
		return d.DetectItemKeysForPrefix(commit.Message, workspacePrefix, DetectionSourceCommitMessage)
	}
	return d.DetectItemKeys(commit.Message, DetectionSourceCommitMessage)
}

// SmartCommitAction is a parsed (itemKey, command, payload) triple from a
// smart-commit-formatted line. One line can produce multiple actions when
// multiple keys and/or multiple commands are combined (Jira cross-product
// semantics).
type SmartCommitAction struct {
	Key     DetectedItemKey
	Command string // normalised: "comment" or a transition slug (lowercase, hyphenated)
	Payload string // trimmed; empty for non-comment commands
}

// smartCmdRegex finds "#word" tokens that are at the start of a line or
// preceded by whitespace. This avoids matching fragments inside URLs.
var smartCmdRegex = regexp.MustCompile(`(?:^|\s)#([A-Za-z][A-Za-z0-9_-]*)`)

// ParseSmartCommitActions extracts Jira-style smart-commit actions from free
// text (commit message or PR body). Per Jira:
//   - Syntax is line-scoped: "KEY [KEY...] #cmd [args] [#cmd2 [args]]".
//   - Item keys must appear before the first "#cmd" on the line.
//   - "#comment" consumes the rest of the line up to the next "#cmd".
//   - For other commands, only the first token is considered (no args).
//   - Multi-key + multi-command produces the cross-product.
//
// workspacePrefix restricts key detection to that workspace; pass "" to accept
// any prefix.
func (d *ItemKeyDetector) ParseSmartCommitActions(text, workspacePrefix string) []SmartCommitAction {
	var actions []SmartCommitAction
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		hashIdx := indexSmartCommitHash(line)
		if hashIdx < 0 {
			continue
		}
		keysPart := line[:hashIdx]
		cmdPart := line[hashIdx:]

		var keys []DetectedItemKey
		if workspacePrefix != "" {
			keys = d.DetectItemKeysForPrefix(keysPart, workspacePrefix, DetectionSourceCommitMessage)
		} else {
			keys = d.DetectItemKeys(keysPart, DetectionSourceCommitMessage)
		}
		if len(keys) == 0 {
			continue
		}

		commands := parseSmartCommitCommands(cmdPart)
		if len(commands) == 0 {
			continue
		}

		for _, key := range keys {
			for _, cmd := range commands {
				actions = append(actions, SmartCommitAction{
					Key:     key,
					Command: cmd.name,
					Payload: cmd.payload,
				})
			}
		}
	}
	return actions
}

// indexSmartCommitHash returns the index of the first "#cmd" token on the line,
// or -1 if none. A valid token is "#" at position 0 or preceded by whitespace,
// followed by an ASCII letter.
func indexSmartCommitHash(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] != '#' {
			continue
		}
		if i > 0 {
			prev := line[i-1]
			if prev != ' ' && prev != '\t' {
				continue
			}
		}
		if i+1 < len(line) {
			c := line[i+1]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				return i
			}
		}
	}
	return -1
}

type smartCommitCommand struct {
	name    string
	payload string
}

func parseSmartCommitCommands(s string) []smartCommitCommand {
	locs := smartCmdRegex.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		return nil
	}
	var out []smartCommitCommand
	for i, loc := range locs {
		name := strings.ToLower(s[loc[2]:loc[3]])
		payloadStart := loc[3]
		payloadEnd := len(s)
		if i+1 < len(locs) {
			// locs[i+1][0] points at the whitespace char before the next '#';
			// trim to that boundary.
			payloadEnd = locs[i+1][0]
		}
		payload := strings.TrimSpace(s[payloadStart:payloadEnd])
		if name != "comment" {
			payload = ""
		}
		out = append(out, smartCommitCommand{name: name, payload: payload})
	}
	return out
}

// NormalizeBranchName extracts potential item key from common branch naming patterns
// Examples:
//   - feature/PROJ-123-add-login -> PROJ-123
//   - bugfix/PROJ-42-fix-crash -> PROJ-42
//   - PROJ-123 -> PROJ-123
func (d *ItemKeyDetector) NormalizeBranchName(branchName string) string {
	// First try direct match
	keys := d.DetectItemKeys(branchName, DetectionSourceBranchName)
	if len(keys) > 0 {
		return keys[0].Key
	}

	// Handle prefixes like feature/, bugfix/, etc.
	parts := strings.Split(branchName, "/")
	if len(parts) > 1 {
		// Try the last part
		keys = d.DetectItemKeys(parts[len(parts)-1], DetectionSourceBranchName)
		if len(keys) > 0 {
			return keys[0].Key
		}
	}

	return ""
}
