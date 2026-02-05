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
