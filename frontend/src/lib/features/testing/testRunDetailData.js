/** Load and normalize the complete read model shared by both run screens. */
export async function loadTestRunDetail(apiClient, workspaceId, runId) {
  const detail = await apiClient.tests.testRuns.getDetail(workspaceId, runId);
  if (!detail?.run) throw new Error('Test run not found');

  const snapshots = Array.isArray(detail.bdd_snapshots) ? detail.bdd_snapshots : [];
  const specsByCase = Object.fromEntries(
    snapshots.map((snapshot) => {
      let spec = null;
      try {
        spec = JSON.parse(snapshot.spec || '{}');
      } catch {
        spec = null;
      }
      return [snapshot.test_case_id, spec];
    })
  );

  return {
    run: detail.run,
    testCases: Array.isArray(detail.test_cases)
      ? detail.test_cases.map((testCase) => ({
          ...testCase,
          test_steps: Array.isArray(testCase?.test_steps) ? testCase.test_steps : [],
          bddSpec: specsByCase[testCase.id] ?? null,
        }))
      : [],
    results: Array.isArray(detail.results) ? detail.results : [],
    stepResults: Object.fromEntries(
      (Array.isArray(detail.step_results) ? detail.step_results : []).map((result) => [
        `${result.test_case_id}_${result.step_id}`,
        result,
      ])
    ),
    // One result row per example (or the implicit single row for plain BDD
    // scenarios), keyed by `${testCaseId}_${exampleIndex}`.
    exampleResults: Object.fromEntries(
      (Array.isArray(detail.example_results) ? detail.example_results : []).map((result) => [
        `${result.test_case_id}_${result.example_index}`,
        result,
      ])
    ),
  };
}
