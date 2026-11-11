<script>
  // Right-hand inspector for one workflow transition. This slice owns the
  // Screen section: bind, unbind, or create a screen and bind it. The binding
  // saves immediately through PUT /transitions/{id}/screen, so it is never part
  // of the staged graph save (WI-1835).
  import X from '@lucide/svelte/icons/x';
  import Monitor from '@lucide/svelte/icons/monitor';
  import Plus from '@lucide/svelte/icons/plus';
  import { transitions as transitionsApi } from '../../api/transitions.js';
  import { screens as screensApi } from '../../api/configuration.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import ScreenFieldLayoutEditor from '../../editors/ScreenFieldLayoutEditor.svelte';

  let { transition, statuses = [], onClose = null, onChanged = null } = $props();

  let governance = $state(null);
  let screens = $state([]);
  let loading = $state(false);
  let saving = $state(false);
  let creating = $state(false);
  let newScreenName = $state('');
  // The screen whose fields are open in the layout editor, and the editor
  // instance so the modal footer can drive its save.
  let fieldEditorScreen = $state(null);
  let fieldEditorRef = $state(null);

  let loadToken = 0;
  // Identity of the transition the inspector last loaded, so a staged binding
  // that rewrites `transition.screen` does not refetch governance/screens.
  let loadedKey = null;

  // An edge drawn in this session has no transition id yet, so its binding is
  // staged and applied when the workflow graph is saved.
  const pending = $derived(!transition?.transitionId);

  $effect(() => {
    const id = transition?.transitionId;
    const key = `${id ?? 'new'}:${transition?.edgeId ?? ''}`;
    if (key === loadedKey) return;
    loadedKey = key;

    const token = ++loadToken;
    loading = true;
    const requests = [screensApi.getAllWithFields()];
    if (id) requests.push(transitionsApi.governance(id));
    Promise.all(requests)
      .then(([list, gov]) => {
        if (token !== loadToken) return;
        screens = list || [];
        governance = id ? gov : null;
      })
      .catch(() => {
        if (token === loadToken) governance = null;
      })
      .finally(() => {
        if (token === loadToken) loading = false;
      });
  });

  const screen = $derived(governance?.screen ?? transition?.screen ?? null);

  const fromName = $derived(
    statuses.find((status) => status.id === transition?.from_status_id)?.name || '?'
  );
  const toName = $derived(
    statuses.find((status) => status.id === transition?.to_status_id)?.name || '?'
  );

  // The screens list has no counts, so derive them from the fields when the
  // list was fetched with them. Keeps the staged summary honest for an edge
  // that has no governance row yet.
  function screenSummary(target) {
    if (!target) return null;
    const fields = target.fields ?? [];
    return {
      id: target.id,
      name: target.name,
      field_count: target.field_count ?? fields.length,
      required_field_count:
        target.required_field_count ?? fields.filter((field) => field.is_required).length,
    };
  }

  async function bindScreen(screenId) {
    if (saving) return;

    // Unsaved edges: stage the choice locally and let the designer apply it
    // once the graph save has assigned the transition a row id.
    if (pending) {
      const found = screenId == null ? null : screens.find((item) => item.id === screenId) || null;
      onChanged?.({ edgeId: transition?.edgeId, screen: screenSummary(found) });
      successToast(t('workflowInspector.screenStaged'));
      return;
    }

    if (!transition?.transitionId) return;
    saving = true;
    try {
      governance = await transitionsApi.setScreen(transition.transitionId, screenId);
      successToast(t('workflowInspector.screenSaved'));
      onChanged?.({ transitionId: transition.transitionId, screen: governance?.screen || null });
    } catch (err) {
      errorToast(err?.message || t('workflowInspector.screenSaveFailed'));
    } finally {
      saving = false;
    }
  }

  function openFieldEditor(target) {
    if (target?.id) fieldEditorScreen = { ...target };
  }

  function closeFieldEditor() {
    fieldEditorScreen = null;
  }

  // Saving the layout refreshes the field counts shown next to the screen.
  async function saveFieldEditor() {
    const target = fieldEditorScreen;
    try {
      const summary = await fieldEditorRef?.save();
      fieldEditorScreen = null;
      if (!target?.id || !summary) return;
      if (transition?.transitionId) {
        governance = await transitionsApi.governance(transition.transitionId);
        return;
      }
      if (screen?.id === target.id) {
        onChanged?.({ edgeId: transition?.edgeId, screen: { ...screen, ...summary } });
      }
    } catch (err) {
      errorToast(err?.message || t('workflowInspector.screenSaveFailed'));
    }
  }

  async function createAndBind() {
    const name = newScreenName.trim();
    if (!name || creating) return;
    creating = true;
    try {
      const created = await screensApi.create({ name });
      newScreenName = '';
      // A new screen always carries the always-visible system fields, so fetch
      // them to show accurate counts before the editor saves anything.
      const createdFields = await screensApi.getFields(created.id);
      const createdWithFields = { ...created, fields: createdFields ?? [] };
      screens = [...screens, createdWithFields];
      await bindScreen(created.id);
      // Open the layout editor so a brand-new screen gets its fields without a
      // detour to the Screens page.
      openFieldEditor(createdWithFields);
    } catch (err) {
      errorToast(err?.message || t('workflowInspector.screenCreateFailed'));
    } finally {
      creating = false;
    }
  }
</script>

<aside
  class="w-96 flex-shrink-0 border-l overflow-y-auto min-w-0"
  style="border-color: var(--workflow-border); background-color: var(--workflow-panel); color: var(--workflow-text);"
  data-testid="transition-inspector"
>
  <div class="flex items-center justify-between px-4 py-3 border-b" style="border-color: var(--workflow-border);">
    <div class="min-w-0">
      <div class="text-sm font-medium truncate" data-testid="transition-inspector-title">
        {fromName} → {toName}
      </div>
      <div class="text-xs" style="color: var(--workflow-text-subtle);">
        {t('workflowInspector.subtitle')}
      </div>
    </div>
    <button
      type="button"
      class="p-1.5 rounded border workflow-icon-button cursor-pointer"
      aria-label={t('common.close')}
      onclick={onClose}
    >
      <X class="w-4 h-4" />
    </button>
  </div>

  <div class="p-4 space-y-4">
    {#if loading}
      <div class="text-sm" style="color: var(--workflow-text-subtle);">{t('common.loading')}</div>
    {:else}
      <section data-testid="transition-inspector-screen">
        <div class="flex items-center gap-2 mb-2">
          <Monitor class="w-4 h-4" />
          <span class="text-sm font-medium">{t('workflowInspector.screen')}</span>
        </div>

        {#if pending}
          <div
            class="text-xs rounded border px-2 py-1.5 mb-2"
            style="border-color: var(--workflow-border); color: var(--workflow-text-subtle);"
            data-testid="transition-inspector-pending"
          >
            {t('workflowInspector.pendingHint')}
          </div>
        {/if}

        {#if screen}
          <div class="rounded border p-3 mb-2" style="border-color: var(--workflow-border);">
            <div class="text-sm font-medium" data-testid="transition-inspector-screen-name">{screen.name}</div>
            <div class="text-xs mt-0.5" style="color: var(--workflow-text-subtle);" data-testid="transition-inspector-field-summary">
              {t('workflowInspector.fieldSummary', { fields: screen.field_count, required: screen.required_field_count })}
            </div>
            <button
              type="button"
              class="mt-2 text-xs underline cursor-pointer"
              style="color: var(--workflow-accent);"
              data-testid="transition-inspector-edit-fields"
              onclick={() => openFieldEditor(screen)}
            >
              {t('workflowInspector.editFields')}
            </button>
          </div>
          <div class="flex gap-2 min-w-0">
            <select
              class="flex-1 min-w-0 text-sm rounded border px-2 py-1.5"
              style="border-color: var(--workflow-border); background-color: var(--workflow-surface); color: var(--workflow-text);"
              disabled={saving}
              data-testid="transition-inspector-screen-picker"
              onchange={(e) => e.currentTarget.value && bindScreen(Number(e.currentTarget.value))}
            >
              <option value="">{t('workflowInspector.changeScreen')}</option>
              {#each screens as option (option.id)}
                <option value={option.id}>{option.name}</option>
              {/each}
            </select>
            <button
              type="button"
              class="px-2 py-1.5 text-sm rounded border cursor-pointer flex-shrink-0"
              style="border-color: var(--workflow-border);"
              disabled={saving}
              data-testid="transition-inspector-unbind"
              onclick={() => bindScreen(null)}
            >
              {t('workflowInspector.unbind')}
            </button>
          </div>
        {:else}
          <div class="text-sm mb-2" style="color: var(--workflow-text-subtle);" data-testid="transition-inspector-no-screen">
            {t('workflowInspector.noScreen')}
          </div>
          <select
            class="w-full text-sm rounded border px-2 py-1.5"
            style="border-color: var(--workflow-border); background-color: var(--workflow-surface); color: var(--workflow-text);"
            disabled={saving}
            data-testid="transition-inspector-screen-picker"
            onchange={(e) => e.currentTarget.value && bindScreen(Number(e.currentTarget.value))}
          >
            <option value="">{t('workflowInspector.chooseScreen')}</option>
            {#each screens as option (option.id)}
              <option value={option.id}>{option.name}</option>
            {/each}
          </select>
        {/if}

        <div class="mt-4 pt-3 border-t" style="border-color: var(--workflow-border);">
          <div class="text-xs font-medium" style="color: var(--workflow-text);">
            {t('workflowInspector.createHeading')}
          </div>
          <p class="text-xs mt-0.5 mb-2" style="color: var(--workflow-text-subtle);">
            {t('workflowInspector.createHint')}
          </p>
          <div class="flex gap-2 min-w-0">
            <input
              class="flex-1 min-w-0 text-sm rounded border px-2 py-1.5"
              style="border-color: var(--workflow-border); background-color: var(--workflow-surface); color: var(--workflow-text);"
              placeholder={t('workflowInspector.newScreenPlaceholder')}
              bind:value={newScreenName}
              data-testid="transition-inspector-new-screen-name"
            />
            <button
              type="button"
              class="flex items-center gap-1 px-2 py-1.5 text-sm rounded border cursor-pointer flex-shrink-0 disabled:opacity-50"
              style="border-color: var(--workflow-border);"
              disabled={creating || !newScreenName.trim()}
              data-testid="transition-inspector-create-screen"
              onclick={createAndBind}
            >
              <Plus class="w-3.5 h-3.5" />
              {t('workflowInspector.createAndBind')}
            </button>
          </div>
        </div>
      </section>
    {/if}
  </div>
</aside>

{#if fieldEditorScreen}
  <Modal
    isOpen={true}
    onclose={closeFieldEditor}
    maxWidth="max-w-5xl"
    onSubmit={saveFieldEditor}
    dataTestid="transition-screen-field-editor"
  >
    <ModalHeader
      title={t('workflowInspector.editFieldsTitle', { name: fieldEditorScreen.name })}
      showCloseButton={false}
    />
    <div class="px-6 py-4">
      <ScreenFieldLayoutEditor bind:this={fieldEditorRef} screen={fieldEditorScreen} />
    </div>
    <DialogFooter
      onCancel={closeFieldEditor}
      onConfirm={saveFieldEditor}
      confirmLabel={t('screensPage.saveFields')}
      showKeyboardHint
    />
  </Modal>
{/if}
