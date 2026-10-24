<script>
  import Text from '../../components/Text.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    value = null,
    editable = false,
    rollup = null,
    onSave = () => {},
  } = $props();

  let editing = $state(false);
  let editValue = $state('');
  let error = $state(false);

  function startEdit() {
    if (!editable) return;
    editValue = value == null ? '' : String(value);
    error = false;
    editing = true;
  }

  function save(event) {
    // A masked type=number field (browser badInput) must not read as an
    // intentional clear.
    if (event?.currentTarget?.validity?.badInput) {
      error = true;
      return;
    }
    const raw = String(editValue ?? '').trim();
    if (raw === '') {
      // Empty input is an explicit clear.
      error = false;
      editing = false;
      if (value != null) onSave(null);
      return;
    }
    const parsed = Number(raw);
    if (!Number.isFinite(parsed) || parsed < 0) {
      // Reject invalid input; stay editing until fixed or escaped.
      error = true;
      return;
    }
    error = false;
    editing = false;
    if (parsed !== (value ?? null)) onSave(parsed);
  }

  function cancel() {
    editing = false;
    error = false;
  }
</script>

<div class="mb-1" data-testid="item-story-points-field">
  {#if editing}
    <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm">
      <Text variant="subtle" size="sm">{t('items.storyPoints')}</Text>
      <!-- svelte-ignore a11y_autofocus -->
      <input
        data-testid="story-points-input"
        type="number"
        step="0.5"
        min="0"
        aria-label={t('items.storyPoints')}
        aria-invalid={error}
        aria-describedby={error ? 'story-points-input-error' : undefined}
        class="w-20 text-right text-sm px-1.5 py-0.5 border-0 bg-transparent focus:outline-none focus:ring-0 transition-all duration-200"
        style="color: {error ? 'var(--ds-text-danger, #cc3344)' : 'var(--ds-text)'};"
        value={editValue ?? ''}
        onfocus={(e) => e.currentTarget.select()}
        oninput={(e) => { editValue = e.currentTarget.value; error = false; }}
        onblur={save}
        onkeydown={(e) => {
          if (e.key === 'Enter') { e.currentTarget.blur(); }
          if (e.key === 'Escape') { cancel(); }
        }}
        autofocus
      />
      {#if error}
        <span id="story-points-input-error" class="sr-only">
          {t('items.enterField', { field: t('items.storyPoints') })}
        </span>
      {/if}
    </div>
  {:else}
    <button
      onclick={startEdit}
      class="hover-bg w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
      disabled={!editable}
    >
      <Text variant="subtle" size="sm">{t('items.storyPoints')}</Text>
      <div class="flex items-center gap-2">
        {#if value != null && value !== 0}
          <span style="color: var(--ds-text);">{value}</span>
        {:else}
          <Text variant="subtle" size="sm">{t('common.none')}</Text>
        {/if}
      </div>
    </button>
    {#if rollup?.contributors > 0}
      <p class="text-xs mt-0.5 text-right px-2" style="color: var(--ds-text-subtle);" data-testid="story-points-child-rollup">
        {t('items.storyPointsChildRollup', { points: rollup.points, count: rollup.contributors, plural: rollup.contributors === 1 ? '' : 's' })}
      </p>
    {/if}
  {/if}
</div>
