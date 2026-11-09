<script>
  import Text from '../../components/Text.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import EditableField from '../../editors/EditableField.svelte';

  let {
    value = null,
    editable = false,
    rollup = null,
    onSave = () => {},
  } = $props();

  // A masked type=number field (browser badInput) must not read as an
  // intentional clear. The wrapper owns the draft, so the input reports the
  // bad state here before blur commits an empty string.
  let badInput = $state(false);

  function commit(draft) {
    if (badInput) {
      badInput = false;
      throw new Error(t('items.enterField', { field: t('items.storyPoints') }));
    }
    const raw = String(draft ?? '').trim();
    if (raw === '') {
      if (value != null) onSave(null);
      return;
    }
    const parsed = Number(raw);
    if (!Number.isFinite(parsed) || parsed < 0) {
      throw new Error(t('items.enterField', { field: t('items.storyPoints') }));
    }
    if (parsed !== (value ?? null)) onSave(parsed);
  }
</script>

<div class="mb-1" data-testid="item-story-points-field">
  <EditableField
    value={value}
    editable={editable}
    oncommit={commit}
    onstartedit={() => (badInput = false)}
  >
    {#snippet display()}
      <div class="hover-bg group flex w-full items-center justify-between gap-2 rounded px-2 py-1.5 text-sm transition-colors">
        <Text variant="subtle" size="sm">{t('items.storyPoints')}</Text>
        <div class="flex items-center gap-2">
          {#if value != null && value !== 0}
            <span style="color: var(--ds-text);">{value}</span>
          {:else}
            <Text variant="subtle" size="sm">{t('common.none')}</Text>
          {/if}
        </div>
      </div>
    {/snippet}
    {#snippet editor({ draft, setDraft, error })}
      <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm">
        <Text variant="subtle" size="sm">{t('items.storyPoints')}</Text>
        <!-- svelte-ignore a11y_autofocus -->
        <input
          data-testid="story-points-input"
          type="number"
          step="0.5"
          min="0"
          aria-label={t('items.storyPoints')}
          aria-invalid={error ? 'true' : 'false'}
          aria-describedby={error ? 'story-points-input-error' : undefined}
          class="w-20 text-right text-sm px-1.5 py-0.5 border-0 bg-transparent focus:outline-none focus:ring-0 transition-all duration-200"
          style="color: {error ? 'var(--ds-text-danger, #cc3344)' : 'var(--ds-text)'};"
          value={draft ?? ''}
          onfocus={(e) => e.currentTarget.select()}
          oninput={(e) => {
            badInput = e.currentTarget.validity.badInput;
            setDraft(e.currentTarget.value);
          }}
          autofocus
        />
        {#if error}
          <span id="story-points-input-error" class="sr-only">{error}</span>
        {/if}
      </div>
    {/snippet}
  </EditableField>
  {#if rollup?.contributors > 0}
    <p
      class="text-xs mt-0.5 text-right px-2"
      style="color: var(--ds-text-subtle);"
      data-testid="story-points-child-rollup"
    >
      {t('items.storyPointsChildRollup', {
        points: rollup.points,
        count: rollup.contributors,
        plural: rollup.contributors === 1 ? '' : 's',
      })}
    </p>
  {/if}
</div>
