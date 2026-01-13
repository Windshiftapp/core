<script>
  import DropdownMenu from '../../../lib/DropdownMenu.svelte'
  import { Settings, User, LogOut, Edit, Trash2, Copy, Archive, Star, MoreHorizontal, ChevronDown, Filter } from 'lucide-svelte'

  let searchValue = ''
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">DropdownMenu</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Rich dropdown menu component built on Melt UI with support for icons, subtitles, dividers, and search.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <DropdownMenu
      triggerText="Options"
      triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
      items={[
        { id: 'edit', title: 'Edit', icon: Edit, onClick: () => alert('Edit') },
        { id: 'copy', title: 'Copy', icon: Copy, onClick: () => alert('Copy') },
        { type: 'divider' },
        { id: 'delete', title: 'Delete', icon: Trash2, hoverClass: 'hover:bg-red-50 hover:text-red-700', onClick: () => alert('Delete') }
      ]}
    />
  </section>

  <!-- With Icons and Subtitles -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icons and Subtitles</h2>
    <DropdownMenu
      triggerText="User Menu"
      triggerIcon={User}
      triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
      items={[
        {
          id: 'profile',
          title: 'Profile',
          subtitle: 'View your profile',
          icon: User,
          iconColor: '#3b82f6',
          onClick: () => alert('Profile')
        },
        {
          id: 'settings',
          title: 'Settings',
          subtitle: 'Configure preferences',
          icon: Settings,
          iconColor: '#6b7280',
          onClick: () => alert('Settings')
        },
        { type: 'divider' },
        {
          id: 'logout',
          title: 'Log Out',
          subtitle: 'Sign out of your account',
          icon: LogOut,
          iconColor: '#ef4444',
          hoverClass: 'hover:bg-red-50',
          onClick: () => alert('Logout')
        }
      ]}
    />
  </section>

  <!-- Icon Only Trigger -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Icon Only Trigger</h2>
    <DropdownMenu
      triggerIcon={MoreHorizontal}
      iconOnly
      showChevron={false}
      triggerClass="p-2 border rounded hover:bg-gray-50"
      items={[
        { id: 'edit', title: 'Edit', icon: Edit },
        { id: 'archive', title: 'Archive', icon: Archive },
        { id: 'star', title: 'Star', icon: Star },
        { type: 'divider' },
        { id: 'delete', title: 'Delete', icon: Trash2, hoverClass: 'hover:bg-red-50 hover:text-red-700' }
      ]}
    />
  </section>

  <!-- With Search -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Search</h2>
    <DropdownMenu
      triggerText="Select Item"
      triggerIcon={Filter}
      triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
      items={[
        {
          type: 'search',
          placeholder: 'Search items...',
          value: searchValue,
          onInput: (v) => searchValue = v
        },
        { type: 'divider' },
        { id: 'item1', title: 'Item 1' },
        { id: 'item2', title: 'Item 2' },
        { id: 'item3', title: 'Item 3' },
        { id: 'item4', title: 'Item 4' },
        { id: 'item5', title: 'Item 5' }
      ]}
    />
  </section>

  <!-- With Colored Background -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Colored Background</h2>
    <div class="flex gap-4">
      <DropdownMenu
        triggerText="Blue"
        triggerBgColor="#3b82f6"
        items={[
          { id: 'opt1', title: 'Option 1' },
          { id: 'opt2', title: 'Option 2' }
        ]}
      />
      <DropdownMenu
        triggerText="Green"
        triggerBgColor="#22c55e"
        items={[
          { id: 'opt1', title: 'Option 1' },
          { id: 'opt2', title: 'Option 2' }
        ]}
      />
      <DropdownMenu
        triggerText="Purple"
        triggerBgColor="#8b5cf6"
        items={[
          { id: 'opt1', title: 'Option 1' },
          { id: 'opt2', title: 'Option 2' }
        ]}
      />
    </div>
  </section>

  <!-- Placements -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Placements</h2>
    <div class="flex gap-4 flex-wrap">
      <DropdownMenu
        triggerText="Bottom"
        placement="bottom"
        triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
        items={[{ id: '1', title: 'Item 1' }, { id: '2', title: 'Item 2' }]}
      />
      <DropdownMenu
        triggerText="Top"
        placement="top"
        triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
        items={[{ id: '1', title: 'Item 1' }, { id: '2', title: 'Item 2' }]}
      />
      <DropdownMenu
        triggerText="Left"
        placement="left"
        triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
        items={[{ id: '1', title: 'Item 1' }, { id: '2', title: 'Item 2' }]}
      />
      <DropdownMenu
        triggerText="Right"
        placement="right"
        triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
        items={[{ id: '1', title: 'Item 1' }, { id: '2', title: 'Item 2' }]}
      />
    </div>
  </section>

  <!-- With Checkboxes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Checkboxes</h2>
    <DropdownMenu
      triggerText="Filters"
      triggerIcon={Filter}
      triggerClass="px-4 py-2 border rounded hover:bg-gray-50"
      items={[
        { id: 'active', type: 'checkbox', title: 'Active', checked: true, onChange: (v) => console.log('Active:', v), closeOnSelect: false },
        { id: 'pending', type: 'checkbox', title: 'Pending', checked: false, onChange: (v) => console.log('Pending:', v), closeOnSelect: false },
        { id: 'completed', type: 'checkbox', title: 'Completed', checked: true, onChange: (v) => console.log('Completed:', v), closeOnSelect: false }
      ]}
    />
  </section>

  <!-- Item Types Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Item Types</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <pre class="text-sm overflow-x-auto" style="color: var(--ds-text);">{`// Regular item
{ id: 'edit', title: 'Edit', subtitle: 'Optional', icon: Edit, onClick: () => {} }

// Divider
{ type: 'divider' }

// Text (non-interactive)
{ type: 'text', text: 'No items found' }

// Search input
{ type: 'search', placeholder: 'Search...', value: '', onInput: (v) => {} }

// Checkbox
{ type: 'checkbox', title: 'Option', checked: false, onChange: (v) => {} }

// Group (nested items)
{ type: 'group', items: [...] }`}</pre>
    </div>
  </section>

  <!-- Props Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props Reference</h2>
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
            <td class="p-2"><code>triggerText</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>triggerIcon</code></td>
            <td class="p-2">Lucide icon component</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>triggerAvatar</code></td>
            <td class="p-2">string (URL)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>triggerBgColor</code></td>
            <td class="p-2">string (hex color)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>triggerIconBgColor</code></td>
            <td class="p-2">string (hex color)</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>items</code></td>
            <td class="p-2">MenuItem[]</td>
            <td class="p-2">[]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placement</code></td>
            <td class="p-2">'bottom' | 'top' | 'left' | 'right'</td>
            <td class="p-2">'bottom'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>maxWidth</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'max-w-3xl'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showChevron</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>iconOnly</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>onOpen</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
