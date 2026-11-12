# Windshift DC Import Companion

A small, read-only Jira Data Center plugin that exposes configuration the public
Jira REST API does not, so the Windshift importer can read it faithfully.

See `docs/jira-dc-import-companion-plugin.md` in the core repo for the concept.

## Status

MVP: the workflow endpoint. It returns a project's workflow scheme and each
workflow's full graph — statuses, transitions, and the condition / validator /
post-function descriptors that `GET /rest/api/2/workflow` hides.

## Build

```bash
atlas-mvn package
# -> target/dc-import-companion-1.0.0.jar
```

## Install

Upload the JAR through UPM:

```bash
curl -u admin:admin -X POST \
  -H "X-Atlassian-Token: no-check" \
  -F "plugin=@target/dc-import-companion-1.0.0.jar" \
  http://localhost:8080/rest/plugins/1.0/
```

The harness wires this behind `JIRA_DC_INSTALL_COMPANION=1`.

## API

Base: `/rest/windshift-import/1.0`. Admin-only, read-only, JSON.

### `GET /capabilities`

```json
{ "pluginVersion": "1.0.0", "deploymentType": "datacenter",
  "capabilities": ["workflows", "screens"] }
```

### `GET /project/{projectKey}/screens`

```json
{
  "issueTypeScreens": {
    "default": { "createScreenId": "10002", "editScreenId": "10003", "viewScreenId": "10003" }
  },
  "screens": [ {
    "id": "10002", "name": "Create Screen", "description": "", "tabCount": 1,
    "fields": [ { "id": "summary", "name": "Summary" },
                { "id": "customfield_10104", "name": "Story Points" } ]
  } ]
}
```

### `GET /project/{projectKey}/workflows`

```json
{
  "project": { "id": "10000", "key": "ENG", "name": "Engineering" },
  "issueTypeWorkflowIds": { "10001": "ENG: Software Simplified Workflow" },
  "workflows": [ {
    "id": "ENG: Software Simplified Workflow",
    "name": "ENG: Software Simplified Workflow",
    "statuses": [ { "id": "10000", "name": "To Do" } ],
    "transitions": [ {
      "id": "41", "name": "Done", "type": "DIRECTED",
      "fromStatusIds": ["3"], "toStatusId": "10001",
      "conditions": [ {
        "type": "class",
        "class": "com.atlassian.jira.workflow.condition.PermissionCondition",
        "kind": "permission",
        "args": { "class.name": "...PermissionCondition", "permission": "Resolve Issue" }
      } ],
      "validators": [],
      "postFunctions": [ { "type": "class", "class": "...", "kind": "unknown", "args": {} } ]
    } ]
  } ]
}
```

`kind` is the normalized view of the common built-ins (`user_in_group`,
`user_in_role`, `permission`, `only_assignee`, `subtask_blocking`, `unknown`);
the raw `class` + `args` are always present for forward compatibility.

## Security

Read-only. Requires global `ADMINISTER`; returns `403` otherwise. Never returns
credentials or secrets.
