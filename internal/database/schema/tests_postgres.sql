-- Test management tables
-- Test folders for organizing test cases
CREATE TABLE IF NOT EXISTS test_folders (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	parent_id INTEGER REFERENCES test_folders(id) ON DELETE SET NULL,
	name TEXT NOT NULL,
	description TEXT,
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS test_cases (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	folder_id INTEGER REFERENCES test_folders(id) ON DELETE SET NULL,
	title TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	priority TEXT NOT NULL DEFAULT 'medium',
	status TEXT NOT NULL DEFAULT 'active',
	estimated_duration INTEGER DEFAULT 0,
	preconditions TEXT DEFAULT '',
	sort_order INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_test_folders_workspace_id ON test_folders(workspace_id);
CREATE INDEX IF NOT EXISTS idx_test_folders_sort_order ON test_folders(sort_order);
CREATE INDEX IF NOT EXISTS idx_test_folders_parent_id ON test_folders(parent_id);
CREATE INDEX IF NOT EXISTS idx_test_cases_workspace_id ON test_cases(workspace_id);
CREATE INDEX IF NOT EXISTS idx_test_cases_folder_id ON test_cases(folder_id);
CREATE INDEX IF NOT EXISTS idx_test_cases_sort_order ON test_cases(sort_order);

CREATE TABLE IF NOT EXISTS test_sets (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	milestone_id INTEGER REFERENCES milestones(id) ON DELETE SET NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_test_sets_workspace_id ON test_sets(workspace_id);

CREATE TABLE IF NOT EXISTS set_test_cases (
	id SERIAL PRIMARY KEY,
	set_id INTEGER NOT NULL,
	test_case_id INTEGER NOT NULL,
	FOREIGN KEY (set_id) REFERENCES test_sets(id) ON DELETE CASCADE,
	FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE,
	UNIQUE(set_id, test_case_id)
);

CREATE TABLE IF NOT EXISTS test_run_templates (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	set_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (set_id) REFERENCES test_sets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_test_run_templates_workspace_id ON test_run_templates(workspace_id);

CREATE TABLE IF NOT EXISTS test_runs (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	template_id INTEGER,
	set_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	assignee_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	ended_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	FOREIGN KEY (template_id) REFERENCES test_run_templates(id) ON DELETE SET NULL,
	FOREIGN KEY (set_id) REFERENCES test_sets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_test_runs_workspace_id ON test_runs(workspace_id);

CREATE TABLE IF NOT EXISTS test_results (
	id SERIAL PRIMARY KEY,
	run_id INTEGER NOT NULL,
	test_case_id INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'not_run',
	actual_result TEXT,
	notes TEXT,
	executed_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE,
	FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE,
	UNIQUE(run_id, test_case_id)
);

-- Test labels for categorizing test cases (workspace-scoped)
CREATE TABLE IF NOT EXISTS test_labels (
	id SERIAL PRIMARY KEY,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '#3B82F6',
	description TEXT DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	UNIQUE(workspace_id, name)
);

CREATE INDEX IF NOT EXISTS idx_test_labels_workspace_id ON test_labels(workspace_id);

-- Junction table for test case labels
CREATE TABLE IF NOT EXISTS test_case_labels (
	id SERIAL PRIMARY KEY,
	test_case_id INTEGER NOT NULL,
	label_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE,
	FOREIGN KEY (label_id) REFERENCES test_labels(id) ON DELETE CASCADE,
	UNIQUE(test_case_id, label_id)
);

-- Test steps for detailed test case execution
CREATE TABLE IF NOT EXISTS test_steps (
	id SERIAL PRIMARY KEY,
	test_case_id INTEGER NOT NULL,
	step_number INTEGER NOT NULL,
	action TEXT NOT NULL,
	data TEXT DEFAULT '',
	expected TEXT NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE
);

-- Test step results for execution tracking
CREATE TABLE IF NOT EXISTS test_step_results (
	id SERIAL PRIMARY KEY,
	test_result_id INTEGER NOT NULL,
	test_step_id INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'not_run',
	actual_result TEXT DEFAULT '',
	notes TEXT DEFAULT '',
	item_id INTEGER,
	executed_at TIMESTAMP,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (test_result_id) REFERENCES test_results(id) ON DELETE CASCADE,
	FOREIGN KEY (test_step_id) REFERENCES test_steps(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE SET NULL,
	UNIQUE(test_result_id, test_step_id)
);

-- Junction table for linking test results to work items
CREATE TABLE IF NOT EXISTS test_result_items (
	id SERIAL PRIMARY KEY,
	test_result_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (test_result_id) REFERENCES test_results(id) ON DELETE CASCADE,
	FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
	UNIQUE(test_result_id, item_id)
);

CREATE INDEX IF NOT EXISTS idx_test_result_items_test_result_id ON test_result_items(test_result_id);
CREATE INDEX IF NOT EXISTS idx_test_result_items_item_id ON test_result_items(item_id);

CREATE INDEX IF NOT EXISTS idx_set_test_cases_set_id ON set_test_cases(set_id);
CREATE INDEX IF NOT EXISTS idx_set_test_cases_test_case_id ON set_test_cases(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_runs_set_id ON test_runs(set_id);
CREATE INDEX IF NOT EXISTS idx_test_results_run_id ON test_results(run_id);
CREATE INDEX IF NOT EXISTS idx_test_results_test_case_id ON test_results(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_case_labels_test_case_id ON test_case_labels(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_case_labels_label_id ON test_case_labels(label_id);
CREATE INDEX IF NOT EXISTS idx_test_steps_test_case_id ON test_steps(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_step_results_test_result_id ON test_step_results(test_result_id);
CREATE INDEX IF NOT EXISTS idx_test_step_results_test_step_id ON test_step_results(test_step_id);
CREATE INDEX IF NOT EXISTS idx_test_sets_milestone_id ON test_sets(milestone_id);
