<script>
  import Glyph from './Glyph.svelte'

  let { candidate } = $props()

  let mainExpanded = $state(true)
  let activeMain = $state('collections')
  let activeAdmin = $state('custom-fields')

  const mainItems = [
    { id: 'collections', label: 'Collections' },
    { id: 'time', label: 'Time & projects' },
    { id: 'milestones', label: 'Milestones' },
    { id: 'iterations', label: 'Iterations' },
    { id: 'knowledge', label: 'Knowledge base' },
    { id: 'assets', label: 'Assets' },
    { id: 'channels', label: 'Channels' },
    { id: 'portal', label: 'Portal hub' },
    { id: 'organizations', label: 'Organizations' },
    { id: 'teams', label: 'Teams' },
  ]

  const adminGroups = [
    {
      label: 'Content & structure',
      items: [
        { id: 'custom-fields', iconKey: 'customFields', label: 'Custom fields' },
        { id: 'screens', label: 'Screens' },
        { id: 'hierarchy', label: 'Hierarchy levels' },
        { id: 'item-types', iconKey: 'itemTypes', label: 'Item types' },
        { id: 'priorities', label: 'Priorities' },
      ],
    },
    {
      label: 'Workflow & process',
      items: [
        { id: 'statuses', label: 'Statuses' },
        { id: 'workflows', label: 'Workflows' },
        { id: 'approvals', label: 'Approval sets' },
      ],
    },
    {
      label: 'Integrations',
      items: [
        { id: 'ai', label: 'AI connections' },
        { id: 'scm', label: 'SCM providers' },
        { id: 'integrations', label: 'Integrations' },
        { id: 'links', label: 'Link types' },
        { id: 'attachments', label: 'Attachments' },
        { id: 'themes', label: 'Themes' },
      ],
    },
  ]

  function iconFor(key) {
    return candidate.icons[key]
  }

  function selectedIconFor(key) {
    return candidate.selectedIcons?.[key] ?? null
  }

  function mainStyle(id) {
    if (activeMain !== id) return 'color: var(--ds-text-subtle);'
    if (candidate.refined) {
      return 'background: color-mix(in srgb, var(--ds-background-selected) 58%, transparent); color: var(--ds-text); box-shadow: inset 2px 0 0 #0c66e4; font-weight: 550;'
    }
    return 'background: var(--ds-background-selected); color: var(--ds-text-selected);'
  }

  function adminStyle(id) {
    if (activeAdmin !== id) return 'color: var(--ds-text-subtle);'
    if (candidate.refined) {
      return 'background: color-mix(in srgb, var(--ds-background-selected) 58%, transparent); color: var(--ds-text); font-weight: 550;'
    }
    return 'background: var(--ds-background-selected); color: var(--ds-text-selected);'
  }
</script>

<section data-testid="icon-navigation-mock">
  <header class="mb-6 flex flex-wrap items-start justify-between gap-4">
    <div>
      <div class="mb-2 flex items-center gap-2">
        <span
          class="rounded-full bg-ds-accent-blue-subtle px-2.5 py-1 text-xs font-semibold text-ds-text-accent-blue"
          data-testid="icon-navigation-active-candidate"
        >
          {candidate.label}
        </span>
        <span class="rounded-full px-2.5 py-1 text-xs" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
          {candidate.license}
        </span>
      </div>
      <p class="max-w-2xl text-sm" style="color: var(--ds-text-subtle);">{candidate.description}</p>
    </div>
    <button
      class="rounded-md border px-3 py-2 text-sm font-medium"
      style="border-color: var(--ds-border); color: var(--ds-text); background: var(--ds-surface-raised);"
      data-testid="icon-navigation-density"
      onclick={() => (mainExpanded = !mainExpanded)}
    >
      {mainExpanded ? 'Show compact main nav' : 'Show expanded main nav'}
    </button>
  </header>

  <div class="grid gap-6 2xl:grid-cols-2">
    <article>
      <div class="mb-2 flex items-center justify-between">
        <h2 class="text-sm font-semibold" style="color: var(--ds-text);">Main navigation</h2>
        <span class="text-xs" style="color: var(--ds-text-subtlest);">{candidate.mainSize}px icons · 40px rows</span>
      </div>
      <div class="flex h-[760px] overflow-hidden rounded-xl border shadow-sm" style="border-color: var(--ds-border); background: var(--ds-surface);">
        <aside
          class="flex shrink-0 flex-col border-r py-4 transition-[width] duration-200"
          class:w-[216px]={mainExpanded}
          class:w-16={!mainExpanded}
          style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
        >
          <div class="mb-6 flex h-10 items-center gap-3 px-3.5">
            <div class="grid size-8 shrink-0 place-items-center rounded-lg bg-ds-accent-blue text-sm font-bold text-white">W</div>
            {#if mainExpanded}<span class="text-sm font-semibold" style="color: var(--ds-text);">Windshift</span>{/if}
          </div>

          <nav class="flex flex-1 flex-col gap-1 px-2.5" aria-label="Mock main navigation">
            <button
              class="flex h-10 items-center text-sm"
              class:gap-2.5={candidate.refined}
              class:gap-3={!candidate.refined}
              class:rounded-lg={candidate.refined}
              class:rounded-md={!candidate.refined}
              class:px-2={candidate.refined}
              class:px-2.5={!candidate.refined}
              style="color: var(--ds-text-subtle);"
            >
              <span
                class="grid shrink-0 place-items-center"
                class:size-7={candidate.refined}
                class:rounded-lg={candidate.refined}
                class:border={candidate.refined}
                class:border-transparent={candidate.refined}
              >
                <Glyph
                  icon={iconFor('workspace')}
                  mode={candidate.mode}
                  size={candidate.mainSize}
                  selected={false}
                />
              </span>
              {#if mainExpanded}<span>Workspaces</span>{/if}
            </button>

            {#each mainItems as item (item.id)}
              <button
                class="flex h-10 items-center text-sm transition-all"
                class:gap-2.5={candidate.refined}
                class:gap-3={!candidate.refined}
                class:rounded-lg={candidate.refined}
                class:rounded-md={!candidate.refined}
                class:px-2={candidate.refined}
                class:px-2.5={!candidate.refined}
                class:justify-center={!mainExpanded}
                style={mainStyle(item.id)}
                data-testid="icon-navigation-main-{item.id}"
                onclick={() => (activeMain = item.id)}
              >
                <span
                  class="grid shrink-0 place-items-center transition-all"
                  class:size-7={candidate.refined}
                  class:size-5={!candidate.refined}
                  class:rounded-lg={candidate.refined}
                  class:border={candidate.refined}
                  class:border-ds-accent-blue-subtle={candidate.refined && activeMain === item.id}
                  class:bg-ds-accent-blue-subtle={candidate.refined && activeMain === item.id}
                  class:text-ds-text-accent-blue={candidate.refined && activeMain === item.id}
                  class:border-transparent={candidate.refined && activeMain !== item.id}
                >
                  <Glyph
                    icon={iconFor(item.id)}
                    selectedIcon={selectedIconFor(item.id)}
                    mode={candidate.mode}
                    size={candidate.mainSize}
                    selected={activeMain === item.id}
                  />
                </span>
                {#if mainExpanded}<span class="truncate">{item.label}</span>{/if}
              </button>
            {/each}

            <div class="my-2 border-t" style="border-color: var(--ds-border);"></div>
            <button
              class="flex h-10 items-center bg-ds-interactive text-sm font-medium text-white shadow-sm"
              class:gap-2.5={candidate.refined}
              class:gap-3={!candidate.refined}
              class:rounded-lg={candidate.refined}
              class:rounded-md={!candidate.refined}
              class:px-2={candidate.refined}
              class:px-2.5={!candidate.refined}
              class:justify-center={!mainExpanded}
            >
              <span class="grid shrink-0 place-items-center {candidate.refined ? 'size-7 rounded-lg bg-white/15' : ''}">
                <Glyph icon={iconFor('create')} mode={candidate.mode} size={candidate.mainSize} selected={false} />
              </span>
              {#if mainExpanded}<span>Create</span>{/if}
            </button>

            <button
              class="flex h-10 items-center text-sm"
              class:gap-2.5={candidate.refined}
              class:gap-3={!candidate.refined}
              class:rounded-lg={candidate.refined}
              class:rounded-md={!candidate.refined}
              class:px-2={candidate.refined}
              class:px-2.5={!candidate.refined}
              class:justify-center={!mainExpanded}
              style="color: var(--ds-text-subtle);"
            >
              <span class="grid shrink-0 place-items-center" class:size-7={candidate.refined}>
                <Glyph icon={iconFor('search')} mode={candidate.mode} size={candidate.mainSize} selected={false} />
              </span>
              {#if mainExpanded}<span>Search</span>{/if}
            </button>
          </nav>

          <div class="flex flex-col gap-1 px-2.5">
            {#each [
              { key: 'settings', label: 'Administration' },
              { key: 'notifications', label: 'Notifications' },
            ] as utility}
              <button
                class="flex h-10 items-center text-sm"
                class:gap-2.5={candidate.refined}
                class:gap-3={!candidate.refined}
                class:rounded-lg={candidate.refined}
                class:rounded-md={!candidate.refined}
                class:px-2={candidate.refined}
                class:px-2.5={!candidate.refined}
                class:justify-center={!mainExpanded}
                style="color: var(--ds-text-subtle);"
              >
                <span class="grid shrink-0 place-items-center" class:size-7={candidate.refined}>
                  <Glyph icon={iconFor(utility.key)} mode={candidate.mode} size={candidate.mainSize} selected={false} />
                </span>
                {#if mainExpanded}<span>{utility.label}</span>{/if}
              </button>
            {/each}
            <div class="flex h-10 items-center gap-3 px-2.5" class:justify-center={!mainExpanded}>
              <div class="grid size-7 shrink-0 place-items-center rounded-full bg-ds-accent-purple text-[10px] font-semibold text-white">SE</div>
              {#if mainExpanded}<span class="text-sm" style="color: var(--ds-text);">Stefan Ernst</span>{/if}
            </div>
          </div>
        </aside>

        <div class="min-w-0 flex-1 p-6">
          <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtlest);">Collections</p>
          <h3 class="mt-1 text-xl font-semibold" style="color: var(--ds-text);">Product launch</h3>
          <div class="mt-5 grid gap-3 sm:grid-cols-2">
            {#each ['Open work', 'Due this week', 'Recently updated', 'Team capacity'] as card, index}
              <div class="rounded-lg border p-4" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
                <div class="mb-5 h-2 w-16 rounded bg-ds-accent-blue-subtle"></div>
                <div class="text-2xl font-semibold" style="color: var(--ds-text);">{[24, 7, 13, 82][index]}{index === 3 ? '%' : ''}</div>
                <div class="mt-1 text-xs" style="color: var(--ds-text-subtle);">{card}</div>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </article>

    <article>
      <div class="mb-2 flex items-center justify-between">
        <h2 class="text-sm font-semibold" style="color: var(--ds-text);">Administration navigation</h2>
        <span class="text-xs" style="color: var(--ds-text-subtlest);">{candidate.adminSize}px icons · dense hierarchy</span>
      </div>
      <div class="flex h-[760px] overflow-hidden rounded-xl border shadow-sm" style="border-color: var(--ds-border); background: var(--ds-surface);">
        <aside class="w-[292px] shrink-0 overflow-y-auto border-r p-4" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
          <h3 class="text-lg font-semibold" style="color: var(--ds-text);">Administration</h3>
          <label
            class="mt-4 flex h-9 items-center gap-2 border px-3"
            class:rounded-lg={candidate.refined}
            class:rounded-md={!candidate.refined}
            class:shadow-sm={candidate.refined}
            style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
          >
            <Glyph icon={iconFor('search')} mode={candidate.mode} size={candidate.adminSize} selected={false} />
            <input class="min-w-0 flex-1 bg-transparent text-sm outline-none" data-testid="icon-navigation-admin-search" placeholder="Search settings" />
          </label>

          <nav class="mt-5 space-y-5" aria-label="Mock administration navigation">
            {#each adminGroups as group}
              <div>
                <h4 class="mb-1 px-2 text-[11px] font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtlest);">{group.label}</h4>
                <div class="space-y-0.5">
                  {#each group.items as item (item.id)}
                    {@const key = item.iconKey ?? item.id}
                    <button
                      class="flex w-full items-center gap-2.5 text-left text-sm transition-all"
                      class:h-9={candidate.refined}
                      class:h-8={!candidate.refined}
                      class:rounded-lg={candidate.refined}
                      class:rounded-md={!candidate.refined}
                      class:px-1.5={candidate.refined}
                      class:px-2={!candidate.refined}
                      style={adminStyle(item.id)}
                      data-testid="icon-navigation-admin-{item.id}"
                      onclick={() => (activeAdmin = item.id)}
                    >
                      <span
                        class="grid shrink-0 place-items-center transition-all"
                        class:size-6={candidate.refined}
                        class:size-4={!candidate.refined}
                        class:rounded-md={candidate.refined}
                        class:bg-ds-interactive={candidate.refined && activeAdmin === item.id}
                        class:text-white={candidate.refined && activeAdmin === item.id}
                      >
                        <Glyph
                          icon={iconFor(key)}
                          selectedIcon={selectedIconFor(key)}
                          mode={candidate.mode}
                          size={candidate.adminSize}
                          selected={activeAdmin === item.id}
                        />
                      </span>
                      <span class="truncate">{item.label}</span>
                    </button>
                  {/each}
                </div>
              </div>
            {/each}
          </nav>
        </aside>

        <div class="min-w-0 flex-1 p-6">
          <div
            class="flex size-10 items-center justify-center bg-ds-accent-blue-subtle text-ds-text-accent-blue"
            class:rounded-xl={candidate.refined}
            class:rounded-lg={!candidate.refined}
            class:border={candidate.refined}
            class:border-ds-accent-blue-subtle={candidate.refined}
            class:shadow-sm={candidate.refined}
          >
            <Glyph icon={iconFor('customFields')} mode={candidate.mode} size={candidate.refined ? 21 : 24} selected={true} />
          </div>
          <h3 class="mt-4 text-xl font-semibold" style="color: var(--ds-text);">Custom fields</h3>
          <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">Define reusable fields for work items and assets.</p>
          <div class="mt-6 space-y-3">
            {#each ['Customer impact', 'Target release', 'Risk assessment', 'Service owner'] as field}
              <div class="flex items-center justify-between rounded-lg border p-3" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
                <div>
                  <div class="text-sm font-medium" style="color: var(--ds-text);">{field}</div>
                  <div class="mt-0.5 text-xs" style="color: var(--ds-text-subtlest);">Available in 3 item types</div>
                </div>
                <Glyph icon={iconFor('mail')} mode={candidate.mode} size={candidate.adminSize} selected={false} />
              </div>
            {/each}
          </div>
          <div class="mt-6 flex items-center gap-2 text-xs" style="color: var(--ds-text-subtlest);">
            <Glyph icon={iconFor('security')} mode={candidate.mode} size={candidate.adminSize} selected={false} />
            <span>System administration</span>
            <Glyph icon={iconFor('activity')} mode={candidate.mode} size={candidate.adminSize} selected={false} />
          </div>
        </div>
      </div>
    </article>
  </div>
</section>
