<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { workspacesStore } from '../../stores/workspaces.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Label from '../../components/Label.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
	import TextField from '../../components/TextField.svelte';

  let {
    isOpen = $bindable(false),
    channelId = null,
    onPrepare = async () => {},
    onCreated = () => {},
    onClose = () => {},
  } = $props();

  let submitting = $state(false);
  let itemTypes = $state([]);
  let availableWorkspaces = $state([]);
  let configSets = [];

  let formData = $state({
    name: '',
    description: '',
    icon: 'FileText',
    color: '#6b7280',
    item_type_id: null,
    workspace_id: null,
  });

  onMount(async () => {
    try {
      const [allConfigSets] = await Promise.all([
        api.configurationSets.getAll(),
      ]);
      const allWorkspaces = await workspacesStore.load();
      configSets = allConfigSets?.configuration_sets || [];
      availableWorkspaces = allWorkspaces || [];
    } catch (err) {
      console.error('Failed to load workspaces:', err);
    }
  });

  $effect(() => {
    if (isOpen) {
      formData = {
        name: '',
        description: '',
        icon: 'FileText',
        color: '#6b7280',
        item_type_id: null,
        workspace_id: null,
      };
      itemTypes = [];
    }
  });

  $effect(() => {
    const wsId = formData.workspace_id;
    if (!wsId) {
      itemTypes = [];
      return;
    }
    const ws = availableWorkspaces.find(w => w.id === wsId);
    let configSetId = ws?.configuration_set_id;
    if (!configSetId) {
      const defaultCs = configSets.find(cs => cs.is_default);
      if (defaultCs) configSetId = defaultCs.id;
    }
    const filters = configSetId ? { configuration_set_id: configSetId } : {};
    const requestedWsId = wsId;
    api.itemTypes.getAll(filters).then(types => {
      // Out-of-order responses: if workspace_id changed while the request was
      // in flight, drop this result so a slower earlier fetch can't clobber a
      // faster later one.
      if (formData.workspace_id !== requestedWsId) return;
      itemTypes = types;
      formData.item_type_id = null;
    }).catch(err => {
      if (formData.workspace_id !== requestedWsId) return;
      console.error('Failed to load item types:', err);
      itemTypes = [];
    });
  });

  async function handleSubmit() {
    if (!formData.name.trim()) {
      errorToast(t('forms.formNameRequired', 'Name is required'));
      return;
    }
    if (!formData.item_type_id) {
      errorToast(t('forms.selectItemType', 'Please select an item type'));
      return;
    }

    try {
      submitting = true;
      await onPrepare(formData.workspace_id);
      await api.requestTypes.create(channelId, {
        name: formData.name.trim(),
        description: formData.description.trim(),
        icon: formData.icon,
        color: formData.color,
        item_type_id: formData.item_type_id,
        workspace_id: formData.workspace_id || null,
        is_active: true,
      });
      handleClose();
      onCreated();
    } catch (err) {
      console.error('Failed to create form:', err);
      errorToast(err.message || t('common.error'));
    } finally {
      submitting = false;
    }
  }

  function handleClose() {
    isOpen = false;
    onClose();
  }
</script>

<Modal
  bind:isOpen
  onclose={handleClose}
  onSubmit={handleSubmit}
  submitDisabled={!formData.name.trim() || !formData.workspace_id || !formData.item_type_id}
  maxWidth="max-w-lg"
  autoFocus={true}
>
  {#snippet children(submitHint)}
    <!-- Header -->
    <ModalHeader title={t('forms.createForm')} showCloseButton={false} />

    <!-- Content -->
    <div class="p-6 space-y-4">
      <div>
        <TextField
          label={t('forms.formName')}
          id="form-name"
          required
          labelColor="default"
          placeholder={t('forms.formNamePlaceholder')}
          bind:value={formData.name}
        />
        <DescriptionText>
          The public-facing name people use to choose this form.
        </DescriptionText>
      </div>

      <div>
        <Label color="default" required class="mb-2">{t('channel.targetWorkspace')}</Label>
        <BasePicker
          id="form-workspace"
          bind:value={formData.workspace_id}
          items={availableWorkspaces}
          placeholder={t('channel.selectWorkspace')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
          optionTestid={(option) => `form-workspace-${option.value}`}
        />
        <DescriptionText>
          Every response to this form creates a work item in this workspace.
        </DescriptionText>
      </div>

      <div>
        <Label color="default" required class="mb-2">{t('forms.createsItemType')}</Label>
        <BasePicker
          id="form-item-type"
          bind:value={formData.item_type_id}
          items={itemTypes}
          placeholder={t('forms.selectItemType')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
          optionTestid={(option) => `form-item-type-${option.value}`}
          disabled={!formData.workspace_id}
        />
        {#if !formData.workspace_id}
          <DescriptionText>
            {t('channel.selectWorkspaceFirst')}
          </DescriptionText>
        {:else}
          <DescriptionText>
            This determines the workflow, fields, and item type created by each response.
          </DescriptionText>
        {/if}
      </div>
    </div>

    <!-- Footer -->
    <DialogFooter
      onCancel={handleClose}
      onConfirm={handleSubmit}
      confirmLabel={t('forms.createForm')}
      confirmTestid="form-create-confirm"
      disabled={!formData.name.trim() || !formData.workspace_id || !formData.item_type_id || submitting}
      showKeyboardHint={true}
      confirmKeyboardHint={submitHint}
    />
  {/snippet}
</Modal>
