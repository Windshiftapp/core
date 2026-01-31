<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import Select from '../../components/Select.svelte';
  import { Plus, Package, Edit, Trash2, Settings, FolderTree, Users, User, ChevronRight, ChevronDown, Folder, FolderOpen, MoreHorizontal, X } from 'lucide-svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import FieldLayoutEditor from '../../editors/FieldLayoutEditor.svelte';
  import ColorPicker from '../../editors/ColorPicker.svelte';
  import Label from '../../components/Label.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  // State for asset sets
  let assetSets = $state([]);
  let selectedSetId = $state(null);
  let selectedSet = $derived(assetSets.find(s => s.id === selectedSetId));

  // State for tabs
  let activeTab = $state('types'); // 'types', 'categories', 'permissions'

  // Asset Types state
  let assetTypes = $state([]);
  let showTypeForm = $state(false);
  let editingType = $state(null);
  let typeFormData = $state({ name: '', description: '', icon: 'package', color: '#6b7280', is_active: true });

  // Asset Categories state
  let assetCategories = $state([]);
  let showCategoryForm = $state(false);
  let editingCategory = $state(null);
  let categoryFormData = $state({ name: '', description: '', parent_id: null });
  let expandedCategories = $state(new Set());


  // Set roles state
  let roleAssignments = $state({ user_roles: [], group_roles: [], everyone_role: null });
  let assetRoles = $state([]);
  let availableGroups = $state([]);
  let showRoleForm = $state(false);
  let roleFormData = $state({ type: 'user', user_id: null, group_id: null, role_id: null });
  let everyoneRoleId = $state(null);
  let availableUsers = $state([]);

  // Set form state
  let showSetForm = $state(false);
  let editingSet = $state(null);
  let setFormData = $state({ name: '', description: '', is_default: false });

  // Field assignment state
  let showFieldsModal = $state(false);
  let editingTypeForFields = $state(null);
  let availableFields = $state([]);
  let typeFields = $state([]);  // Full field objects with display_order, is_required

  onMount(async () => {
    await Promise.all([
      loadAssetSets(),
      loadUsers(),
      loadAssetRoles(),
      loadGroups(),
    ]);
  });

  async function loadAssetSets() {
    try {
      const sets = await api.assetSets.getAll();
      assetSets = sets || [];
      if (assetSets.length > 0 && !selectedSetId) {
        const defaultSet = assetSets.find(s => s.is_default) || assetSets[0];
        selectedSetId = defaultSet.id;
      }
    } catch (error) {
      console.error('Failed to load asset sets:', error);
    }
  }

  async function loadUsers() {
    try {
      const users = await api.getUsers();
      availableUsers = users || [];
    } catch (error) {
      console.error('Failed to load users:', error);
    }
  }

  // Load data when set changes
  $effect(() => {
    if (selectedSetId) {
      loadAssetTypes();
      loadAssetCategories();
      loadSetRoles();
    }
  });

  // Asset Set functions
  function showAddSetForm() {
    showSetForm = true;
    editingSet = null;
    setFormData = { name: '', description: '', is_default: false };
  }

  function showEditSetForm(set) {
    showSetForm = true;
    editingSet = set;
    setFormData = { name: set.name, description: set.description || '', is_default: set.is_default };
  }

  async function handleSetSubmit() {
    try {
      if (editingSet) {
        await api.assetSets.update(editingSet.id, setFormData);
      } else {
        await api.assetSets.create(setFormData);
      }
      await loadAssetSets();
      showSetForm = false;
    } catch (error) {
      console.error('Failed to save asset set:', error);
      alert(t('dialogs.alerts.failedToSave', { error: error.message }));
    }
  }

  async function deleteSet(id) {
    if (confirm(t('dialogs.confirmations.deleteAssetSet'))) {
      try {
        await api.assetSets.delete(id);
        if (selectedSetId === id) {
          selectedSetId = null;
        }
        await loadAssetSets();
      } catch (error) {
        console.error('Failed to delete asset set:', error);
        alert(t('dialogs.alerts.failedToDelete', { error: error.message }));
      }
    }
  }

  // Asset Type functions
  async function loadAssetTypes() {
    if (!selectedSetId) return;
    try {
      const types = await api.assetTypes.getAll(selectedSetId);
      assetTypes = types || [];
    } catch (error) {
      console.error('Failed to load asset types:', error);
    }
  }

  function showAddTypeForm() {
    showTypeForm = true;
    editingType = null;
    typeFormData = { name: '', description: '', icon: 'package', color: '#6b7280', is_active: true };
  }

  function showEditTypeForm(type) {
    showTypeForm = true;
    editingType = type;
    typeFormData = {
      name: type.name,
      description: type.description || '',
      icon: type.icon || 'package',
      color: type.color || '#6b7280',
      is_active: type.is_active
    };
  }

  async function handleTypeSubmit() {
    try {
      if (editingType) {
        await api.assetTypes.update(editingType.id, typeFormData);
      } else {
        await api.assetTypes.create(selectedSetId, typeFormData);
      }
      await loadAssetTypes();
      showTypeForm = false;
    } catch (error) {
      console.error('Failed to save asset type:', error);
      alert(t('dialogs.alerts.failedToSave', { error: error.message }));
    }
  }

  async function deleteType(id) {
    if (confirm(t('dialogs.confirmations.deleteAssetType'))) {
      try {
        await api.assetTypes.delete(id);
        await loadAssetTypes();
      } catch (error) {
        console.error('Failed to delete asset type:', error);
        alert(t('dialogs.alerts.failedToDelete', { error: error.message }));
      }
    }
  }

  // Field assignment functions
  async function showFieldsForm(type) {
    editingTypeForFields = type;

    try {
      // Load all available custom fields (excluding system default fields)
      // Note: Work item system fields (Status, Priority, etc.) don't apply to assets
      const fields = await api.customFields.getAll();
      const customFields = (fields || [])
        .filter(f => !f.system_default)
        .map(f => ({
          identifier: f.id.toString(),
          id: f.id,
          name: f.name,
          type: 'custom',
          fieldType: f.field_type,
          description: f.description,
          category: 'Custom Fields'
        }));

      availableFields = customFields;

      // Load currently assigned fields for this type
      const assignedFields = await api.assetTypes.getFields(type.id);
      typeFields = (assignedFields || []).map((f, index) => ({
        field_identifier: f.custom_field_id.toString(),
        field_type: 'custom',
        field_name: f.field_name,
        display_order: f.display_order ?? index,
        is_required: f.is_required ?? false
      }));

      // Ensure Title field is always present (first, protected)
      if (!typeFields.some(f => f.field_identifier === 'title')) {
        typeFields = [
          { field_identifier: 'title', field_type: 'system', field_name: 'Title', display_order: 0, is_required: true },
          ...typeFields.map(f => ({ ...f, display_order: f.display_order + 1 }))
        ];
      }

      // Ensure Description field is present (after title)
      if (!typeFields.some(f => f.field_identifier === 'description')) {
        const titleIndex = typeFields.findIndex(f => f.field_identifier === 'title');
        const insertIndex = titleIndex >= 0 ? titleIndex + 1 : 0;
        typeFields = [
          ...typeFields.slice(0, insertIndex),
          { field_identifier: 'description', field_type: 'system', field_name: 'Description', display_order: insertIndex, is_required: false },
          ...typeFields.slice(insertIndex).map(f => ({ ...f, display_order: f.display_order + 1 }))
        ];
      }

      showFieldsModal = true;
    } catch (error) {
      console.error('Failed to load fields:', error);
      alert(t('dialogs.alerts.failedToLoadFields', { error: error.message }));
    }
  }

  async function handleFieldsSubmit() {
    try {
      // Transform to API format with ordering and required flags
      // Only save custom fields - system fields are implicit
      const fieldsData = {
        fields: typeFields
          .filter(f => f.field_type === 'custom')
          .map((f, index) => ({
            custom_field_id: parseInt(f.field_identifier),
            is_required: f.is_required ?? false,
            display_order: index
          }))
      };
      await api.assetTypes.updateFields(editingTypeForFields.id, fieldsData);
      showFieldsModal = false;
      editingTypeForFields = null;
      typeFields = [];
    } catch (error) {
      console.error('Failed to save field assignments:', error);
      alert(t('dialogs.alerts.failedToSaveFields', { error: error.message }));
    }
  }

  function handleFieldsCancel() {
    showFieldsModal = false;
    editingTypeForFields = null;
    typeFields = [];
  }

  // Asset Category functions
  async function loadAssetCategories() {
    if (!selectedSetId) return;
    try {
      const categories = await api.assetCategories.getAll(selectedSetId, true);
      assetCategories = categories || [];
    } catch (error) {
      console.error('Failed to load asset categories:', error);
    }
  }

  function showAddCategoryForm(parentId = null) {
    showCategoryForm = true;
    editingCategory = null;
    categoryFormData = { name: '', description: '', parent_id: parentId };
  }

  function showEditCategoryForm(category) {
    showCategoryForm = true;
    editingCategory = category;
    categoryFormData = {
      name: category.name,
      description: category.description || '',
      parent_id: category.parent_id
    };
  }

  async function handleCategorySubmit() {
    try {
      if (editingCategory) {
        await api.assetCategories.update(editingCategory.id, categoryFormData);
      } else {
        await api.assetCategories.create(selectedSetId, categoryFormData);
      }
      await loadAssetCategories();
      showCategoryForm = false;
    } catch (error) {
      console.error('Failed to save category:', error);
      alert(t('dialogs.alerts.failedToSave', { error: error.message }));
    }
  }

  async function deleteCategory(id) {
    if (confirm(t('dialogs.confirmations.deleteCategory'))) {
      try {
        await api.assetCategories.delete(id);
        await loadAssetCategories();
      } catch (error) {
        console.error('Failed to delete category:', error);
        alert(t('dialogs.alerts.failedToDelete', { error: error.message }));
      }
    }
  }

  function toggleCategory(categoryId) {
    const newExpanded = new Set(expandedCategories);
    if (newExpanded.has(categoryId)) {
      newExpanded.delete(categoryId);
    } else {
      newExpanded.add(categoryId);
    }
    expandedCategories = newExpanded;
  }

  // Role functions
  async function loadAssetRoles() {
    try {
      const roles = await api.assetRoles.getAll();
      assetRoles = roles || [];
    } catch (error) {
      console.error('Failed to load asset roles:', error);
    }
  }

  async function loadGroups() {
    try {
      const groups = await api.groups.getAll();
      availableGroups = groups || [];
    } catch (error) {
      console.error('Failed to load groups:', error);
    }
  }

  async function loadSetRoles() {
    if (!selectedSetId) return;
    try {
      const roles = await api.assetSets.getRoles(selectedSetId);
      roleAssignments = roles || { user_roles: [], group_roles: [], everyone_role: null };
      everyoneRoleId = roles?.everyone_role?.role_id || null;
    } catch (error) {
      console.error('Failed to load role assignments:', error);
    }
  }

  function showAddRoleForm() {
    showRoleForm = true;
    roleFormData = { type: 'user', user_id: null, group_id: null, role_id: assetRoles[0]?.id || null };
  }

  async function handleRoleSubmit() {
    try {
      const data = {
        role_id: roleFormData.role_id,
      };
      if (roleFormData.type === 'user') {
        data.user_id = roleFormData.user_id;
      } else {
        data.group_id = roleFormData.group_id;
      }
      await api.assetSets.assignRole(selectedSetId, data);
      await loadSetRoles();
      showRoleForm = false;
    } catch (error) {
      console.error('Failed to assign role:', error);
      alert(t('dialogs.alerts.failedToAssignRole', { error: error.message }));
    }
  }

  async function revokeRole(assignmentId, type) {
    if (confirm(t('dialogs.confirmations.revokeRole'))) {
      try {
        await api.assetSets.revokeRole(selectedSetId, assignmentId, type);
        await loadSetRoles();
      } catch (error) {
        console.error('Failed to revoke role:', error);
        alert(t('dialogs.alerts.failedToRevokeRole', { error: error.message }));
      }
    }
  }

  async function handleEveryoneRoleChange() {
    try {
      await api.assetSets.setEveryoneRole(selectedSetId, {
        role_id: everyoneRoleId || null
      });
      await loadSetRoles();
    } catch (error) {
      console.error('Failed to update everyone role:', error);
      alert(t('dialogs.alerts.failedToUpdateRole', { error: error.message }));
    }
  }

  // Combined role assignments for table display
  let allRoleAssignments = $derived(() => {
    const users = (roleAssignments.user_roles || []).map(r => ({
      ...r,
      type: 'user',
      assignee_name: r.user_name || r.user_email || 'Unknown User'
    }));
    const groups = (roleAssignments.group_roles || []).map(r => ({
      ...r,
      type: 'group',
      assignee_name: r.group_name || 'Unknown Group'
    }));
    return [...users, ...groups];
  });

  // Helper to flatten categories for select
  function flattenCategories(categories, level = 0) {
    let result = [];
    for (const cat of categories) {
      result.push({ ...cat, level });
      if (cat.children?.length > 0) {
        result = result.concat(flattenCategories(cat.children, level + 1));
      }
    }
    return result;
  }

  const flatCategories = $derived(flattenCategories(assetCategories));

  // DataTable columns
  const typeColumns = [
    { key: 'name', label: 'Name', slot: 'name' },
    { key: 'color', label: 'Color', slot: 'color' },
    { key: 'asset_count', label: 'Assets' },
    { key: 'is_active', label: 'Status', slot: 'status' },
    { key: 'actions', label: '', slot: 'actions', width: '100px' }
  ];

  const roleColumns = [
    { key: 'assignee_name', label: 'Assignee', slot: 'assignee' },
    { key: 'role_name', label: 'Role', slot: 'role' },
    { key: 'actions', label: '', slot: 'actions', width: '80px' }
  ];
</script>

<div class="p-6">
  <PageHeader title={t('assets.title')} icon={Package} subtitle={t('assets.subtitle')}>
    {#snippet actions()}
      <div class="flex items-center gap-2">
        <Select
          bind:value={selectedSetId}
          class="w-48"
        >
          <option value={null}>{t('assets.selectAssetSet')}</option>
          {#each assetSets as set}
            <option value={set.id}>{set.name}{set.is_default ? ` (${t('assets.default')})` : ''}</option>
          {/each}
        </Select>
        <Button variant="outline" size="sm" onclick={showAddSetForm} class="whitespace-nowrap">
          <Plus class="w-4 h-4 mr-1" />
          {t('assets.newSet')}
        </Button>
      </div>
    {/snippet}
  </PageHeader>

  {#snippet createSetButton()}
    <Button onclick={showAddSetForm}>
      <Plus class="w-4 h-4 mr-1" />
      {t('assets.createAssetSet')}
    </Button>
  {/snippet}

  <div class="rounded-xl border shadow-sm p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  {#if assetSets.length === 0}
    <EmptyState
      icon={Package}
      title={t('assets.noAssetSets')}
      description={t('assets.noAssetSetsDesc')}
      action={createSetButton}
    />
  {:else if !selectedSetId}
    <EmptyState
      icon={Package}
      title={t('assets.selectAnAssetSet')}
      description={t('assets.selectAnAssetSetDesc')}
    />
  {:else}
    <!-- Set info header -->
    <div class="mb-6 p-4 rounded-lg flex justify-between items-center" style="background: var(--ds-surface-raised);">
      <div>
        <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{selectedSet?.name}</h2>
        {#if selectedSet?.description}
          <p class="text-sm" style="color: var(--ds-text-subtle);">{selectedSet.description}</p>
        {/if}
      </div>
      <DropdownMenu
        triggerIcon={MoreHorizontal}
        iconOnly={true}
        showChevron={false}
        triggerClass="p-2 rounded hover-bg"
        items={[
          { id: 'edit', title: t('assets.editSet'), icon: Edit, onClick: () => showEditSetForm(selectedSet) },
          { id: 'delete', title: t('assets.deleteSet'), icon: Trash2, color: 'var(--ds-text-danger)', onClick: () => deleteSet(selectedSetId) }
        ]}
      />
    </div>

    <!-- Tabs -->
    <div class="mb-6" style="border-bottom: 1px solid var(--ds-border);">
      <nav class="flex gap-4">
        <button
          class="pb-2 px-1 border-b-2 transition-colors {activeTab === 'types' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
          onclick={() => activeTab = 'types'}
        >
          <Settings class="w-4 h-4 inline mr-1" />
          {t('assets.types')}
        </button>
        <button
          class="pb-2 px-1 border-b-2 transition-colors {activeTab === 'categories' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
          onclick={() => activeTab = 'categories'}
        >
          <FolderTree class="w-4 h-4 inline mr-1" />
          {t('assets.categories')}
        </button>
        <button
          class="pb-2 px-1 border-b-2 transition-colors {activeTab === 'permissions' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
          onclick={() => activeTab = 'permissions'}
        >
          <Users class="w-4 h-4 inline mr-1" />
          {t('assets.permissions')}
        </button>
      </nav>
    </div>

    <!-- Types Tab -->
    {#if activeTab === 'types'}
      <div class="mb-4 flex justify-end">
        <Button onclick={showAddTypeForm}>
          <Plus class="w-4 h-4 mr-1" />
          {t('assets.newType')}
        </Button>
      </div>

      {#snippet createTypeButton()}
        <Button onclick={showAddTypeForm}>
          <Plus class="w-4 h-4 mr-1" />
          {t('assets.createType')}
        </Button>
      {/snippet}

      {#if assetTypes.length === 0}
        <EmptyState
          icon={Settings}
          title={t('assets.noAssetTypes')}
          description={t('assets.noAssetTypesDesc')}
          action={createTypeButton}
        />
      {:else}
        <DataTable data={assetTypes} columns={typeColumns}>
          <div slot="name" let:item={row}>
            <span class="font-medium">{row.name}</span>
            {#if row.description}
              <p class="text-xs text-gray-500">{row.description}</p>
            {/if}
          </div>
          <div slot="color" let:item={row}>
            <ColorDot color={row.color || '#6b7280'} size="lg" />
          </div>
          <div slot="status" let:item={row}>
            <Lozenge color={row.is_active ? 'green' : 'gray'}>
              {row.is_active ? 'Active' : 'Inactive'}
            </Lozenge>
          </div>
          <div slot="actions" let:item={row}>
            <div class="flex gap-1">
              <Button variant="ghost" size="sm" onclick={() => showFieldsForm(row)} title="Configure Fields">
                <Settings class="w-4 h-4" />
              </Button>
              <Button variant="ghost" size="sm" onclick={() => showEditTypeForm(row)}>
                <Edit class="w-4 h-4" />
              </Button>
              <Button variant="ghost" size="sm" onclick={() => deleteType(row.id)}>
                <Trash2 class="w-4 h-4 text-red-500" />
              </Button>
            </div>
          </div>
        </DataTable>
      {/if}
    {/if}

    <!-- Categories Tab -->
    {#if activeTab === 'categories'}
      <div class="mb-4 flex justify-end">
        <Button onclick={() => showAddCategoryForm(null)}>
          <Plus class="w-4 h-4 mr-1" />
          {t('assets.newCategory')}
        </Button>
      </div>

      {#snippet createCategoryButton()}
        <Button onclick={() => showAddCategoryForm(null)}>
          <Plus class="w-4 h-4 mr-1" />
          {t('assets.createCategory')}
        </Button>
      {/snippet}

      {#if assetCategories.length === 0}
        <EmptyState
          icon={FolderTree}
          title={t('assets.noCategories')}
          description={t('assets.noCategoriesDesc')}
          action={createCategoryButton}
        />
      {:else}
        <div class="rounded-lg" style="border: 1px solid var(--ds-border);">
          {#snippet renderCategory(category, level = 0)}
            <div
              class="flex items-center justify-between p-3 category-row"
              style="padding-left: {16 + level * 24}px; border-bottom: 1px solid var(--ds-border);"
            >
              <div class="flex items-center gap-2">
                {#if category.has_children}
                  <button
                    onclick={() => toggleCategory(category.id)}
                    class="p-1 rounded"
                    style="background: transparent;"
                    onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
                    onmouseleave={(e) => e.currentTarget.style.background = 'transparent'}
                  >
                    {#if expandedCategories.has(category.id)}
                      <ChevronDown class="w-4 h-4" style="color: var(--ds-icon);" />
                    {:else}
                      <ChevronRight class="w-4 h-4" style="color: var(--ds-icon);" />
                    {/if}
                  </button>
                {:else}
                  <span class="w-6"></span>
                {/if}
                {#if expandedCategories.has(category.id)}
                  <FolderOpen class="w-4 h-4 text-yellow-500" />
                {:else}
                  <Folder class="w-4 h-4 text-yellow-500" />
                {/if}
                <span class="font-medium" style="color: var(--ds-text);">{category.name}</span>
                {#if category.asset_count > 0}
                  <span class="text-xs" style="color: var(--ds-text-subtlest);">({category.asset_count})</span>
                {/if}
              </div>
              <div class="flex gap-1">
                <Button variant="ghost" size="sm" onclick={() => showAddCategoryForm(category.id)} title="Add subcategory">
                  <Plus class="w-4 h-4" />
                </Button>
                <Button variant="ghost" size="sm" onclick={() => showEditCategoryForm(category)}>
                  <Edit class="w-4 h-4" />
                </Button>
                <Button variant="ghost" size="sm" onclick={() => deleteCategory(category.id)}>
                  <Trash2 class="w-4 h-4 text-red-500" />
                </Button>
              </div>
            </div>
            {#if category.has_children && expandedCategories.has(category.id) && category.children}
              {#each category.children as child}
                {@render renderCategory(child, level + 1)}
              {/each}
            {/if}
          {/snippet}
          {#each assetCategories as category}
            {@render renderCategory(category)}
          {/each}
        </div>
      {/if}
    {/if}

    <!-- Permissions Tab -->
    {#if activeTab === 'permissions'}
      <!-- Everyone Default Section -->
      <div class="mb-6 p-4 rounded-lg" style="background: var(--ds-surface); border: 1px solid var(--ds-border);">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('assets.everyoneRole')}</h3>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {t('assets.everyoneRoleDesc')}
            </p>
          </div>
          <div class="w-48">
            <Select bind:value={everyoneRoleId} onchange={handleEveryoneRoleChange}>
              <option value={null}>{t('common.none')}</option>
              {#each assetRoles as role}
                <option value={role.id}>{role.name}</option>
              {/each}
            </Select>
          </div>
        </div>
      </div>

      <!-- Role Assignments -->
      <div class="mb-4 flex justify-between items-center">
        <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{t('assets.permissions')}</h3>
        <Button onclick={showAddRoleForm}>
          <Plus class="w-4 h-4 mr-1" />
          {t('assets.assignRole')}
        </Button>
      </div>

      {#snippet assignRoleButton()}
        <Button onclick={showAddRoleForm}>
          <Plus class="w-4 h-4 mr-1" />
          {t('assets.assignRole')}
        </Button>
      {/snippet}

      {#if allRoleAssignments().length === 0}
        <EmptyState
          icon={Users}
          title={t('assets.noRoleAssignments')}
          description={t('assets.noRoleAssignmentsDesc')}
          action={assignRoleButton}
        />
      {:else}
        <DataTable data={allRoleAssignments()} columns={roleColumns}>
          <div slot="assignee" let:item={row}>
            <div class="flex items-center gap-2">
              {#if row.type === 'user'}
                <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium" style="background-color: rgb(219 234 254); color: rgb(30 64 175);">
                  <User class="w-3 h-3" />
                  {row.assignee_name}
                </span>
              {:else}
                <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium" style="background-color: rgb(237 233 254); color: rgb(91 33 182);">
                  <Users class="w-3 h-3" />
                  {row.assignee_name}
                </span>
              {/if}
            </div>
          </div>
          <div slot="role" let:item={row}>
            <Lozenge color={row.role_name === 'Administrator' ? 'red' : row.role_name === 'Editor' ? 'yellow' : 'blue'}>
              {row.role_name}
            </Lozenge>
          </div>
          <div slot="actions" let:item={row}>
            <Button variant="ghost" size="sm" onclick={() => revokeRole(row.id, row.type)}>
              <X class="w-4 h-4" style="color: var(--ds-text-danger);" />
            </Button>
          </div>
        </DataTable>
      {/if}
    {/if}
  {/if}
  </div>
</div>

<!-- Asset Set Form Modal -->
<Modal isOpen={showSetForm} onclose={() => showSetForm = false}>
  <ModalHeader title={editingSet ? t('assets.editSet') : t('assets.createAssetSet')} onClose={() => showSetForm = false} />
  <form onsubmit={(e) => { e.preventDefault(); handleSetSubmit(); }} class="p-6">
    <div class="space-y-4">
      <div>
        <Label color="default" class="mb-1">{t('common.name')}</Label>
        <input
          type="text"
          bind:value={setFormData.name}
          required
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        />
      </div>
      <div>
        <Label color="default" class="mb-1">{t('common.description')}</Label>
        <textarea
          bind:value={setFormData.description}
          rows="3"
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        ></textarea>
      </div>
      <Checkbox bind:checked={setFormData.is_default} label={t('assets.default')} />
    </div>
    <div class="flex justify-end gap-2 mt-6">
      <Button variant="outline" type="button" onclick={() => showSetForm = false}>{t('common.cancel')}</Button>
      <Button type="submit">{editingSet ? t('common.save') : t('common.create')}</Button>
    </div>
  </form>
</Modal>

<!-- Asset Type Form Modal -->
<Modal isOpen={showTypeForm} onclose={() => showTypeForm = false}>
  <ModalHeader title={editingType ? t('assets.editType') : t('assets.createType')} onClose={() => showTypeForm = false} />
  <form onsubmit={(e) => { e.preventDefault(); handleTypeSubmit(); }} class="p-6">
    <div class="space-y-4">
      <div>
        <Label color="default" class="mb-1">{t('common.name')}</Label>
        <input
          type="text"
          bind:value={typeFormData.name}
          required
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        />
      </div>
      <div>
        <Label color="default" class="mb-1">{t('common.description')}</Label>
        <textarea
          bind:value={typeFormData.description}
          rows="2"
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        ></textarea>
      </div>
      <div>
        <ColorPicker bind:value={typeFormData.color} label={t('common.color')} />
      </div>
      <Checkbox bind:checked={typeFormData.is_active} label={t('common.active')} />
    </div>
    <div class="flex justify-end gap-2 mt-6">
      <Button variant="outline" type="button" onclick={() => showTypeForm = false}>{t('common.cancel')}</Button>
      <Button type="submit">{editingType ? t('common.save') : t('common.create')}</Button>
    </div>
  </form>
</Modal>

<!-- Category Form Modal -->
<Modal isOpen={showCategoryForm} onclose={() => showCategoryForm = false}>
  <ModalHeader title={editingCategory ? t('assets.editCategory') : t('assets.createCategory')} onClose={() => showCategoryForm = false} />
  <form onsubmit={(e) => { e.preventDefault(); handleCategorySubmit(); }} class="p-6">
    <div class="space-y-4">
      <div>
        <Label color="default" class="mb-1">{t('common.name')}</Label>
        <input
          type="text"
          bind:value={categoryFormData.name}
          required
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        />
      </div>
      <div>
        <Label color="default" class="mb-1">{t('common.description')}</Label>
        <textarea
          bind:value={categoryFormData.description}
          rows="2"
          class="w-full px-3 py-2 rounded-lg"
          style="background: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        ></textarea>
      </div>
      <div>
        <Label color="default" class="mb-1">{t('assets.parentCategory')}</Label>
        <Select bind:value={categoryFormData.parent_id}>
          <option value={null}>{t('assets.noParent')}</option>
          {#each flatCategories.filter(c => c.id !== editingCategory?.id) as cat}
            <option value={cat.id}>{'  '.repeat(cat.level)}{cat.name}</option>
          {/each}
        </Select>
      </div>
    </div>
    <div class="flex justify-end gap-2 mt-6">
      <Button variant="outline" type="button" onclick={() => showCategoryForm = false}>{t('common.cancel')}</Button>
      <Button type="submit">{editingCategory ? t('common.save') : t('common.create')}</Button>
    </div>
  </form>
</Modal>

<!-- Role Assignment Form Modal -->
<Modal isOpen={showRoleForm} onclose={() => showRoleForm = false}>
  <ModalHeader title={t('assets.assignRole')} onClose={() => showRoleForm = false} />
  <form onsubmit={(e) => { e.preventDefault(); handleRoleSubmit(); }} class="p-6">
    <div class="space-y-4">
      <!-- Assignee Type Toggle -->
      <div>
        <Label color="default" class="mb-2">{t('common.assignTo')}</Label>
        <div class="flex gap-2">
          <button
            type="button"
            class="flex-1 px-3 py-2 text-sm rounded-lg border transition-colors {roleFormData.type === 'user' ? 'border-blue-500 bg-blue-50 text-blue-700' : 'border-gray-300 text-gray-600 hover:bg-gray-50'}"
            onclick={() => roleFormData.type = 'user'}
          >
            <User class="w-4 h-4 inline mr-1" />
            {t('common.user')}
          </button>
          <button
            type="button"
            class="flex-1 px-3 py-2 text-sm rounded-lg border transition-colors {roleFormData.type === 'group' ? 'border-purple-500 bg-purple-50 text-purple-700' : 'border-gray-300 text-gray-600 hover:bg-gray-50'}"
            onclick={() => roleFormData.type = 'group'}
          >
            <Users class="w-4 h-4 inline mr-1" />
            {t('common.group')}
          </button>
        </div>
      </div>

      <!-- User/Group Select -->
      {#if roleFormData.type === 'user'}
        <div>
          <Label color="default" class="mb-1">{t('common.user')}</Label>
          <Select bind:value={roleFormData.user_id} required>
            <option value={null}>{t('pickers.selectUser')}</option>
            {#each availableUsers as user}
              <option value={user.id}>{user.display_name || user.username} ({user.email})</option>
            {/each}
          </Select>
        </div>
      {:else}
        <div>
          <Label color="default" class="mb-1">{t('common.group')}</Label>
          <Select bind:value={roleFormData.group_id} required>
            <option value={null}>{t('pickers.selectGroup')}</option>
            {#each availableGroups as group}
              <option value={group.id}>{group.name}</option>
            {/each}
          </Select>
        </div>
      {/if}

      <!-- Role Select -->
      <div>
        <Label color="default" class="mb-1">{t('assets.role')}</Label>
        <Select bind:value={roleFormData.role_id} required>
          {#each assetRoles as role}
            <option value={role.id}>{role.name}{role.description ? ` - ${role.description}` : ''}</option>
          {/each}
        </Select>
      </div>
    </div>
    <div class="flex justify-end gap-2 mt-6">
      <Button variant="outline" type="button" onclick={() => showRoleForm = false}>{t('common.cancel')}</Button>
      <Button type="submit">{t('common.assign')}</Button>
    </div>
  </form>
</Modal>

<!-- Field Assignment Modal -->
<FieldLayoutEditor
  bind:isOpen={showFieldsModal}
  title="Configure Fields"
  subtitle={editingTypeForFields?.name || ''}
  {availableFields}
  bind:selectedFields={typeFields}
  showRequiredToggle={true}
  protectedFieldIds={['title']}
  showTypeLabels={true}
  onSave={handleFieldsSubmit}
  onCancel={handleFieldsCancel}
/>

