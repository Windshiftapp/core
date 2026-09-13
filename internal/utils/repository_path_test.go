package utils

import "testing"

func TestSplitRepositoryPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path      string
		namespace string
		project   string
		ok        bool
	}{
		{path: "owner/project", namespace: "owner", project: "project", ok: true},
		{path: "group/subgroup/project", namespace: "group/subgroup", project: "project", ok: true},
		{path: " group/subgroup/project ", namespace: "group/subgroup", project: "project", ok: true},
		{path: "project", ok: false},
		{path: "/project", ok: false},
		{path: "group/", ok: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			namespace, project, ok := SplitRepositoryPath(test.path)
			if namespace != test.namespace || project != test.project || ok != test.ok {
				t.Fatalf("SplitRepositoryPath(%q) = %q, %q, %v", test.path, namespace, project, ok)
			}
		})
	}
}
