# WI-2147 implementation qualification

Status: **In Progress / draft, not release-qualified**. The binding plan is
WI-2147 attachment 916. This report does not rescope or waive its acceptance
criteria. Both repositories use `feat/wi-2147-shared-virtualization` worktrees.

## Implemented

- Shared generic controller, Svelte adapter, keyed sparse renderer, root
  registry, occurrence keys, measurement, query anchors and bounded anchor LRU.
- Tree, list, queue, flat grouped backlog and one-model paired roadmap adapters.
- Editor/picker/menu/preview retention, focus/Tab support, optional drag bridge,
  loaded-row nonvirtual preference and print mode; translated preference copy.
- Independent native-table catalog example; test fixtures remain in core-tests.
- Board/map remain intentionally unvirtualized. Existing board optional-read
  viewport gates are unchanged.
- Exact MIT dependency `@tanstack/virtual-core@3.19.9`, package-lock preserved;
  npm audit reported zero vulnerabilities. 3.19.11 was rejected by release-age
  policy; that policy was not overridden.

## Evidence collected on macOS / Bun 1.4 / Chromium

The pre-migration tree regression mounted 1,000 rows against a <=30 bound and
failed. The migrated unit regression passes. Generic component cases exercise
100/101 thresholds, 1,000/10,000 records, exact scrolling contents, sparse focus
pins, logical Tab, native tables, no-ResizeObserver fallback, print cancellation,
offscreen draft/failed-save retention, paired snapshots and ten mount/scroll/
unmount cycles returning observation counts to zero. Additional tests cover
projection keys, fractional anchors, prepend/reorder/delete, query restoration,
reference counts, cancellation, shared-root subscriptions, LRU and account
isolation, including a late old-controller teardown after account switching.

Regression-first fixes during implementation include collapse snapshot races,
preview pin acquisition loops, layout-only anchor restoration and print-cancel
position loss.

### Commands and results

From core:

```sh
VITEST_RUNTIME=bun ../core-tests/run-overlay-script.sh . scripts/run-frontend-tests.sh src/lib/virtualization src/lib/components/VirtualRows.test.ts src/lib/features/collections src/lib/features/support src/lib/editors/EditableField.test.js src/lib/pickers
```

Initial focused run: **62 files / 405 tests passed**.
Review follow-up focused run: **63 files / 414 tests passed** using the same command.
The nine-spec combined Chromium rerun also passed **21 tests** including setup,
with zero retries/skips (artifacts `test-results-76593-1794564652`). The earlier
full-suite baseline comparison was not repeated for this review-only follow-up.

```sh
VITEST_RUNTIME=bun ../core-tests/run-overlay-script.sh . scripts/run-frontend-tests.sh
```

Full run at that snapshot: **484 files passed, 9 failed; 3,644 tests passed,
5 failed**. Failures are OpenUI/Zod `z.config` under Bun (seven import-failing
suites and the CSP test) and four Vite prebundle config imports. Reproduction
against the untouched original core checkout with its own lockfile:

```sh
VITEST_RUNTIME=bun ../core-tests/run-overlay-script.sh /Users/stefanernst/realigned/windshift-core/core scripts/run-frontend-tests.sh src/lib/openui/zodJitless.test.js src/viteDepPrebundle.test.js
```

Same five test failures reproduced there. No unrelated runtime/runner fix was
folded into this item; the full suite must not be described as green.

Browser commands executed with user approval outside the restricted sandbox:

```sh
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/collection-tree-pagination.spec.ts tests/roadmap-scheduling.spec.ts tests/support-queue.spec.ts
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/roadmap-scheduling.spec.ts
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/virtual-rows.spec.ts tests/board-drag-workflow.spec.ts tests/collection-map-swimlanes.spec.ts tests/backlog-story-points-availability.spec.ts tests/collection-roadmap-sse-live.spec.ts
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/virtual-rows.spec.ts tests/backlog-virtual-drag.spec.ts
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/backlog-virtual-drag.spec.ts
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/virtual-rows.spec.ts
```

- Tree pagination and five queue workflows passed in the initial combined run.
- Roadmap scheduling suite initially reproduced the preview loop; rerun after
  its fix passed all six tests including setup, with no retries/skips.
- Board native drag/auto-scroll/workflow rejection, map swimlanes, backlog
  story-point absence/persistence, and roadmap SSE deletion passed.
- Production-API 1,000-row tree fixture passed visited-row unmount, down/back,
  wrapper bounds and full-rendering preference. The initial fixture needed to
  account for clicking the footer preference scrolling to the end, and to
  navigate away before deleting fixtures to avoid teardown-request 404s.
- Native backlog drag fixture passes auto-scroll beyond the initial viewport,
  retained offscreen source, reordered visible neighbors and reload persistence.
  Frame-scale viewport polling avoids missing a target passing through a moving
  viewport; it does not retry the test or increase its timeout.
- The 1,000-row fixture also passed paired-roadmap mounted-key equality and
  <=1px row alignment, timeline-led scrolling and tree-led return to the start.
- Artifacts, traces, videos and disposable server logs are retained under the
  sibling core-tests `e2e/test-results-*` and this worktree's `.e2e-server-*`.

From frontend, `bun run typecheck` passed with zero errors/warnings;
`bun run check:hmr-boundaries` passed. Browser runner app/embed builds and entry
asset guards passed. `bun run ds:build` passed with existing catalog accessibility
warnings. Reported root shell was 30.31 KiB raw / 12.29 KiB gzip;
this is **not** the shared-base incremental dependency-closure measurement.
Biome was run on changed non-Svelte modules. No Go production code changed.

## Blocking work before ready-for-review / completion

- Complete the large list/search, largest queue page, and 20-section/1,000-item
  backlog fixtures; current native drag proof uses 140 loaded rows.
- Complete cross-section/empty/collapsed/closed-target drag workflows,
  cancellation, deletion/permission/SSE invalidation, rejection and partial
  failure recovery with single-operation assertions.
- Qualify all IME/portalled focus/editor policies, disabled/group-row keyboard
  traversal and deleted/forbidden-row recovery, plus cross-account/workspace
  state isolation through real UI flows.
- Complete variable-height width/font/locale/RTL/terminal split/200%-zoom
  anchor drift measurements, threshold/normal-mode transitions, root replacement,
  back/forward/modal return and asynchronous data-load restoration.
- Extend large paired-roadmap alignment coverage to zoom/panel resizing and
  offscreen-crossing dependency geometry, permission/date-edit invalidation
  and pointer cancellation.
- Roadmap's horizontal header is a JS mirror of native scroll and may lag during
  momentum scrolling. Its horizontal-scrollbar compensation first arrives via
  ResizeObserver. Explicitly check first paint and momentum scroll in Safari/
  WebKit; settled <=1px alignment alone does not qualify those visual risks.
- Finish the remaining named browser matrix from attachment 916; existing
  board/map samples are not the entire regression matrix.
- Capture identical baseline/candidate five-paired-run performance evidence,
  4x-CPU scroll JS/long-task timings, retained heap and the <=20kB incremental
  shared-base gzip closure. No performance budget is claimed satisfied yet.
- Run WebKit or documented Safari/manual accessibility/print checks, and final
  lint/build/unit/browser checks on the exact proposed commits.

These remain on WI-2147; no completion or shipping approval is implied by the
draft PRs or by passing sampled tests.

## PR review follow-up

Findings on core PR #502 are addressed as follows; the release blockers above
remain unchanged:

1. Duplicate/empty keys are repaired with collision-safe positional keys at the
   controller boundary, with one warning per controller. Strict `indexRows`
   validation remains available; malformed occurrences cannot claim stable
   identity through reorder. A rendered malformed-response regression passes.
2. DOM registration uses only pane-qualified storage. Nonvirtual anchor and
   scroll geometry uses the canonical list, not whichever pane attached last.
   Tests attach panes in the adverse order and remove one independently.
3. Drag outcome flags reset on every new gesture. Stable reused drag-data objects
   can commit/cancel then start another gesture; one drop still has one claim.
   Drag source ownership checks the exact registered pane node, not just a key.
4. Restoration identities are caller-supplied surface namespaces. README now
   requires distinct prefixes for independent roots displaying the same query.
5. The first logical tree row again suppresses its top border. Shared row snippet
   metadata exposes its logical index; down/back scrolling tests the boundary.
6. Eager normal/full/print mounting is intentional, not a deferred-render mode.
   Reintroducing LazyRender would defeat Find/copy/accessibility/print. Its cost
   and outstanding 50-row performance qualification are explicit in README.
7. Queue scope now includes accepted search/QL, collection, workspace, page and
   size. Backlog uses its actual effective backlog sub-filter. Tree has no
   free-text search; collapse/expansion deliberately retain a surviving anchor.
8. First-paint scrollbar compensation and JS horizontal-header momentum lag are
   explicitly tracked as visual risks, not declared qualified by settled tests.
9. The viewport registry itself tolerates missing ResizeObserver. A rendered
   conditional-node test confirms attachment effects are disposed even while
   their keyed VirtualRow survives and is reordered.
