# Windshift v0.4.3

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

### Team Management
Create and manage teams with member assignments. Teams can be used in automation actions for round-robin task assignment, distributing work evenly across team members.

### Round-Robin Assignment Action Node
A new automation action node (`round_robin_assign`) allows automatic assignment of items to team members in a rotating fashion. Supports skipping members who are on leave and using leave substitutes.

### Hotkey & Modal Focus Fixes
Fixed an issue where global keyboard shortcuts (e.g., the 'c' create shortcut) could fire while a modal dialog was open, causing the dialog to appear to close unexpectedly. Three layers of defense were added:
- Hotkeys on buttons behind an open modal are now automatically blocked
- Keyboard events inside modals no longer propagate to global listeners
- Dropdown menus no longer incorrectly send focus to the document body on close

### Terminal Panel Restricted to Desktop App
The terminal navigation link in the sidebar is now only visible when running inside the Tauri desktop app, where it is actually functional.

---

## Bug Fixes

- **Custom Field Dialog:** Fixed dialog closing when typing after selecting a field type from the dropdown
- **Dropdown Focus:** Removed erroneous `blur()` call that sent focus to `document.body` when closing a dropdown menu

## Other Changes

- Removed Playwright e2e test suite and related dependencies
- Added `teams.manage` permission for team administration
