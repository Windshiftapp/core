<script>
  import { Plus, X } from 'lucide-svelte';
  import { APP_NAME } from '../constants.js';
  import { portalStore } from '../stores/portal.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
</script>

<!-- Footer -->
<footer class="border-t mt-auto" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  <div class="w-full px-6 py-8">
    <div class="max-w-[80%] mx-auto">
      <!-- 3-Column Footer Layout -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-8 mb-6">
        {#each portalStore.footerColumns as column, columnIndex}
          <div>
            <!-- Column Title -->
            {#if portalStore.isEditing}
              <input
                type="text"
                value={column.title}
                oninput={(e) => portalStore.updateColumnTitle(columnIndex, e.target.value)}
                class="text-sm font-semibold mb-3 bg-transparent border-b border-dashed focus:outline-none w-full"
                style="color: var(--ds-text); border-color: var(--ds-border);"
                placeholder="Column {columnIndex + 1} title"
              />
            {:else if column.title}
              <h3 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">
                {column.title}
              </h3>
            {/if}

            <!-- Column Links -->
            <div class="space-y-2">
              {#each column.links as link, linkIndex}
                <div class="flex items-center gap-2">
                  {#if portalStore.isEditing}
                    <div class="flex-1 space-y-1">
                      <input
                        type="text"
                        value={link.text}
                        oninput={(e) => portalStore.updateFooterLink(columnIndex, linkIndex, 'text', e.target.value)}
                        class="w-full text-sm bg-transparent border-b border-dashed focus:outline-none"
                        style="color: var(--ds-text-subtle); border-color: var(--ds-border);"
                        placeholder="Link text"
                      />
                      <input
                        type="text"
                        value={link.url}
                        oninput={(e) => portalStore.updateFooterLink(columnIndex, linkIndex, 'url', e.target.value)}
                        class="w-full text-xs bg-transparent border-b border-dashed focus:outline-none"
                        style="color: var(--ds-text-subtle); border-color: var(--ds-border);"
                        placeholder="URL"
                      />
                    </div>
                    <button
                      onclick={() => portalStore.removeFooterLink(columnIndex, linkIndex)}
                      class="p-1 rounded transition-all hover:bg-red-100"
                      title={t('portal.removeLink')}
                    >
                      <X class="w-3 h-3 text-red-600" />
                    </button>
                  {:else if link.text && link.url}
                    <a
                      href={link.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      class="text-sm hover:opacity-80 transition-opacity"
                      style="color: var(--ds-text-subtle);"
                    >
                      {link.text}
                    </a>
                  {/if}
                </div>
              {/each}

              <!-- Add Link Button (Edit Mode) -->
              {#if portalStore.isEditing}
                <button
                  onclick={() => portalStore.addFooterLink(columnIndex)}
                  class="flex items-center gap-1 text-xs px-2 py-1 rounded border border-dashed transition-all hover:border-solid"
                  style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
                >
                  <Plus class="w-3 h-3" />
                  <span>{t('portal.addLink')}</span>
                </button>
              {/if}
            </div>
          </div>
        {/each}
      </div>

      <!-- "Powered by {APP_NAME}" - Not configurable -->
      <div class="border-t pt-3 text-center" style="border-color: var(--ds-border);">
        <p class="text-xs" style="color: var(--ds-text-subtle);">
          Powered by {APP_NAME}
        </p>
      </div>
    </div>
  </div>
</footer>
