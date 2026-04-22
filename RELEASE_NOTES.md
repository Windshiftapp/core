# Windshift v0.5.7

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Features

### Action execution actor

Workspace actions no longer run with whichever permissions the triggering user happens to hold. Two new concepts address the gap:

- **`actor_user_id` on `actions`** — nullable override. When null the action continues to run under the triggering user, preserving prior behaviour. When set, every node executes under the named user's permissions and side-effects (comment authorship, item history, cascade events) are attributed to them.
- **`action.set_actor` global permission** — required to set or change `actor_user_id`. Seeded with no default role assignment; only `system.admin` or an explicit grant can configure an override. The permission is global-scope because an actor override grants cross-workspace reach and cannot be bounded by workspace-scoped `action.manage` alone.

The execution engine now centralises this as `EffectiveActorID` on `ExecutionContext` and threads it through every node executor (`set_field` column and custom, `set_status`, `add_comment`, `notify_user`, `round_robin_assign`, `create_asset`, `update_asset`), plus the downstream `WorkflowService.PerformTransition`, `CommentService.Create`, `NotificationService.NotifyUsers`, and cascade `ActionEvent` / `AssetActionEvent` emissions.

### Per-node permission enforcement

Previously, node authorisation was inconsistent: `create_asset` / `update_asset` checked asset-set RBAC, but `set_field`, `set_status`, `add_comment` and `round_robin_assign` wrote through without a permission check. The effective actor is now checked against the workspace before each mutating node runs — `item.edit` for `set_field`, `set_status`, and `round_robin_assign`; `item.comment` for `add_comment`. Asset mutations still go through the existing asset-set check, unchanged. `notify_user` remains unchecked because it mutates no workspace state.

Authorisation failures fail the node, mark the action `failed`, and record the missing-permission error in the execution trace. A missing permission-service wiring refuses closed rather than silently skipping the check.

### Action execution audit trail

`action_execution_logs` gains `trigger_user_id` and `effective_actor_user_id` so the per-run record distinguishes who caused the event from whose rights governed the run. Every set-or-change of an action's actor also writes a dedicated `automation.set_actor` entry to the generic audit log with the previous and new actor IDs and the administrator who made the change.

## Enhancements

### Action flow editor

- A **run-as** picker sits above the node palette. Users with `action.set_actor` can choose any user (or clear back to "run as triggering user"); users without the permission see a read-only label showing the currently configured actor or a hint explaining the default.
- New nodes added from the palette now land at the centre of the visible canvas rather than a fixed coordinate region that frequently sat outside the viewport. Placement is computed from the live viewport (tracked via `onmove`, keeping SvelteFlow in uncontrolled mode so `defaultViewport` still governs first render) and offset by half a node footprint. A small random jitter keeps successive clicks from stacking pixel-perfectly.
- The minimap now colour-codes nodes by type, mirroring the accent colours used on the canvas (trigger amber, `set_field` purple, `set_status` teal, `add_comment` orange, `notify_user` magenta, `condition` yellow, `update_asset` teal, `create_asset` green). The minimap shell itself picks up a surface-raised background, border, shadow and rounded corners so it reads as editor chrome rather than a bare overlay.

### Security page

- API token creation exposes the full scope set as a grid with resource rows and read / write / delete columns. Preset buttons (Read-only, Read + Write, Clear) cover common picks; an Admin grid renders only for system administrators. The prior hardcoded three-scope default is pre-selected; **Create** is disabled until at least one scope is chosen.

### Minor UX

- Service-user checkbox on the user-create modal no longer wraps its label when the descriptive hint is long; the hint is free to wrap below.
- Spacing fix on the user profile page.

## Upgrade notes

- `actions` gains an `actor_user_id` column and `action_execution_logs` gains `trigger_user_id` and `effective_actor_user_id`. Existing rows migrate with these fields null; behaviour for actions without an override is unchanged.
- The new global permission `action.set_actor` is seeded on upgrade but not granted to any role. Assign it explicitly to administrators who need to configure actor overrides.
- Workspace actions that previously succeeded by relying on the triggering user's lack-of-enforcement on `set_field` / `set_status` / `round_robin_assign` / `add_comment` will now fail if the effective actor lacks the corresponding workspace permission. Review audit logs for `automation.execute` failures after upgrade; the most common fix is to grant the triggering user `item.edit` / `item.comment` on the workspace, or to set an explicit `actor_user_id` on the action.
