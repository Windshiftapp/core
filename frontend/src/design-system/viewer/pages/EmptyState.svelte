<script>
  import EmptyState from '../../../lib/components/EmptyState.svelte'
  import Button from '../../../lib/components/Button.svelte'
  import { Inbox, Search, FileText, Users, Plus, FolderOpen } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">EmptyState</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Empty state placeholder component for when there's no data to display.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div
      class="p-6 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <EmptyState title="No data" />
    </div>
  </section>

  <!-- With Description -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Description</h2>
    <div
      class="p-6 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <EmptyState
        title="No items found"
        description="Try adjusting your search or filter criteria."
      />
    </div>
  </section>

  <!-- Custom Icons -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Icons</h2>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div
        class="p-6 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <EmptyState
          icon={Search}
          title="No search results"
          description="We couldn't find anything matching your query."
        />
      </div>
      <div
        class="p-6 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <EmptyState
          icon={FileText}
          title="No documents"
          description="Upload your first document to get started."
        />
      </div>
      <div
        class="p-6 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <EmptyState
          icon={Users}
          title="No team members"
          description="Invite team members to collaborate."
        />
      </div>
      <div
        class="p-6 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <EmptyState
          icon={FolderOpen}
          title="No projects"
          description="Create a project to organize your work."
        />
      </div>
    </div>
  </section>

  <!-- With Action -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Action</h2>
    <div
      class="p-6 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <EmptyState
        icon={Inbox}
        title="Your inbox is empty"
        description="When you receive messages, they'll appear here."
      >
        {#snippet action()}
          <Button variant="primary" icon={Plus}>
            Compose Message
          </Button>
        {/snippet}
      </EmptyState>
    </div>
  </section>

  <!-- Without Icon -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Without Icon</h2>
    <div
      class="p-6 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <EmptyState
        icon={null}
        title="Nothing to see here"
        description="This section is currently empty."
      />
    </div>
  </section>

  <!-- Real World Examples -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Real World Examples</h2>
    <div class="space-y-4">
      <div
        class="p-6 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <EmptyState
          icon={Search}
          title={'No results for "foobar"'}
          description="Try different keywords or check your spelling."
        >
          {#snippet action()}
            <Button variant="secondary">Clear Search</Button>
          {/snippet}
        </EmptyState>
      </div>
      <div
        class="p-6 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <EmptyState
          icon={FileText}
          title="Get started with your first task"
          description="Tasks help you track work and stay organized. Create your first task to begin."
        >
          {#snippet action()}
            <Button variant="primary" icon={Plus}>Create Task</Button>
          {/snippet}
        </EmptyState>
      </div>
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
            <td class="p-2"><code>icon</code></td>
            <td class="p-2">Lucide icon component</td>
            <td class="p-2">Inbox</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>title</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'No data'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>description</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>action</code></td>
            <td class="p-2">snippet</td>
            <td class="p-2">null</td>
          </tr>
          <tr>
            <td class="p-2"><code>class</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
