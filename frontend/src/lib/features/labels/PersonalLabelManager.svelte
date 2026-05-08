<script>
  import { onMount } from 'svelte';
  import { Tag, Plus, Pencil, Trash2, Check, X } from 'lucide-svelte';
  import { api } from '../../api.js';
  import { authStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import Input from '../../components/Input.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import IconSelector from '../../pickers/IconSelector.svelte';

  let labels = $state(/** @type {any[]} */ ([]));
  let loading = $state(true);
  let search = $state('');
  let showForm = $state(false);
  let editingId = $state(/** @type {number | null} */ (null));
  let formName = $state('');
  let formColor = $state('#3B82F6');
  let saving = $state(false);

  let currentUserId = $derived(authStore.currentUser?.id ?? null);

  // Profile → Labels is the *personal* labels manager. Shared labels
  // (`user_id === null`) are still returned by the API (the item label
  // picker needs them) but are not editable from this surface, so the
  // list filters down to the current user's own rows.
  let filtered = $derived.by(() => {
    const mineOnly = labels.filter((l) => l.user_id != null);
    const q = search.trim().toLowerCase();
    if (!q) return mineOnly;
    return mineOnly.filter((l) => l.name?.toLowerCase().includes(q));
  });

  onMount(() => {
    void load();
  });

  async function load() {
    loading = true;
    try {
      const response = await api.personalLabels.getAll();
      labels = response || [];
    } catch (err) {
      console.error('Failed to load personal labels:', err);
      errorToast(err?.message || 'Failed to load labels');
      labels = [];
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    editingId = null;
    formName = '';
    formColor = '#3B82F6';
    showForm = true;
  }

  function openEdit(label) {
    editingId = label.id;
    formName = label.name;
    formColor = label.color || '#3B82F6';
    showForm = true;
  }

  function closeForm() {
    showForm = false;
    editingId = null;
    formName = '';
    formColor = '#3B82F6';
  }

  async function submitForm(e) {
    e?.preventDefault?.();
    const name = formName.trim();
    if (!name) {
      errorToast(t('common.nameRequired') || 'Name is required');
      return;
    }
    const payload = {
      name,
      color: formColor,
      // Always personal — Profile manager never mints shared labels.
      user_id: currentUserId
    };
    saving = true;
    try {
      if (editingId == null) {
        await api.personalLabels.create(payload);
        successToast(t('users.labels.created') || 'Label created');
      } else {
        await api.personalLabels.update(editingId, payload);
        successToast(t('users.labels.updated') || 'Label updated');
      }
      closeForm();
      await load();
    } catch (err) {
      console.error('Failed to save label:', err);
      errorToast(err?.message || 'Failed to save label');
    } finally {
      saving = false;
    }
  }

  async function deleteLabel(label) {
    const ok = await confirm({
      title: t('users.labels.deleteTitle') || 'Delete label?',
      message:
        t('users.labels.deleteMessage', { name: label.name }) ||
        `Delete "${label.name}"? This will remove it from any items it is attached to.`,
      confirmText: t('common.delete') || 'Delete',
      variant: 'danger'
    });
    if (!ok) return;
    try {
      await api.personalLabels.delete(label.id);
      successToast(t('users.labels.deleted') || 'Label deleted');
      await load();
    } catch (err) {
      console.error('Failed to delete label:', err);
      errorToast(err?.message || 'Failed to delete label');
    }
  }
</script>

<div class="space-y-4">
  <div class="flex items-center justify-between gap-2">
    <Input
      placeholder={t('users.labels.searchPlaceholder') || 'Search labels'}
      bind:value={search}
      size="small"
      class="flex-1"
    />
    <Button
      variant="primary"
      size="small"
      icon={Plus}
      onclick={openCreate}
    >
      {t('users.labels.new') || 'New label'}
    </Button>
  </div>

  {#if showForm}
    <form
      onsubmit={submitForm}
      class="rounded border p-4 space-y-3"
      style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);"
    >
      <h4 class="font-medium" style="color: var(--ds-text);">
        {editingId == null
          ? (t('users.labels.createTitle') || 'Create label')
          : (t('users.labels.editTitle') || 'Edit label')}
      </h4>
      <div class="flex gap-3 items-start">
        <div class="flex-1">
          <Label class="block text-xs font-medium mb-1">{t('common.name') || 'Name'}</Label>
          <Input
            bind:value={formName}
            placeholder={t('users.labels.namePlaceholder') || 'e.g. urgent'}
            required
            size="small"
          />
        </div>
        <div>
          <Label class="block text-xs font-medium mb-1">{t('common.color') || 'Color'}</Label>
          <IconSelector bind:selectedColor={formColor} colorOnly compact />
        </div>
      </div>
      <div class="flex gap-2 pt-1">
        <Button type="submit" variant="primary" size="small" disabled={saving} icon={Check}>
          {editingId == null
            ? (t('common.create') || 'Create')
            : (t('common.save') || 'Save')}
        </Button>
        <Button type="button" variant="default" size="small" onclick={closeForm} icon={X}>
          {t('common.cancel') || 'Cancel'}
        </Button>
      </div>
    </form>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-8">
      <Spinner size="md" />
    </div>
  {:else if filtered.length === 0}
    <div class="rounded border p-8 text-center" style="border-color: var(--ds-border);">
      <Tag class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-text-subtle);" />
      <div class="text-sm" style="color: var(--ds-text);">
        {t('users.labels.empty') || 'No labels yet'}
      </div>
      <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
        {t('users.labels.emptyHint') || 'Create your first label to organize items.'}
      </div>
    </div>
  {:else}
    {@render labelGroup(t('users.labels.mine') || 'My labels', filtered)}
  {/if}
</div>

{#snippet labelGroup(title, items)}
  {#if items.length > 0}
    <section class="space-y-2">
      <h5 class="text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">
        {title}
      </h5>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        {#each items as label (label.id)}
          <div
            class="flex items-center gap-3 px-3 py-2 rounded border"
            style="background-color: var(--ds-surface); border-color: var(--ds-border);"
            data-testid="personal-label-row"
          >
            <span
              class="inline-block w-3 h-3 rounded-full flex-shrink-0"
              style="background-color: {label.color || '#3B82F6'};"
              aria-hidden="true"
            ></span>
            <span class="flex-1 min-w-0 truncate text-sm" style="color: var(--ds-text);">
              {label.name}
            </span>
            <button
              type="button"
              class="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)] transition-colors"
              aria-label={t('common.edit') || 'Edit'}
              onclick={() => openEdit(label)}
            >
              <Pencil class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            </button>
            <button
              type="button"
              class="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)] transition-colors"
              aria-label={t('common.delete') || 'Delete'}
              onclick={() => deleteLabel(label)}
            >
              <Trash2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            </button>
          </div>
        {/each}
      </div>
    </section>
  {/if}
{/snippet}
