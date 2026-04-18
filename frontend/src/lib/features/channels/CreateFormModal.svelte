<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import IconSelector from '../../pickers/IconSelector.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';

  let {
    isOpen = $bindable(false),
    channelId = null,
    channelWorkspaceIds = [],
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
      const [allWorkspaces, allConfigSets] = await Promise.all([
        api.workspaces.getAll(),
        api.configurationSets.getAll(),
      ]);
      configSets = allConfigSets?.configuration_sets || [];
      if (channelWorkspaceIds && channelWorkspaceIds.length > 0) {
        availableWorkspaces = allWorkspaces.filter(ws => channelWorkspaceIds.includes(ws.id));
      } else {
        availableWorkspaces = allWorkspaces;
      }
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
    api.itemTypes.getAll(filters).then(types => {
      itemTypes = types;
      formData.item_type_id = null;
    }).catch(err => {
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
        <Label for="form-name" required color="default" class="mb-2">{t('forms.formName')}</Label>
        <Input
          id="form-name"
          bind:value={formData.name}
          required
          placeholder={t('forms.formNamePlaceholder')}
        />
      </div>

      <div>
        <Label for="form-description" color="default" class="mb-2">{t('forms.formDescription')}</Label>
        <Textarea
          id="form-description"
          bind:value={formData.description}
          rows={3}
          placeholder={t('forms.formDescriptionPlaceholder')}
        />
      </div>

      <div>
        <IconSelector
          bind:selectedIcon={formData.icon}
          bind:selectedColor={formData.color}
          label={t('portal.iconAndColor')}
          compact
        />
      </div>

      <div>
        <Label color="default" required class="mb-2">{t('channel.targetWorkspace')}</Label>
        <BasePicker
          bind:value={formData.workspace_id}
          items={availableWorkspaces}
          placeholder={t('channel.selectWorkspace')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
        />
      </div>

      <div>
        <Label color="default" required class="mb-2">{t('forms.createsItemType')}</Label>
        <BasePicker
          bind:value={formData.item_type_id}
          items={itemTypes}
          placeholder={t('forms.selectItemType')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
          disabled={!formData.workspace_id}
        />
        {#if !formData.workspace_id}
          <DescriptionText>
            {t('channel.selectWorkspaceFirst')}
          </DescriptionText>
        {:else}
          <DescriptionText>
            {t('portal.submissionsCreateItemType')}
          </DescriptionText>
        {/if}
      </div>
    </div>

    <!-- Footer -->
    <DialogFooter
      onCancel={handleClose}
      onConfirm={handleSubmit}
      confirmLabel={t('forms.createForm')}
      disabled={!formData.name.trim() || !formData.workspace_id || !formData.item_type_id || submitting}
      showKeyboardHint={true}
      confirmKeyboardHint={submitHint}
    />
  {/snippet}
</Modal>
