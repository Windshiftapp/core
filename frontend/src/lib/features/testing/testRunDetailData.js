import { tests } from '../../api/tests/index.js';
/** Load and normalize the complete read model shared by both run screens. */
export async function loadTestRunDetail(workspaceId, runId) {
  const detail = await tests.testRuns.getDetail(workspaceId, runId);
  if (!detail?.run) throw new Error('Test run not found');

  return {
    run: detail.run,
    testCases: Array.isArray(detail.test_cases)
      ? detail.test_cases.map((testCase) => ({
          ...testCase,
          test_steps: Array.isArray(testCase?.test_steps) ? testCase.test_steps : [],
        }))
      : [],
    results: Array.isArray(detail.results) ? detail.results : [],
    stepResults: Object.fromEntries(
      (Array.isArray(detail.step_results) ? detail.step_results : []).map((result) => [
        `${result.test_case_id}_${result.step_id}`,
        result,
      ])
    ),
    // BDD execution state: run-frozen scenario specs and per-example results.
    bddSnapshots: Object.fromEntries(
      (Array.isArray(detail.bdd_snapshots) ? detail.bdd_snapshots : []).map((snapshot) => [
        snapshot.test_case_id,
        snapshot,
      ])
    ),
    bddExampleResults: detail.example_results ?? [],
  };
}
