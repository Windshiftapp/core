<script>
  import { IconPlus, IconChevronUp, IconChevronDown, IconTrash, IconX, IconUserPlus, IconEdit } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';

  let { schedule, canEdit, onChange } = $props();

  let editingLayer = $state(null);
  let formData = $state(emptyLayer());
  let memberStaging = $state([]); // ordered list of {id, name, email} for the layer being edited
  let memberPickerValue = $state(null);
  let showLayerModal = $state(false);

  const rotationOptions = [
    { value: 'daily', label: t('teams.oncall.daily') },
    { value: 'weekly', label: t('teams.oncall.weekly') },
    { value: 'custom', label: t('teams.oncall.custom') },
  ];

  function emptyLayer() {
    return {
      name: '',
      priority: 1,
      rotation_type: 'weekly',
      rotation_interval_days: 7,
      handoff_time: '09:00',
      start_date: new Date().toISOString().slice(0, 10),
      end_date: '',
    };
  }

  function startCreate() {
    editingLayer = null;
    formData = emptyLayer();
    formData.priority = (schedule.layers?.length ?? 0) + 1;
    memberStaging = [];
    showLayerModal = true;
  }

  function startEdit(layer) {
    editingLayer = layer;
    formData = {
      name: layer.name,
      priority: layer.priority,
      rotation_type: layer.rotation_type,
      rotation_interval_days: layer.rotation_interval_days,
      handoff_time: layer.handoff_time,
      start_date: layer.start_date,
      end_date: layer.end_date || '',
    };
    memberStaging = (layer.members || [])
      .slice()
      .sort((a, b) => a.position - b.position)
      .map((m) => ({ id: m.user_id, name: m.user_name, email: m.user_email }));
    showLayerModal = true;
  }

  function close() {
    showLayerModal = false;
    editingLayer = null;
    formData = emptyLayer();
    memberStaging = [];
    memberPickerValue = null;
  }

  function moveMember(index, delta) {
    const newIndex = index + delta;
    if (newIndex < 0 || newIndex >= memberStaging.length) return;
    const next = memberStaging.slice();
    [next[index], next[newIndex]] = [next[newIndex], next[index]];
    memberStaging = next;
  }

  function removeStagedMember(index) {
    memberStaging = memberStaging.filter((_, i) => i !== index);
  }

  function onMemberPicked(user) {
    if (!user || user.id == null) return;
    if (memberStaging.some((m) => m.id === user.id)) {
      memberPickerValue = null;
      return;
    }
    memberStaging = [
      ...memberStaging,
      { id: user.id, name: `${user.first_name ?? ''} ${user.last_name ?? ''}`.trim() || user.email, email: user.email },
    ];
    memberPickerValue = null;
  }

  async function save() {
    if (!formData.name?.trim()) {
      errorToast(t('teams.oncall.layerNameRequired'));
      return;
    }
    try {
      const payload = {
        name: formData.name,
        priority: Number(formData.priority) || 1,
        rotation_type: formData.rotation_type,
        rotation_interval_days: Number(formData.rotation_interval_days) || 0,
        handoff_time: formData.handoff_time,
        start_date: formData.start_date,
        end_date: formData.end_date ? formData.end_date : null,
      };

      let layerId;
      if (editingLayer) {
        await api.onCallSchedules.updateLayer(schedule.id, editingLayer.id, payload);
        layerId = editingLayer.id;
      } else {
        const result = await api.onCallSchedules.addLayer(schedule.id, payload);
        layerId = result.id;
      }

      if (layerId) {
        await api.onCallSchedules.setLayerMembers(
          schedule.id,
          layerId,
          memberStaging.map((m) => m.id)
        );
      }

      successToast(editingLayer ? t('teams.oncall.layerUpdated') : t('teams.oncall.layerCreated'));
      close();
      await onChange?.();
    } catch (err) {
      errorToast(err.message || t('teams.oncall.failedToSaveLayer'));
    }
  }

  async function deleteLayer(layer) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('teams.oncall.confirmDeleteLayer', { name: layer.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await api.onCallSchedules.deleteLayer(schedule.id, layer.id);
      successToast(t('teams.oncall.layerDeleted'));
      await onChange?.();
    } catch (err) {
      errorToast(err.message || t('teams.oncall.failedToDeleteLayer'));
    }
  }

  const sortedLayers = $derived(
    (schedule.layers || []).slice().sort((a, b) => a.priority - b.priority)
  );
</script>

<div class="space-y-3">
  <div class="flex items-center justify-between">
    <h5 class="text-sm font-medium" style="color: var(--ds-text)">
      {t('teams.oncall.layers')}
    </h5>
    {#if canEdit}
      <Button
        variant="ghost"
        size="sm"
        icon={IconPlus}
        onclick={startCreate}
        dataTestid="layer-add"
      >
        {t('teams.oncall.addLayer')}
      </Button>
    {/if}
  </div>

  {#if sortedLayers.length === 0}
    <div class="text-sm py-2" style="color: var(--ds-text-subtle)">
      {t('teams.oncall.noLayers')}
    </div>
  {:else}
    <div class="space-y-2">
      {#each sortedLayers as layer (layer.id)}
        <div
          class="rounded border p-3"
          style="border-color: var(--ds-border); background-color: var(--ds-surface);"
          data-testid="layer-row"
          data-layer-id={layer.id}
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-xs font-mono px-1.5 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                #{layer.priority}
              </span>
              <span class="text-sm font-medium" style="color: var(--ds-text)">
                {layer.name}
              </span>
              <Lozenge color="blue" text={layer.rotation_type} />
              <span class="text-xs" style="color: var(--ds-text-subtle)">
                {layer.handoff_time}
              </span>
            </div>
            {#if canEdit}
              <div class="flex gap-1">
                <Button variant="ghost" size="sm" icon={IconEdit} onclick={() => startEdit(layer)}>
                  {t('common.edit')}
                </Button>
                <Button variant="ghost" size="sm" icon={IconTrash} onclick={() => deleteLayer(layer)}>
                  {t('common.delete')}
                </Button>
              </div>
            {/if}
          </div>
          {#if layer.members && layer.members.length > 0}
            <div class="mt-2 flex flex-wrap gap-2">
              {#each layer.members.slice().sort((a, b) => a.position - b.position) as member (member.id)}
                <span class="text-xs px-2 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">
                  {member.position + 1}. {member.user_name}
                </span>
              {/each}
            </div>
          {:else}
            <div class="mt-2 text-xs" style="color: var(--ds-text-subtle)">
              {t('teams.oncall.noMembers')}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<Modal isOpen={showLayerModal} onclose={close} onSubmit={save} maxWidth="max-w-2xl">
  <ModalHeader
    title={editingLayer ? t('teams.oncall.editLayer') : t('teams.oncall.addLayer')}
    onClose={close}
  />
  <div class="px-6 py-4 max-h-[70vh] overflow-y-auto space-y-4">
    <div class="grid grid-cols-2 gap-4">
      <div>
        <label for="layer-name" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.layerName')}
        </label>
        <Input id="layer-name" bind:value={formData.name} required />
      </div>
      <div>
        <label for="layer-priority" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.priority')}
        </label>
        <Input id="layer-priority" type="number" bind:value={formData.priority} min="1" />
      </div>
      <div>
        <label for="layer-rotation-type" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.rotationType')}
        </label>
        <Select id="layer-rotation-type" bind:value={formData.rotation_type} options={rotationOptions} />
      </div>
      <div>
        <label for="layer-interval" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.intervalDays')}
        </label>
        <Input id="layer-interval" type="number" bind:value={formData.rotation_interval_days} min="0" />
      </div>
      <div>
        <label for="layer-handoff" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.handoffTime')}
        </label>
        <Input id="layer-handoff" type="time" bind:value={formData.handoff_time} />
      </div>
      <div>
        <label for="layer-start" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.startDate')}
        </label>
        <Input id="layer-start" type="date" bind:value={formData.start_date} required />
      </div>
      <div class="col-span-2">
        <label for="layer-end" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.oncall.endDateOptional')}
        </label>
        <Input id="layer-end" type="date" bind:value={formData.end_date} />
      </div>
    </div>

    <div>
      <h6 class="text-sm font-medium mb-2" style="color: var(--ds-text)">
        {t('teams.oncall.members')}
      </h6>
      <div class="max-w-md mb-3">
        <UserPicker
          bind:value={memberPickerValue}
          placeholder={t('teams.searchUser')}
          onSelect={onMemberPicked}
        />
      </div>
      {#if memberStaging.length === 0}
        <div class="text-sm" style="color: var(--ds-text-subtle)">
          {t('teams.oncall.addMembersHint')}
        </div>
      {:else}
        <div class="space-y-2">
          {#each memberStaging as m, i (m.id)}
            <div class="flex items-center justify-between p-2 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
              <div class="flex items-center gap-2">
                <span class="text-xs font-mono" style="color: var(--ds-text-subtle)">{i + 1}</span>
                <span class="text-sm" style="color: var(--ds-text)">{m.name}</span>
                <span class="text-xs" style="color: var(--ds-text-subtle)">{m.email}</span>
              </div>
              <div class="flex gap-1">
                <button
                  class="p-1 rounded"
                  style="color: var(--ds-text-subtle)"
                  onclick={() => moveMember(i, -1)}
                  disabled={i === 0}
                  data-testid={`layer-member-up-${i}`}
                  aria-label={t('teams.oncall.moveUp')}
                >
                  <IconChevronUp class="w-4 h-4" />
                </button>
                <button
                  class="p-1 rounded"
                  style="color: var(--ds-text-subtle)"
                  onclick={() => moveMember(i, 1)}
                  disabled={i === memberStaging.length - 1}
                  data-testid={`layer-member-down-${i}`}
                  aria-label={t('teams.oncall.moveDown')}
                >
                  <IconChevronDown class="w-4 h-4" />
                </button>
                <button
                  class="p-1 rounded"
                  style="color: var(--ds-text-subtle)"
                  onclick={() => removeStagedMember(i)}
                  aria-label={t('common.remove')}
                >
                  <IconX class="w-4 h-4" />
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
  <DialogFooter
    confirmLabel={editingLayer ? t('common.save') : t('teams.oncall.addLayer')}
    onCancel={close}
    onConfirm={save}
    showKeyboardHint
    confirmTestid="layer-save"
  />
</Modal>
