<script>
  import Building from '@lucide/svelte/icons/building';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import FileText from '@lucide/svelte/icons/file-text';
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import ListPlus from '@lucide/svelte/icons/list-plus';
  import Send from '@lucide/svelte/icons/send';
  import Settings2 from '@lucide/svelte/icons/settings-2';
  import Target from '@lucide/svelte/icons/target';
  import X from '@lucide/svelte/icons/x';
  import AlertBox from '../../../lib/components/AlertBox.svelte';
  import Button from '../../../lib/components/Button.svelte';
  import Tabs from '../../../lib/components/Tabs.svelte';
  import ChannelFormConfig from '../../../lib/features/channels/ChannelFormConfig.svelte';
  import QuickAddForm from '../../../lib/features/collections/QuickAddForm.svelte';
  import FormRenderer from '../../../lib/features/forms/FormRenderer.svelte';
  import CollectionForm from '../../../lib/forms/CollectionForm.svelte';
  import CreateFormFrame from '../../../lib/forms/CreateFormFrame.svelte';
  import MilestoneForm from '../../../lib/forms/MilestoneForm.svelte';
  import WorkItemForm from '../../../lib/forms/WorkItemForm.svelte';
  import WorkspaceForm from '../../../lib/forms/WorkspaceForm.svelte';
  import StaticViewBackground from '../../../lib/layout/StaticViewBackground.svelte';
  import ViewHeader from '../../../lib/layout/ViewHeader.svelte';
  import ChipPicker from '../../../lib/pickers/ChipPicker.svelte';
  import { getDisplayString, getShortcut } from '../../../lib/utils/keyboardShortcuts.js';

  const formPatterns = [
    {
      id: 'creation',
      label: 'Creation modal',
      icon: FileText,
      testid: 'design-system-form-pattern-creation',
    },
    {
      id: 'request',
      label: 'Public request',
      icon: Send,
      testid: 'design-system-form-pattern-request',
    },
    {
      id: 'configuration',
      label: 'Configuration',
      icon: Settings2,
      testid: 'design-system-form-pattern-configuration',
    },
    {
      id: 'quick-add',
      label: 'Quick add',
      icon: ListPlus,
      testid: 'design-system-form-pattern-quick-add',
    },
  ];

  const exampleTypes = [
    { value: 'work-item', label: 'Work item', icon: FileText },
    { value: 'milestone', label: 'Milestone', icon: Target },
    { value: 'workspace', label: 'Workspace', icon: Building },
    { value: 'collection', label: 'Collection', icon: FolderOpen },
  ];
  const typeIcons = Object.fromEntries(exampleTypes.map((type) => [type.value, type.icon]));

  const workspace = {
    id: 1,
    key: 'WIND',
    name: 'Windshift',
    description: 'Product planning and delivery',
    color: 'var(--ds-interactive)',
    icon: 'Package',
  };
  const itemTypes = [
    {
      id: 1,
      name: 'Task',
      icon: 'CheckSquare',
      color: 'var(--ds-interactive)',
      hierarchy_level: 1,
      sort_order: 1,
    },
    {
      id: 2,
      name: 'Story',
      icon: 'BookOpen',
      color: '#7c3aed',
      hierarchy_level: 1,
      sort_order: 2,
    },
  ];
  const priorities = [
    { id: 1, name: 'High', icon: 'ArrowUp', color: '#dc2626', sort_order: 1 },
    { id: 2, name: 'Medium', icon: 'Minus', color: '#d97706', sort_order: 2 },
    { id: 3, name: 'Low', icon: 'ArrowDown', color: '#6b7280', sort_order: 3 },
  ];
  const collectionCategories = [
    { id: 1, name: 'Delivery' },
    { id: 2, name: 'Planning' },
    { id: 3, name: 'Operations' },
  ];
  const submitShortcut = getShortcut('modal', 'submit');

  const initialWorkItemData = {
    name: 'Prepare the release readiness review',
    description: 'Confirm ownership, rollout checks, and communication before the release window.',
    due_date: '2026-08-14',
    start_date: '',
    end_date: '',
    workspace_id: workspace.id,
    priority_id: null,
    milestone_ids: [],
    assignee_id: null,
    iteration_id: null,
    project_id: null,
    label_names: [],
    story_points: '',
    estimate: '',
    item_type_id: 1,
  };
  const initialWorkspaceData = {
    name: 'Product operations',
    key: 'PO',
    description: 'Plan launches, prioritize customer feedback, and keep delivery work visible.',
  };
  const initialMilestoneData = {
    name: 'Autumn release',
    description: 'Coordinate final product, documentation, and operational readiness.',
    target_date: '2026-09-30',
    status: 'planning',
  };
  const initialCollectionData = {
    name: 'Launch readiness',
    description: 'A focused view of the work required before launch.',
    workspace_id: workspace.id,
  };

  const publicRequestDetail = {
    form_id: 101,
    fields: [
      {
        field_type: 'default',
        field_identifier: 'title',
        display_name: 'What do you need help with?',
        description: 'A concise summary helps us route your request.',
        is_required: true,
        step_number: 1,
      },
      {
        field_type: 'default',
        field_identifier: 'description',
        display_name: 'Request details',
        is_required: true,
        step_number: 1,
      },
      {
        field_type: 'virtual',
        field_identifier: 'urgency',
        display_name: 'Urgency',
        virtual_field_type: 'select',
        virtual_field_options: JSON.stringify([
          { value: 'normal', label: 'Normal — within a few days' },
          { value: 'high', label: 'High — blocking current work' },
          { value: 'critical', label: 'Critical — service unavailable' },
        ]),
        is_required: true,
        step_number: 2,
      },
      {
        field_type: 'virtual',
        field_identifier: 'impact',
        display_name: 'Who is affected?',
        description: 'Include teams, customers, or business processes.',
        virtual_field_type: 'textarea',
        is_required: false,
        step_number: 2,
      },
      {
        field_type: 'virtual',
        field_identifier: 'follow_up',
        display_name: 'I am available for follow-up questions',
        virtual_field_type: 'checkbox',
        is_required: false,
        step_number: 2,
      },
    ],
    custom_field_definitions: [],
  };
  const publicRequestValues = {
    title: 'Access to the customer research repository',
    description: 'Our product team needs read access before the next discovery round.',
    custom_fields: {
      urgency: 'normal',
      impact: 'Four product managers and two designers are affected.',
      follow_up: true,
    },
  };

  const requestedPattern = new URLSearchParams(window.location.search).get('pattern');
  let activePattern = $state(
    formPatterns.some((pattern) => pattern.id === requestedPattern) ? requestedPattern : 'creation',
  );
  let selectedType = $state('work-item');
  let showFrame = $state(true);
  let submitted = $state(false);
  let workItemData = $state({ ...initialWorkItemData });
  let workspaceData = $state({ ...initialWorkspaceData });
  let milestoneData = $state({ ...initialMilestoneData });
  let collectionData = $state({ ...initialCollectionData });
  let collectionCategoryId = $state(1);
  let workspaceFormRef = $state(null);
  let milestoneFormRef = $state(null);
  let collectionFormRef = $state(null);
  let quickAddSubmitted = $state(false);
  let quickAddState = $state({
    title: 'Add release notes to the launch checklist',
    workspaceId: workspace.id,
    itemTypeId: itemTypes[0].id,
    availableTypes: itemTypes,
    error: '',
  });
  let channelFormData = $state({
    slug: 'product-feedback',
    workspace_ids: [workspace.id],
    enabled: true,
    theme: 'light',
    brand_color: '#2874bb',
    logo_url: '',
    success_message: 'Thanks — your feedback has been added to our product queue.',
    redirect_url: '',
  });

  const workItemStore = {
    get formData() {
      return workItemData;
    },
    set formData(value) {
      workItemData = value;
    },
    selectedWorkspace: workspace,
    availableItemTypes: itemTypes,
    currentConfigSet: { priorities_detailed: priorities },
    users: [],
    milestones: [],
    iterations: [],
    timeProjects: [],
    screenSystemFields: ['priority', 'due_date'],
    screenFields: [
      { field_type: 'system', field_identifier: 'priority', is_required: false },
      { field_type: 'system', field_identifier: 'due_date', is_required: false },
    ],
    customFieldValues: {},
    customFieldsLoaded: false,
    validationErrors: [],
    selectedLabels: [],
    templateOptions: [
      { id: 1, name: 'Release task', description_body: 'Document the release readiness checks.' },
    ],
    mandatoryTemplate: null,
    selectedTemplateId: null,
    templateApplyNonce: 0,
    configSetLoadedForWorkspace: workspace.id,
    screenFieldsLoadedForKey: `${workspace.id}-1`,
    storedWorkspaceId: null,
    lastPersistedItemTypeId: 1,
    parentItem: null,
    nonRequiredCustomFields: [],
    nonRequiredFullSystemFields: [],
    requiredCustomFields: [],
    requiredSystemFields: [],
    get selectedItemType() {
      return itemTypes.find((type) => type.id === workItemData.item_type_id) || null;
    },
    get configSetPriorities() {
      return priorities;
    },
    get templateLocked() {
      return false;
    },
    get selectedAssignee() {
      return null;
    },
    get selectedMilestones() {
      return [];
    },
    isFieldConfigured(identifier) {
      return this.screenSystemFields.includes(identifier);
    },
    isFieldRequired() {
      return false;
    },
    setItemType(itemTypeId) {
      workItemData.item_type_id = itemTypeId;
      this.lastPersistedItemTypeId = itemTypeId;
    },
    setWorkspace(selectedWorkspace) {
      this.selectedWorkspace = selectedWorkspace;
      workItemData.workspace_id = selectedWorkspace?.id ?? null;
    },
    applyTemplate(templateId) {
      const template = this.templateOptions.find((option) => option.id === templateId);
      if (!template) return;
      this.selectedTemplateId = templateId;
      workItemData.description = template.description_body;
      this.templateApplyNonce += 1;
    },
    applyStoredWorkspace() {},
    applyStoredItemType() {},
    applyConfigSetDefault() {},
    loadWorkspaceDetails() {},
    loadConfigSetForWorkspace() {},
    loadScreenFieldsForItemType() {},
    addPendingDescriptionImage() {},
  };

  let selectedExample = $derived(
    exampleTypes.find((type) => type.value === selectedType) || exampleTypes[0],
  );
  let isCurrentValid = $derived.by(() => {
    if (selectedType === 'work-item') {
      return Boolean(workItemData.name.trim() && workItemData.workspace_id && workItemData.item_type_id);
    }
    if (selectedType === 'workspace') {
      return Boolean(workspaceData.name.trim() && workspaceData.key.trim());
    }
    if (selectedType === 'milestone') {
      return Boolean(milestoneData.name.trim() && milestoneData.target_date);
    }
    return Boolean(collectionData.name.trim());
  });

  function selectExample(type) {
    selectedType = type.value;
    submitted = false;
  }

  function selectPattern({ tab }) {
    const url = new URL(window.location.href);
    if (tab === 'creation') {
      url.searchParams.delete('pattern');
    } else {
      url.searchParams.set('pattern', tab);
    }
    window.history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`);
  }

  function handleSubmit() {
    if (!isCurrentValid) return;
    if (selectedType === 'workspace' && !workspaceFormRef?.validate()) return;
    if (selectedType === 'milestone' && !milestoneFormRef?.validate()) return;
    if (selectedType === 'collection' && !collectionFormRef?.validate()) return;
    submitted = true;
  }

  function closeExample() {
    showFrame = false;
    submitted = false;
  }

  function updateQuickAddField(_parentId, field, value) {
    quickAddState[field] = value;
    quickAddSubmitted = false;
  }

  function submitQuickAdd() {
    quickAddSubmitted = true;
  }
</script>

<StaticViewBackground contentClass="p-6" testid="design-system-form-example">
  <div class="mx-auto max-w-4xl">
    <div class="mb-8">
      <ViewHeader
        workspaceName="Windshift"
        collection="Composition examples"
        viewName="Form patterns"
      />
    </div>

    <div class="mb-5 max-w-2xl">
      <p class="text-sm leading-6" style="color: var(--ds-text-subtle);">
        Compare the production patterns used for creation, public requests, configuration, and quick entry. Every example uses the application component with local preview data.
      </p>
    </div>

    <Tabs tabs={formPatterns} bind:activeTab={activePattern} onTabChange={selectPattern}>
      {#if activePattern === 'creation'}
        <div class="mb-5 max-w-2xl">
          <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Creation modal</h2>
          <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
            The compact creation pattern used for work items, milestones, workspaces, and collections.
          </p>
        </div>

        {#if submitted}
          <AlertBox
            variant="success"
            class="mb-4 max-w-lg"
            message="The {selectedExample.label.toLowerCase()} example is valid. Nothing was saved."
          />
        {/if}

        {#if showFrame}
          <div class="flex justify-center lg:justify-start">
            <CreateFormFrame class="mx-0" dataTestid="design-system-create-form">
              {#snippet header()}
              <ChipPicker
                value={selectedType}
                items={exampleTypes}
                getValue={(type) => type.value}
                getLabel={(type) => type.label}
                icon={typeIcons[selectedType]}
                placeholder="Entity type"
                testId="design-system-form-type"
                onSelect={selectExample}
              >
                {#snippet itemSnippet({ item })}
                  <item.icon size={16} style="color: var(--ds-text-subtle);" />
                  <span class="font-medium">{item.label}</span>
                {/snippet}
              </ChipPicker>
              <ChevronRight size={14} style="color: var(--ds-text-subtle);" />

              {#if selectedType === 'work-item'}
                <ChipPicker
                  value={workspace.id}
                  items={[workspace]}
                  getValue={(item) => item.id}
                  getLabel={(item) => item.key || item.name}
                  icon={Building}
                  placeholder="Workspace"
                  searchable={true}
                  searchFields={['name', 'key']}
                  testId="design-system-form-workspace"
                >
                  {#snippet itemSnippet({ item })}
                    <div
                      class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded"
                      style="background-color: {item.color};"
                    >
                      <Building size={10} style="color: #fff;" />
                    </div>
                    <span class="truncate font-medium">{item.name}</span>
                    <span
                      class="flex-shrink-0 rounded px-1.5 py-0.5 text-xs"
                      style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                    >
                      {item.key}
                    </span>
                  {/snippet}
                </ChipPicker>
                <ChevronRight size={14} style="color: var(--ds-text-subtle);" />
              {/if}

              <span class="font-medium" style="color: var(--ds-text);">
                New {selectedExample.label}
              </span>
              <button
                onclick={closeExample}
                class="ml-auto rounded p-1.5 transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
                style="color: var(--ds-text-subtle);"
                aria-label="Close example"
              >
                <X size={16} />
              </button>
              {/snippet}

              {#snippet body()}
                {#if selectedType === 'work-item'}
                  <WorkItemForm formStore={workItemStore} />
                {:else if selectedType === 'milestone'}
                  <MilestoneForm bind:this={milestoneFormRef} bind:formData={milestoneData} />
                {:else if selectedType === 'workspace'}
                  <WorkspaceForm bind:this={workspaceFormRef} bind:formData={workspaceData} />
                {:else}
                  <CollectionForm
                    bind:this={collectionFormRef}
                    bind:formData={collectionData}
                    bind:categoryId={collectionCategoryId}
                    categories={collectionCategories}
                  />
                {/if}
              {/snippet}

              {#snippet footer()}
                <!-- shortcut-guard-exempt: static design-system composition demo does not register application shortcuts. -->
                <Button
                  id="design-system-form-submit"
                  dataTestid="design-system-form-submit"
                  variant="primary"
                  size="medium"
                  keyboardHint={getDisplayString(submitShortcut)}
                  disabled={!isCurrentValid}
                  onclick={handleSubmit}
                >
                  Create {selectedExample.label}
                </Button>
              {/snippet}
            </CreateFormFrame>
          </div>
        {:else}
          <Button variant="primary" onclick={() => (showFrame = true)}>Open form example</Button>
        {/if}
      {:else if activePattern === 'request'}
        <div class="max-w-2xl">
          <div class="mb-5">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Public request form</h2>
            <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
              The customer-facing, multi-step pattern rendered by public form channels.
            </p>
          </div>

          <div
            class="rounded-xl border p-6 shadow-sm"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          >
            <div class="mb-6">
              <h3 class="text-xl font-bold" style="color: var(--ds-text);">Product access request</h3>
              <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
                Tell the product operations team what you need and who is affected.
              </p>
            </div>

            <FormRenderer
              formSlug="design-system-preview"
              formId={publicRequestDetail.form_id}
              formConfig={{ submit_button_text: 'Send request' }}
              initialDetail={publicRequestDetail}
              initialValues={publicRequestValues}
              submitForm={async () => ({ success_message: 'Request preview submitted.' })}
            />
          </div>
        </div>
      {:else if activePattern === 'configuration'}
        <div class="max-w-2xl">
          <div class="mb-5">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Configuration form</h2>
            <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
              The full settings pattern used to configure and publish a form channel.
            </p>
          </div>

          <div
            class="rounded-lg border p-6"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          >
            <div>
              <h3 class="text-base font-semibold" style="color: var(--ds-text);">Product feedback channel</h3>
              <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
                Hosted form appearance, destination, confirmation, and integration settings.
              </p>
            </div>
            <ChannelFormConfig
              bind:formData={channelFormData}
              workspaces={[workspace]}
            />
          </div>
        </div>
      {:else}
        <div class="max-w-2xl">
          <div class="mb-5">
            <h2 class="text-lg font-semibold" style="color: var(--ds-text);">Inline quick add</h2>
            <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
              The lightweight entry pattern used directly inside collection boards.
            </p>
          </div>

          {#if quickAddSubmitted}
            <AlertBox
              variant="success"
              class="mb-4 max-w-lg"
              message="The quick-add example is valid. Nothing was saved."
            />
          {/if}

          <div
            class="rounded-lg p-6"
            style="background-color: var(--ds-background-neutral);"
            data-testid="design-system-quick-add"
          >
            <div class="max-w-lg">
              <QuickAddForm
                parentId="design-system"
                formState={quickAddState}
                workspaces={[workspace]}
                cardBgStyle="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
                onUpdateField={updateQuickAddField}
                onCreate={submitQuickAdd}
                onCancel={() => (quickAddState.title = '')}
              />
            </div>
          </div>
        </div>
      {/if}
    </Tabs>
  </div>
</StaticViewBackground>
