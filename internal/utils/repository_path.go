package utils

import "strings"

// SplitRepositoryPath splits namespace/project at the final slash so nested
// GitLab namespaces remain intact in the namespace portion.
func SplitRepositoryPath(fullName string) (namespace, project string, ok bool) {
	fullName = strings.TrimSpace(fullName)
	separator := strings.LastIndex(fullName, "/")
	if separator <= 0 || separator == len(fullName)-1 {
		return "", "", false
	}
	return fullName[:separator], fullName[separator+1:], true
}
