<script>
  // Editor for one main-column custom field. It mirrors the description editor:
  // a local draft, explicit Save/Cancel, and the description's keyboard
  // shortcuts. Nothing commits on outside clicks, so toolbar menus and
  // neighbouring sections never end the edit, and the draft is never pushed
  // back into the editor while the user types.
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { getDisplayString, getShortcut, matchesShortcut } from '../../utils/keyboardShortcuts.js';
  import { customFieldPlaceholder, isRichTextCustomFieldType } from '../../utils/customFieldTypes.js';

  let {
    field,
    initialValue = '',
    workspaceId = null,
    saving = false,
    onsave = undefined,
    oncancel = undefined,
    // Optional per-screen hint; overrides the field description as placeholder.
    hint = '',
  } = $props();

  const saveShortcut = getShortcut('description', 'save');
  const cancelShortcut = getShortcut('description', 'cancel');

  // Captured once: the editor owns the document until Save or Cancel.
  // svelte-ignore state_referenced_locally
  let draft = $state(initialValue ?? '');
  let richtextEditor = $state(null);
  // True while this editor's own save is in flight. The input is locked so
  // nothing typed after Save can be discarded when the save closes the editor.
  let submitting = $state(false);

  const isRichText = $derived(isRichTextCustomFieldType(field.field_type));
  const isSingleLine = $derived(field.field_type === 'text');
  // The screen's hint, else "Enter <field>".
  const placeholder = $derived(
    customFieldPlaceholder(field, hint, t('items.enterField', { field: field.name.toLowerCase() }))
  );

  async function save() {
    // Same lock as the Save button, so the keyboard shortcut cannot submit
    // while another field's save is pending or this one is in flight.
    if (saving || submitting) return;
    // Milkdown's change listener is debounced; read the live document so the
    // last keystroke is never lost.
    const value = isRichText ? (richtextEditor?.getMarkdown?.() ?? draft) : draft;
    submitting = true;
    try {
      await onsave?.(value);
    } finally {
      // A successful save closes the editor; a failed one leaves it open with
      // the draft intact and editable again.
      submitting = false;
    }
  }

  function handleKeydown(event) {
    if (matchesShortcut(event, saveShortcut)) {
      event.preventDefault();
      save();
    } else if (matchesShortcut(event, cancelShortcut)) {
      event.preventDefault();
      event.stopPropagation(); // keep Escape from closing the item modal
      oncancel?.();
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="space-y-3" onkeydown={handleKeydown} data-testid={`item-main-custom-field-editor-${field.id}`}>
  <div inert={submitting} aria-busy={submitting}>
    {#if isRichText}
      <MilkdownEditor
        bind:this={richtextEditor}
        bind:content={draft}
        {placeholder}
        readonly={false}
        showToolbar={true}
        allowImageUpload={false}
        {workspaceId}
        testId={`item-main-custom-field-input-${field.id}`}
      />
    {:else if isSingleLine}
      <Input
        bind:value={draft}
        {placeholder}
        autofocus
        dataTestid={`item-main-custom-field-input-${field.id}`}
      />
    {:else}
      <Textarea
        bind:value={draft}
        {placeholder}
        rows={6}
        autofocus
        data-testid={`item-main-custom-field-input-${field.id}`}
      />
    {/if}
  </div>
  <div class="flex items-center gap-2">
    <Button
      variant="primary"
      onclick={save}
      disabled={saving || submitting}
      keyboardHint={getDisplayString(saveShortcut)}
      dataTestid={`item-main-custom-field-save-${field.id}`}
    >
      {t('common.save')}
    </Button>
    <Button variant="default" onclick={() => oncancel?.()} dataTestid={`item-main-custom-field-cancel-${field.id}`}>
      {t('common.cancel')}
    </Button>
  </div>
</div>
