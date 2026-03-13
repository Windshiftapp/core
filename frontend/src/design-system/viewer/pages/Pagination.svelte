<script>
  import Pagination from '../../../lib/components/Pagination.svelte'

  let page1 = 1
  let page2 = 5
  let page3 = 1
  let pageSize = 25
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Pagination</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Pagination component for navigating through large datasets with page size controls.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <Pagination
        currentPage={page1}
        totalItems={150}
        itemsPerPage={25}
        on:pageChange={(e) => page1 = e.detail.page}
      />
    </div>
  </section>

  <!-- Many Pages -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Many Pages</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      When there are many pages, the component shows ellipsis for better navigation.
    </p>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <Pagination
        currentPage={page2}
        totalItems={500}
        itemsPerPage={25}
        on:pageChange={(e) => page2 = e.detail.page}
      />
    </div>
  </section>

  <!-- With Page Size Options -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Page Size Options</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <Pagination
        currentPage={page3}
        totalItems={250}
        itemsPerPage={pageSize}
        showPageSizes
        pageSizeOptions={[10, 25, 50, 100]}
        on:pageChange={(e) => {
          page3 = e.detail.page
        }}
        on:pageSizeChange={(e) => {
          page3 = e.detail.page
          pageSize = e.detail.itemsPerPage
        }}
      />
    </div>
  </section>

  <!-- Compact Mode -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Compact Mode</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <Pagination
        currentPage={1}
        totalItems={100}
        itemsPerPage={10}
        compact
      />
    </div>
  </section>

  <!-- Max Items Limit -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Max Items Limit</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Shows a warning when total items exceeds the max limit.
    </p>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <Pagination
        currentPage={1}
        totalItems={500}
        itemsPerPage={25}
        maxItems={100}
      />
    </div>
  </section>

  <!-- On Gradient Background -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">On Gradient Background</h2>
    <div
      class="p-6 rounded-lg"
      style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);"
    >
      <Pagination
        currentPage={1}
        totalItems={100}
        itemsPerPage={10}
        hasGradient
      />
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
            <td class="p-2"><code>currentPage</code></td>
            <td class="p-2">number</td>
            <td class="p-2">1</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>totalItems</code></td>
            <td class="p-2">number</td>
            <td class="p-2">0</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>itemsPerPage</code></td>
            <td class="p-2">number</td>
            <td class="p-2">50</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>maxItems</code></td>
            <td class="p-2">number</td>
            <td class="p-2">100</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showPageSizes</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>pageSizeOptions</code></td>
            <td class="p-2">number[]</td>
            <td class="p-2">[10, 25, 50]</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>compact</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>hasGradient</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Events Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Events Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Event</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Detail</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>pageChange</code></td>
            <td class="p-2">{`{ page: number, itemsPerPage: number }`}</td>
          </tr>
          <tr>
            <td class="p-2"><code>pageSizeChange</code></td>
            <td class="p-2">{`{ page: number, itemsPerPage: number }`}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
