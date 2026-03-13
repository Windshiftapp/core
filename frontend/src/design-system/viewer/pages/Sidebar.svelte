<script>
  import SidebarContainer from '../../../lib/layout/SidebarContainer.svelte'
  import SidebarHeader from '../../../lib/layout/SidebarHeader.svelte'
  import SidebarSection from '../../../lib/layout/SidebarSection.svelte'
  import SidebarItem from '../../../lib/layout/SidebarItem.svelte'
  import SidebarSearch from '../../../lib/layout/SidebarSearch.svelte'
  import { Layers, Home, Users, FileText, Settings, HelpCircle, FolderOpen, BarChart, Inbox } from 'lucide-svelte'

  let searchValue = $state('')
  let activeItem = $state('dashboard')
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Sidebar Components</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A set of composable sidebar components for building navigation sidebars.
  </p>

  <!-- Complete Example -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Complete Example</h2>
    <div class="h-[500px] border rounded-lg overflow-hidden" style="border-color: var(--ds-border);">
      <SidebarContainer width="w-64">
        {#snippet header()}
          <SidebarHeader
            icon={Layers}
            title="Windshift"
            description="Design System"
          />
        {/snippet}

        <SidebarSearch bind:value={searchValue} placeholder="Search..." />

        <SidebarSection title="Main">
          <SidebarItem
            icon={Home}
            label="Dashboard"
            isActive={activeItem === 'dashboard'}
            onclick={() => activeItem = 'dashboard'}
          />
          <SidebarItem
            icon={Inbox}
            label="Inbox"
            badge="12"
            isActive={activeItem === 'inbox'}
            onclick={() => activeItem = 'inbox'}
          />
          <SidebarItem
            icon={FolderOpen}
            label="Projects"
            isActive={activeItem === 'projects'}
            onclick={() => activeItem = 'projects'}
          />
        </SidebarSection>

        <SidebarSection title="Workspace" collapsible>
          <SidebarItem
            icon={Users}
            label="Team"
            isActive={activeItem === 'team'}
            onclick={() => activeItem = 'team'}
          />
          <SidebarItem
            icon={FileText}
            label="Documents"
            isActive={activeItem === 'documents'}
            onclick={() => activeItem = 'documents'}
          />
          <SidebarItem
            icon={BarChart}
            label="Analytics"
            isActive={activeItem === 'analytics'}
            onclick={() => activeItem = 'analytics'}
          />
        </SidebarSection>

        {#snippet footer()}
          <div class="p-3">
            <SidebarItem
              icon={Settings}
              label="Settings"
              isActive={activeItem === 'settings'}
              onclick={() => activeItem = 'settings'}
            />
            <SidebarItem
              icon={HelpCircle}
              label="Help"
              isActive={activeItem === 'help'}
              onclick={() => activeItem = 'help'}
            />
          </div>
        {/snippet}
      </SidebarContainer>
    </div>
  </section>

  <!-- Individual Components -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Individual Components</h2>

    <!-- SidebarHeader -->
    <div class="mb-6">
      <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">SidebarHeader</h3>
      <div
        class="w-64 border rounded-lg overflow-hidden"
        style="border-color: var(--ds-border);"
      >
        <SidebarHeader
          icon={Layers}
          title="Project Name"
          description="Workspace subtitle"
        />
      </div>
      <div
        class="mt-2 p-3 rounded text-xs"
        style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
      >
        Props: <code>icon</code>, <code>title</code>, <code>description</code>
      </div>
    </div>

    <!-- SidebarSearch -->
    <div class="mb-6">
      <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">SidebarSearch</h3>
      <div
        class="w-64 border rounded-lg overflow-hidden"
        style="border-color: var(--ds-border); background: var(--ds-surface);"
      >
        <SidebarSearch placeholder="Search items..." />
      </div>
      <div
        class="mt-2 p-3 rounded text-xs"
        style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
      >
        Props: <code>value</code> (bindable), <code>placeholder</code>, <code>autofocus</code>
      </div>
    </div>

    <!-- SidebarSection -->
    <div class="mb-6">
      <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">SidebarSection</h3>
      <div class="flex gap-4">
        <div
          class="w-64 border rounded-lg overflow-hidden"
          style="border-color: var(--ds-border); background: var(--ds-surface);"
        >
          <SidebarSection title="Non-Collapsible">
            <SidebarItem icon={Home} label="Item 1" />
            <SidebarItem icon={Users} label="Item 2" />
          </SidebarSection>
        </div>
        <div
          class="w-64 border rounded-lg overflow-hidden"
          style="border-color: var(--ds-border); background: var(--ds-surface);"
        >
          <SidebarSection title="Collapsible" collapsible>
            <SidebarItem icon={Home} label="Item 1" />
            <SidebarItem icon={Users} label="Item 2" />
          </SidebarSection>
        </div>
      </div>
      <div
        class="mt-2 p-3 rounded text-xs"
        style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
      >
        Props: <code>title</code>, <code>expanded</code>, <code>collapsible</code>
      </div>
    </div>

    <!-- SidebarItem -->
    <div class="mb-6">
      <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">SidebarItem</h3>
      <div
        class="w-64 p-2 border rounded-lg space-y-1"
        style="border-color: var(--ds-border); background: var(--ds-surface);"
      >
        <SidebarItem icon={Home} label="Default" />
        <SidebarItem icon={Home} label="Active" isActive />
        <SidebarItem icon={Home} label="With Badge" badge="5" />
        <SidebarItem icon={Home} label="Disabled" disabled />
        <SidebarItem icon={Home} label="As Link" href="#" />
      </div>
      <div
        class="mt-2 p-3 rounded text-xs"
        style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
      >
        Props: <code>label</code>, <code>icon</code>, <code>href</code>, <code>isActive</code>, <code>disabled</code>, <code>badge</code>, <code>onclick</code>
      </div>
    </div>
  </section>

  <!-- Props Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props Reference</h2>

    <!-- SidebarContainer -->
    <div class="mb-6">
      <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text);">SidebarContainer</h3>
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
              <td class="p-2"><code>width</code></td>
              <td class="p-2">string (Tailwind class)</td>
              <td class="p-2">'w-64'</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>header</code></td>
              <td class="p-2">snippet</td>
              <td class="p-2">null</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>footer</code></td>
              <td class="p-2">snippet</td>
              <td class="p-2">null</td>
            </tr>
            <tr>
              <td class="p-2"><code>children</code></td>
              <td class="p-2">snippet</td>
              <td class="p-2">-</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- SidebarHeader -->
    <div class="mb-6">
      <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text);">SidebarHeader</h3>
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
              <td class="p-2"><code>title</code></td>
              <td class="p-2">string</td>
              <td class="p-2">-</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>description</code></td>
              <td class="p-2">string</td>
              <td class="p-2">''</td>
            </tr>
            <tr>
              <td class="p-2"><code>icon</code></td>
              <td class="p-2">Lucide icon component</td>
              <td class="p-2">null</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- SidebarItem -->
    <div class="mb-6">
      <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text);">SidebarItem</h3>
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
              <td class="p-2"><code>label</code></td>
              <td class="p-2">string</td>
              <td class="p-2">-</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>icon</code></td>
              <td class="p-2">Lucide icon component</td>
              <td class="p-2">null</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>href</code></td>
              <td class="p-2">string</td>
              <td class="p-2">null</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>isActive</code></td>
              <td class="p-2">boolean</td>
              <td class="p-2">false</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>disabled</code></td>
              <td class="p-2">boolean</td>
              <td class="p-2">false</td>
            </tr>
            <tr style="border-bottom: 1px solid var(--ds-border);">
              <td class="p-2"><code>badge</code></td>
              <td class="p-2">string</td>
              <td class="p-2">null</td>
            </tr>
            <tr>
              <td class="p-2"><code>onclick</code></td>
              <td class="p-2">function</td>
              <td class="p-2">null</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</div>
