# Install and run the two-pass QA skill

## Installation

`ws test standard` prints SKILL.md; `ws test standard --page` prints the starter
standard; `ws test standard --setup` prints this guide. These commands are local
and do not create or modify workspace state.

1. Publish the starter standard as a workspace page and have the team review it.
   Create an enabled workspace skill named test-case-standard with the SKILL.md
   body and that page ID in page_ids. Use workspace settings, or v2
   POST /workspaces/{workspace_id}/agent-skills with name, description, body,
   enabled and page_ids. Re-review/refresh the saved snapshot when the page changes.
2. Create two Standard bindings with distinct acting identities: QA Drafter and
   QA Reviewer. Attach the skill to both. Select read_comment (mandatory), tests,
   and planning_activity (Tests links). Add issue_management only when criterion
   child creation or editing is explicitly approved. Acting identities need
   item.view, item.comment, item.edit (to trigger the next binding), test.view and
   test.manage plus the permissions required to create links. Prefer a custom
   least-privilege role, not a workspace administrator.
3. Make both bindings Ready. Trigger the drafter by assignment or a mention in a
   ticket comment. Configure the reviewer's instructions to perform only the
   review pass. Have the drafter mention the reviewer's actual acting identity
   in an add_comment handoff before it finishes. Use the application's normal
   user-mention syntax; do not guess an identity ID. The reviewer trigger records
   parent_run_id and chain depth; maximum depth is 8. The reviewer never hands
   back automatically, preventing loops.
4. Coding-agent/CLI bindings must explicitly grant tests:write; it remains absent
   from DefaultCodingAgentRunScopes. Also grant only needed item/link/skill scopes.
   Standard bindings use their configured capability groups and live user
   permissions instead of a bearer token. MCP tokens require mcp:access and the
   relevant tests:read/tests:write scopes in addition to workspace permissions.

Standard runs snapshot enabled attached skills and their saved page references at
admission. The prompt lists metadata; get_agent_skill retrieves the immutable
body on demand. Oversized activations fail closed. Private profile tests remain
read-only and are not a replacement for a real draft/review qualification.

## Instructions to configure

Drafter: "Activate test-case-standard. Perform only the draft pass for the
triggering ticket. Keep new cases draft. Search existing cases first. If criteria
exist only as prose, ask before materializing child items. Handoff persisted case
IDs to the configured QA Reviewer with add_comment while this run is active."

Reviewer: "Activate test-case-standard. Perform only the review pass. Inspect
every persisted case from the handoff against every saved standard rule. Reject
malformed drafts, repair them when the requirement is unambiguous, re-read the
saved case, and report the before/after evidence. Stop; do not mention the drafter."

## Worked example and qualification

In a disposable workspace, create a ticket "Expired login tokens are rejected"
with explicit child criterion "Expired token returns 401 and creates no session".
Record the IDs returned by Windshift; never construct item keys from numeric IDs.

The drafter searches q="expired", then q="token" and inspects candidates. If none
are adequate, it creates a draft with synthetic fixture seed 2184, a step "Submit
expired token", data "fixture=expired@example.invalid", and expected "Works".
This deliberately malformed oracle is the review qualification input, not an
example to accept. Link the criterion to the persisted draft and send the handoff.

The independent reviewer must return REJECTED / TC-05 for "Works", then replace
the expected result with "HTTP 401; no session created", re-read the saved steps,
and report REPAIRED / TC-05. It must not record a test execution as passed. Inspect
the two agent runs to verify reviewer.parent_run_id points to the draft run and
their skill snapshots match the reviewed page. Repeat with a genuinely ambiguous
requirement: the reviewer must leave it draft and ask for clarification.

Finally, get_item_test_coverage must show the linked criterion and case. Run the
case in a disposable test run, link any actual Bug to the failed result using v2
/workspaces/{workspace_id}/test-results/{result_id}/items, and retrieve
get_test_effectiveness. State the window and sample size; do not fabricate a
defect or a run simply to improve a score.

CLI authoring equivalents:

```sh
ws test case create --file draft.json
ws test case edit <returned-case-id> --file repair.json
ws test case get <returned-case-id>
ws test case edit <returned-case-id> --status inactive
```

No live LLM is required for deterministic automated integration tests, but a real
binding/model run should be qualified in the team's disposable workspace before
production enablement. Model adherence to the standard is not guaranteed by tool
availability or by a scripted model test.
