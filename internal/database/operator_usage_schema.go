package database

const operatorUsageSchema = `CREATE TABLE operator_usage (
	resource TEXT NOT NULL,
	period BIGINT NOT NULL,
	used BIGINT NOT NULL CHECK (used >= 0),
	PRIMARY KEY (resource, period)
);`
