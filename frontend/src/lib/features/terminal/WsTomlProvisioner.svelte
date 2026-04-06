<script>
  import { api } from '../../api.js';
  import { IconX, IconKey, IconCheck, IconLoader2 } from '@tabler/icons-svelte-runes';

  let { folderPath, workspaceKey = '', serverUrl = '', onComplete, onCancel } = $props();

  let token = $state('');
  let generating = $state(false);
  let writing = $state(false);
  let error = $state('');
  let tokenGenerated = $state(false);

  async function generateToken() {
    try {
      generating = true;
      error = '';
      const result = await api.createApiToken({
        name: `ws-cli-${workspaceKey}`,
        permissions: ['read', 'write'],
      });
      token = result.token;
      tokenGenerated = true;
    } catch (err) {
      error = `Failed to generate token: ${err.message}`;
    } finally {
      generating = false;
    }
  }

  async function createWsToml() {
    if (!token.trim()) {
      error = 'A token is required';
      return;
    }

    try {
      writing = true;
      error = '';

      const { writeTextFile } = await import('@tauri-apps/plugin-fs');
      const content = `[server]\nurl = "${serverUrl}"\ntoken = "${token.trim()}"\n\n[defaults]\nworkspace_key = "${workspaceKey}"\n`;

      await writeTextFile(`${folderPath}/ws.toml`, content);
      onComplete?.();
    } catch (err) {
      error = `Failed to write ws.toml: ${err.message}`;
    } finally {
      writing = false;
    }
  }
</script>

<div class="provisioner-overlay absolute inset-0 z-20 flex items-center justify-center" style="background-color: rgba(26, 27, 38, 0.95);">
  <div class="provisioner-card w-96 rounded-lg p-5" style="background-color: #1f2335; border: 1px solid #292e42;">
    <!-- Header -->
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-sm font-semibold" style="color: #c0caf5;">Configure ws.toml</h3>
      <button
        class="p-1 rounded hover:bg-white/10 cursor-pointer"
        onclick={() => onCancel?.()}
        aria-label="Close"
      >
        <IconX class="w-4 h-4" style="color: #565f89;" />
      </button>
    </div>

    <!-- Server URL (read-only) -->
    <div class="mb-3">
      <label class="block text-xs mb-1" style="color: #565f89;">Server URL</label>
      <div class="px-3 py-1.5 rounded text-xs" style="background-color: #16161e; color: #7aa2f7; border: 1px solid #292e42;">
        {serverUrl}
      </div>
    </div>

    <!-- Workspace key (read-only) -->
    <div class="mb-3">
      <label class="block text-xs mb-1" style="color: #565f89;">Workspace Key</label>
      <div class="px-3 py-1.5 rounded text-xs" style="background-color: #16161e; color: #7aa2f7; border: 1px solid #292e42;">
        {workspaceKey}
      </div>
    </div>

    <!-- Token -->
    <div class="mb-4">
      <label class="block text-xs mb-1" style="color: #565f89;">API Token</label>
      <div class="flex gap-2">
        <input
          type="text"
          bind:value={token}
          placeholder={tokenGenerated ? '' : 'Generate or paste a token'}
          class="flex-1 px-3 py-1.5 rounded text-xs outline-none"
          style="background-color: #16161e; color: #c0caf5; border: 1px solid #292e42;"
        />
        <button
          class="flex items-center gap-1 px-3 py-1.5 rounded text-xs font-medium cursor-pointer transition-colors"
          style="background-color: #292e42; color: #7aa2f7;"
          onclick={generateToken}
          disabled={generating}
        >
          {#if generating}
            <IconLoader2 class="w-3.5 h-3.5 animate-spin" />
          {:else}
            <IconKey class="w-3.5 h-3.5" />
          {/if}
          Generate
        </button>
      </div>
    </div>

    {#if error}
      <div class="mb-3 px-3 py-2 rounded text-xs" style="background-color: rgba(247, 118, 142, 0.1); color: #f7768e; border: 1px solid rgba(247, 118, 142, 0.2);">
        {error}
      </div>
    {/if}

    <!-- Actions -->
    <div class="flex justify-end gap-2">
      <button
        class="px-3 py-1.5 rounded text-xs cursor-pointer transition-colors"
        style="color: #565f89;"
        onclick={() => onCancel?.()}
      >
        Cancel
      </button>
      <button
        class="flex items-center gap-1.5 px-4 py-1.5 rounded text-xs font-medium cursor-pointer transition-colors"
        style="background-color: #9ece6a; color: #1a1b26;"
        onclick={createWsToml}
        disabled={writing || !token.trim()}
      >
        {#if writing}
          <IconLoader2 class="w-3.5 h-3.5 animate-spin" />
        {:else}
          <IconCheck class="w-3.5 h-3.5" />
        {/if}
        Create ws.toml
      </button>
    </div>

    <!-- Path info -->
    <div class="mt-3 text-xs" style="color: #414868;">
      Will be written to: {folderPath}/ws.toml
    </div>
  </div>
</div>
