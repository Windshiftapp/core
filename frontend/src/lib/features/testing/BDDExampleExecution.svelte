<script>
  import Loader2 from '@lucide/svelte/icons/loader-2';
  import { tests } from '../../api/tests/index.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Badge from '../../components/Badge.svelte';
  import StatusButton from '../../components/StatusButton.svelte';
  import { parseScenarioSpec, flattenExamples, applyExampleRow } from './bddSpec.js';
  import BDDScenarioView from './BDDScenarioView.svelte';

  /**
   * Execution panel for one BDD case in a run. Results attach to the frozen
   * spec snapshot taken when the case entered the run; later case edits are
   * irrelevant here. One result row per Scenario Outline example, with
   * optional per-step detail behind an expander.
   *
   * @type {{
   *   workspaceId: number | string,
   *   runId: number | string,
   *   testCase: object,
   *   snapshot?: object | null,
   *   initialResults?: Array<object>,
   *   canExecute?: boolean,
   *   onResultsChange?: ((results: Array<object>) => void) | null,
   *   dataTestid?: string,
   * }}
   */
  let {
    workspaceId,
    runId,
    testCase,
    snapshot = null,
    initialResults = [],
    // Recording example results requires test.execute; read-only users just
    // follow along.
    canExecute = true,
    onResultsChange = null,
    dataTestid = 'bdd-example-execution',
  } = $props();

  const spec = $derived(parseScenarioSpec(snapshot?.spec || testCase?.bdd?.spec));
  const rawGherkin = $derived(snapshot?.gherkin || testCase?.bdd?.gherkin || '');
  const exampleRows = $derived(flattenExamples(spec));

  // Execution order for one example: background steps first, then the
  // scenario's own steps — the same numbering the backend validates against.
  const executableSteps = $derived([
    ...(spec?.background || []).map((step) => ({ ...step, isBackground: true })),
    ...(spec?.steps || []).map((step) => ({ ...step, isBackground: false })),
  ]);

  let results = $state({});
  let expanded = $state({});
  let saving = $state({});
  let loadError = $state(null);

  $effect(() => {
    // Re-hydrate whenever the parent hands us a different case's results.
    const next = {};
    for (const result of initialResults) {
      next[result.example_index] = {
        status: result.status || 'not_run',
        actual_result: result.actual_result || '',
        notes: result.notes || '',
        row_values: result.row_values || {},
        step_results: Object.fromEntries(
          (result.step_results || []).map((step) => [step.step_number, step])
        ),
      };
    }
    results = next;
    expanded = {};
    loadError = initialResults.length === 0 && exampleRows.length > 0
      ? t('testing.noExampleResults')
      : null;
  });

  function notifyChange() {
    onResultsChange?.(
      Object.entries(results).map(([index, value]) => ({
        example_index: Number(index),
        ...value,
      }))
    );
  }

  async function saveExample(exampleIndex, fields) {
    saving = { ...saving, [exampleIndex]: true };
    try {
      await tests.testRuns.updateExampleResult(workspaceId, runId, testCase.id, exampleIndex, fields);
      notifyChange();
    } catch (err) {
      console.error('Failed to save example result:', err);
      loadError = err?.message || t('testing.failedToSaveExample');
    } finally {
      saving = { ...saving, [exampleIndex]: false };
    }
  }

  function setExampleStatus(exampleIndex, status) {
    results = {
      ...results,
      [exampleIndex]: { ...(results[exampleIndex] || { row_values: {} }), status },
    };
    saveExample(exampleIndex, { status });
  }

  function setExampleField(exampleIndex, field, value) {
    results = {
      ...results,
      [exampleIndex]: { ...(results[exampleIndex] || { row_values: {} }), [field]: value },
    };
  }

  function commitExampleNotes(exampleIndex, value) {
    setExampleField(exampleIndex, 'notes', value);
    saveExample(exampleIndex, {
      notes: value,
      status: results[exampleIndex]?.status || 'not_run',
      actual_result: results[exampleIndex]?.actual_result || '',
    });
  }

  function toggleSteps(exampleIndex) {
    expanded = { ...expanded, [exampleIndex]: !expanded[exampleIndex] };
  }

  async function saveExampleStep(exampleIndex, stepNumber, fields) {
    saving = { ...saving, [`${exampleIndex}_${stepNumber}`]: true };
    try {
      await tests.testRuns.updateExampleStepResult(
        workspaceId,
        runId,
        testCase.id,
        exampleIndex,
        stepNumber,
        fields
      );
      const example = results[exampleIndex];
      results = {
        ...results,
        [exampleIndex]: {
          ...example,
          step_results: {
            ...(example?.step_results || {}),
            [stepNumber]: { ...(example?.step_results?.[stepNumber] || {}), ...fields },
          },
        },
      };
      notifyChange();
    } catch (err) {
      console.error('Failed to save example step result:', err);
      loadError = err?.message || t('testing.failedToSaveExample');
    } finally {
      saving = { ...saving, [`${exampleIndex}_${stepNumber}`]: false };
    }
  }

  function setExampleStepStatus(exampleIndex, stepNumber, status) {
    saveExampleStep(exampleIndex, stepNumber, {
      status,
      item_id: results[exampleIndex]?.step_results?.[stepNumber]?.item_id ?? null,
    });
  }

  /** @type {Array<{ status: 'passed' | 'failed' | 'blocked' | 'skipped', label: string }>} */
  const statusButtons = [
    { status: 'passed', label: t('testing.pass') },
    { status: 'failed', label: t('testing.fail') },
    { status: 'blocked', label: t('testing.blocked') },
    { status: 'skipped', label: t('testing.skip') },
  ];
</script>

<div class="space-y-4" data-testid={dataTestid}>
  <p class="text-xs" style="color: var(--ds-text-subtle);">
    {t('testing.bddSnapshotNote')}
  </p>

  {#if loadError}
    <p class="text-sm" style="color: var(--ds-text-danger);" data-testid="bdd-example-execution-error">{loadError}</p>
  {/if}

  {#if !spec}
    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('testing.bddSpecUnavailable')}</p>
    {:else}
    {#each exampleRows as row (row.exampleIndex)}
      {@const result = results[row.exampleIndex] || { status: 'not_run', row_values: {} }}
      <div
        class="rounded-lg border p-4 space-y-3"
        style="border-color: var(--ds-border); background-color: var(--ds-surface);"
        data-testid="bdd-example-row"
      >
        <div class="flex items-center justify-between gap-3 flex-wrap">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-sm font-semibold" style="color: var(--ds-text);">
              {t('testing.exampleN', { n: row.exampleIndex + 1 })}
            </span>
            {#each row.header as column, i (column)}
              <Badge size="xs">{column}: {row.row[i] ?? ''}</Badge>
            {/each}
          </div>
          <div class="flex items-center gap-2">
            {#if saving[row.exampleIndex]}
              <Loader2 class="w-4 h-4 animate-spin" style="color: var(--ds-text-subtle);" />
            {/if}
            {#if canExecute}
            {#each statusButtons as button (button.status)}
              <StatusButton
                status={button.status}
                selected={result.status === button.status}
                dataTestid="bdd-example-status-{button.status}"
                onclick={() => setExampleStatus(row.exampleIndex, button.status)}
                class="flex items-center gap-1 px-3 py-1.5 rounded transition cursor-pointer text-sm"
              >
                {button.label}
              </StatusButton>
            {/each}
            {/if}
          </div>
        </div>

        <div>
          <label
            for={`bdd-example-notes-${row.exampleIndex}`}
            class="text-xs font-semibold uppercase tracking-wider"
            style="color: var(--ds-text-subtle);">{t('common.notes')}</label
          >
          <textarea
            id={`bdd-example-notes-${row.exampleIndex}`}
            rows={2}
            class="w-full text-sm p-2 rounded border resize-y"
            style="background-color: var(--ds-surface-raised); color: var(--ds-text); border-color: var(--ds-border);"
            value={result.notes || ''}
            data-testid="bdd-example-notes"
            disabled={!canExecute}
            onblur={(e) => commitExampleNotes(row.exampleIndex, e.currentTarget.value)}
            oninput={(e) => setExampleField(row.exampleIndex, 'notes', e.currentTarget.value)}
          ></textarea>
        </div>

        {#if executableSteps.length > 0}
          <button
            type="button"
            class="text-xs underline"
            style="color: var(--ds-text-subtle);"
            data-testid="bdd-toggle-steps"
            onclick={() => toggleSteps(row.exampleIndex)}
          >
            {expanded[row.exampleIndex]
              ? t('testing.hideExampleSteps')
              : t('testing.showExampleSteps', { count: executableSteps.length })}
          </button>

          {#if expanded[row.exampleIndex]}
            <div class="space-y-3 pl-3 border-l-2" style="border-color: var(--ds-border);" data-testid="bdd-example-steps">
              {#each executableSteps as step, stepIdx (stepIdx)}
                {@const stepNumber = stepIdx + 1}
                {@const stepResult = result.step_results?.[stepNumber] || { status: 'not_run' }}
                <div class="space-y-1">
                  <p class="text-sm">
                    {#if step.isBackground}
                      <span class="text-xs uppercase mr-1" style="color: var(--ds-text-subtle);">{t('testing.background')}</span>
                    {/if}
                    <span class="font-semibold" style="color: var(--ds-interactive);">{step.keyword || ''}</span>
                    <span style="color: var(--ds-text);">
                      {applyExampleRow(step.text, result.row_values)}
                    </span>
                  </p>
                  <div class="flex items-center gap-2 flex-wrap">
                    {#if canExecute}
                      {#each statusButtons as button (button.status)}
                        <button
                          type="button"
                          class="text-xs px-2 py-0.5 rounded border transition cursor-pointer"
                          style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
                          class:selected={stepResult.status === button.status}
                          data-status={button.status}
                          data-testid="bdd-step-status-{button.status}"
                          onclick={() => setExampleStepStatus(row.exampleIndex, stepNumber, button.status)}
                        >
                          {button.label}
                        </button>
                      {/each}
                    {/if}
                    {#if saving[`${row.exampleIndex}_${stepNumber}`]}
                      <Loader2 class="w-3 h-3 animate-spin" style="color: var(--ds-text-subtle);" />
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        {/if}
      </div>
    {/each}

    {#if exampleRows.length > 0}
      <details class="text-sm">
        <summary class="cursor-pointer" style="color: var(--ds-text-subtle);">{t('testing.showSource')}</summary>
        <div class="mt-2">
          <BDDScenarioView {spec} raw={rawGherkin} dataTestid="bdd-execution-scenario-view" />
        </div>
      </details>
    {/if}
  {/if}
</div>

