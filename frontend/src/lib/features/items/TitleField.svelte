<script>
  import EditableField from '../../editors/EditableField.svelte';
  import Input from '../../components/Input.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  // Shared inline title field. Both the work-item detail header and the
  // personal task detail use it so their title editing cannot drift.
  let {
    value = '',
    editable = true,
    oncommit = null,
    displayClass = '',
    inputClass = '',
    displayTestid = undefined,
    inputTestid = undefined,
    placeholder = null,
    ariaLabel = null,
  } = $props();
</script>

<EditableField {value} {editable} {oncommit}>
  {#snippet display()}
    <span class={displayClass} dir="auto" data-testid={displayTestid}>{value}</span>
  {/snippet}
  {#snippet editor({ draft, setDraft })}
    <Input
      type="text"
      variant="ghost"
      dataTestid={inputTestid}
      value={draft ?? ''}
      oninput={(e) => setDraft(e.currentTarget.value)}
      class={inputClass}
      placeholder={placeholder ?? t('items.enterTitle')}
      ariaLabel={ariaLabel ?? undefined}
      autofocus
    />
  {/snippet}
</EditableField>
