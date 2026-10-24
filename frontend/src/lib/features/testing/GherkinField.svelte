<script>
  import Loader2 from '@lucide/svelte/icons/loader-2';
  import CheckCircle2 from '@lucide/svelte/icons/check-circle-2';
  import XCircle from '@lucide/svelte/icons/x-circle';
  import { tests } from '../../api/tests/index.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Button from '../../components/Button.svelte';

  /**
   * Authoring field for a BDD test case's Gherkin source. Plain textarea on
   * purpose — validation and structured errors come from the backend, which
   * owns the Gherkin grammar.
   *
   * @type {{
   *   value?: string,
   *   workspaceId?: number | null,
   *   disabled?: boolean,
   *   dataTestid?: string,
   * }}
   */
  let { value = $bindable(''), workspaceId = null, disabled = false, dataTestid = 'test-case-gherkin' } = $props();

  let validating = $state(false);
  let result = $state(null); // { valid, errors, document }
  let validationError = $state(null);
  // Tracks which content the latest result describes so stale verdicts are
  // never shown after edits.
  let validatedContent = $state('');

  let hasEditsSinceValidation = $derived(validatedContent !== '' && validatedContent !== value);

  async function validate() {
    if (!workspaceId || !value.trim() || validating) return;
    validating = true;
    validationError = null;
    try {
      result = await tests.testCases.validateFeature(workspaceId, value);
      validatedContent = value;
    } catch (err) {
      result = null;
      validatedContent = '';
      validationError = err?.message || t('testing.gherkinValidationFailed');
    } finally {
      validating = false;
    }
  }

  const errors = $derived(result?.errors || []);
  const documentSummary = $derived(result?.document
    ? {
        featureName: result.document.feature_name || '',
        scenarioCount: result.document.scenarios?.length || 0,
      }
    : null);
</script>

<div class="space-y-2">
  <textarea
    bind:value
    disabled={disabled}
    rows={12}
    spellcheck="false"
    class="w-full font-mono text-xs leading-5 p-3 rounded-lg border resize-y"
    style="background-color: var(--ds-surface-raised); color: var(--ds-text); border-color: var(--ds-border);"
    placeholder={'Feature: …\n  Scenario: …\n    Given …\n    When …\n    Then …'}
    data-testid="{dataTestid}-source"
  ></textarea>

  <div class="flex items-center gap-3">
    <Button
      variant="default"
      size="small"
      disabled={disabled || !value.trim() || validating}
      onclick={validate}
      dataTestid="{dataTestid}-validate"
    >
      {#if validating}
        <Loader2 class="w-4 h-4 animate-spin" />
      {/if}
      {t('testing.validateGherkin')}
    </Button>

    {#if result && !hasEditsSinceValidation}
      {#if result.valid}
        <span class="flex items-center gap-1 text-sm" style="color: var(--ds-text-success);" data-testid="{dataTestid}-valid">
          <CheckCircle2 class="w-4 h-4" />
          {documentSummary?.scenarioCount === 1
            ? t('testing.gherkinValidScenario', { scenario: result.document.scenarios[0].name })
            : t('testing.gherkinValid')}
        </span>
      {:else}
        <span class="flex items-center gap-1 text-sm" style="color: var(--ds-text-danger);" data-testid="{dataTestid}-invalid">
          <XCircle class="w-4 h-4" />
          {t('testing.gherkinInvalidCount', { count: errors.length })}
        </span>
      {/if}
    {:else if hasEditsSinceValidation}
      <span class="text-xs" style="color: var(--ds-text-subtle);" data-testid="{dataTestid}-stale">{t('testing.gherkinChangedSinceValidation')}</span>
    {/if}
  </div>

  {#if validationError}
    <p class="text-sm" style="color: var(--ds-text-danger);" data-testid="{dataTestid}-request-error">
      {validationError}
    </p>
  {/if}

  {#if result && !hasEditsSinceValidation && errors.length > 0}
    <ul class="space-y-1" data-testid="{dataTestid}-errors">
      {#each errors as err, i (i)}
        <li class="text-sm font-mono" style="color: var(--ds-text-danger);">
          {t('testing.gherkinErrorLocation', { line: err.line, column: err.column })}: {err.message}
        </li>
      {/each}
    </ul>
  {/if}
</div>
