<script>
  import { Users, Shield, Briefcase, Folder, Tag, Calendar, Flag } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Combobox Pickers</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Melt UI combobox-based pickers for selecting domain entities. These components auto-load data from the API and provide search/filter functionality.
  </p>

  <!-- API Dependency Notice -->
  <div
    class="mb-8 p-4 rounded-lg flex items-start gap-3"
    style="background-color: var(--ds-background-warning); border: 1px solid var(--ds-border-warning);"
  >
    <span style="color: var(--ds-icon-warning);">⚠️</span>
    <div>
      <p class="font-medium" style="color: var(--ds-text-warning);">API-Dependent Components</p>
      <p class="text-sm" style="color: var(--ds-text-warning);">
        These pickers fetch data from the backend API on mount. Live demos require a running server with data.
        Below is documentation of their interfaces and behaviors.
      </p>
    </div>
  </div>

  <!-- WorkspacePicker -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Briefcase size={20} />
      WorkspacePicker
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Multi-select combobox for selecting multiple workspaces. Selected workspaces appear as chips inside the input field.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Multi-select with chip display inside input</li>
        <li>• Searchable by name, key, or description</li>
        <li>• Workspace icon/avatar with color</li>
        <li>• Excludes personal workspaces from selection</li>
        <li>• Chips show workspace key badge</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number[] (bindable)</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select workspaces...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- GroupPicker -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Users size={20} />
      GroupPicker
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Single-select combobox for selecting a user group. Shows group icon and description.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Single selection with autocomplete</li>
        <li>• Searchable by group name or description</li>
        <li>• Green circular icon for groups</li>
        <li>• Shows group description in dropdown</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number | null (bindable)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select group...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- RolePicker -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Shield size={20} />
      RolePicker
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Single-select combobox for selecting a workspace role. Shows role icon and description.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Single selection with autocomplete</li>
        <li>• Searchable by role name or description</li>
        <li>• Blue shield icon for roles</li>
        <li>• Shows role description in dropdown</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number | null (bindable)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select role...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- CollectionPicker -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Folder size={20} />
      CollectionPicker
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Multi-select combobox for selecting collections. Shows workspace labels and collection icons.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Multi-select with chip display</li>
        <li>• Searchable by collection name or workspace</li>
        <li>• Shows workspace key badge in dropdown</li>
        <li>• Collection icon with workspace color</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number[] (bindable)</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select collections...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- LabelCombobox -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Tag size={20} />
      LabelCombobox
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Single-select combobox for selecting a label. Shows color indicator for each label.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Single selection with autocomplete</li>
        <li>• Color dot indicator for each label</li>
        <li>• Workspace-scoped labels</li>
        <li>• Searchable by label name</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number | null (bindable)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>workspaceId</code></td>
            <td class="p-2">number</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select label...'</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- MilestoneCombobox -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Calendar size={20} />
      MilestoneCombobox
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Single-select combobox for selecting a milestone. Shows category color and milestone dates.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Single selection with autocomplete</li>
        <li>• Category color indicator</li>
        <li>• Workspace-scoped milestones</li>
        <li>• Searchable by milestone name</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number | null (bindable)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>workspaceId</code></td>
            <td class="p-2">number</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select milestone...'</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- PersonalLabelCombobox -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Tag size={20} />
      PersonalLabelCombobox
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Multi-select combobox for personal labels with the ability to create new labels on-the-fly.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Multi-select with chip display</li>
        <li>• Create new labels by typing and pressing Enter</li>
        <li>• User-scoped personal labels</li>
        <li>• Color indicators for labels</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>value</code></td>
            <td class="p-2">number[] (bindable)</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>userId</code></td>
            <td class="p-2">number</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select labels...'</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- PriorityPicker -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
      <Flag size={20} />
      PriorityPicker
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Wrapper around ItemPicker configured for priority selection with icon mapping.
    </p>

    <div
      class="p-4 rounded-lg mb-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h4 class="font-medium mb-2" style="color: var(--ds-text);">Features</h4>
      <ul class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
        <li>• Single selection based on ItemPicker</li>
        <li>• Priority icons (Critical, High, Medium, Low, etc.)</li>
        <li>• Workspace-scoped priorities</li>
        <li>• Optional "Unassigned" option</li>
      </ul>
    </div>

    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>selectedPriorityId</code></td>
            <td class="p-2">number | null</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>workspaceId</code></td>
            <td class="p-2">number</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onChange</code></td>
            <td class="p-2">function</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Select priority...'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showUnassigned</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Common Events -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Events</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Event</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onSelect</code></td>
            <td class="p-2">Callback when an item is selected. Receives the selected item object directly.</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onChange</code></td>
            <td class="p-2">Callback when selection changes (for multi-select). Receives array of selected IDs.</td>
          </tr>
          <tr>
            <td class="p-2"><code>onCancel</code></td>
            <td class="p-2">Callback when dropdown closes without a selection.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Usage Pattern -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Usage Pattern</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <pre class="text-sm overflow-x-auto" style="color: var(--ds-text);">{`<script>
  import WorkspacePicker from '$lib/pickers/WorkspacePicker.svelte'
  import GroupPicker from '$lib/pickers/GroupPicker.svelte'
  import RolePicker from '$lib/pickers/RolePicker.svelte'

  let selectedWorkspaces = []
  let selectedGroup = null
  let selectedRole = null
</script>

<WorkspacePicker
  bind:value={selectedWorkspaces}
  label="Workspaces"
  onChange={(ids) => console.log('selected:', ids)}
/>

<GroupPicker
  bind:value={selectedGroup}
  label="Group"
  onSelect={(group) => console.log('selected:', group)}
/>

<RolePicker
  bind:value={selectedRole}
  label="Role"
  onSelect={(role) => console.log('selected:', role)}
/>`}</pre>
    </div>
  </section>
</div>
