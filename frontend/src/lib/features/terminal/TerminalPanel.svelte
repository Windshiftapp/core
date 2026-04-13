<script>
  import { onMount } from 'svelte';
  import { terminalStore } from '../../stores/terminalStore.svelte.js';
  import { workspacePathStore } from '../../stores/workspacePathStore.svelte.js';
  import { currentWorkspace } from '../../stores/workspaces.svelte.js';
  import { currentRoute } from '../../router.js';
  import { dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { IconPlus, IconX, IconTerminal, IconFolder, IconCheck, IconAlertTriangle } from '@tabler/icons-svelte-runes';
  import WsTomlProvisioner from './WsTomlProvisioner.svelte';

  let terminalContainer = $state(null);
  let isDropTarget = $state(false);
  let xtermLoaded = $state(false);
  let error = $state(null);

  // Folder picker state per tab (tabs waiting for folder selection)
  let tabsNeedingFolder = $state(new Set());
  // Provisioner overlay
  let showProvisioner = $state(false);

  // Track active PTYs and terminals per tab
  let ptyInstances = new Map();
  let termInstances = new Map(); // tabId -> { term, fitAddon, wrapperDiv }
  let initializingTabs = new Set(); // sync guard against concurrent initTerminal calls
  let prevEffectWsId; // undefined = not yet initialized

  // Detect if running inside Tauri
  const isTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;

  // Single ResizeObserver for the container
  let containerResizeObserver = null;

  let store = $derived($terminalStore);
  let workspace = $derived($currentWorkspace);
  let route = $derived($currentRoute);

  // Sync workspace context into workspacePathStore when route changes
  $effect(() => {
    const wsId = route.params?.id;
    if (wsId) {
      workspacePathStore.setWorkspace(wsId);
    }
  });

  /**
   * Show a specific tab's wrapper div, hide all others.
   * Fits the active terminal and notifies PTY of resize.
   */
  function showTab(tabId) {
    if (!terminalContainer) return;

    // Hide all wrappers
    for (const [id, entry] of termInstances) {
      if (entry.wrapperDiv) {
        entry.wrapperDiv.style.display = id === tabId ? '' : 'none';
      }
    }

    // Fit the now-visible terminal
    const entry = termInstances.get(tabId);
    if (entry) {
      requestAnimationFrame(() => {
        try {
          entry.fitAddon.fit();
          entry.term.focus();
          const ptyEntry = ptyInstances.get(tabId);
          if (ptyEntry && entry.term.cols && entry.term.rows) {
            ptyEntry.resize(entry.term.cols, entry.term.rows);
          }
        } catch {
          // Container might not be visible yet
        }
      });
    }
  }

  /**
   * Create xterm.js Terminal + addons + wrapper div. Does NOT spawn PTY.
   */
  async function initTerminal(tabId) {
    if (!terminalContainer) return;

    // Already initialized — just show it
    if (termInstances.has(tabId)) {
      showTab(tabId);
      return;
    }

    // Prevent concurrent initialization of the same tab
    if (initializingTabs.has(tabId)) return;
    initializingTabs.add(tabId);

    let Terminal, FitAddon;
    try {
      ({ Terminal } = await import('@xterm/xterm'));
      ({ FitAddon } = await import('@xterm/addon-fit'));
    } catch (err) {
      error = `xterm import failed: ${err.message}`;
      console.error('xterm import failed:', err);
      initializingTabs.delete(tabId);
      return;
    }

    try {

      const newTerm = new Terminal({
        cursorBlink: true,
        fontSize: 13,
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, Monaco, 'Courier New', monospace",
        theme: {
          background: '#1a1b26',
          foreground: '#c0caf5',
          cursor: '#c0caf5',
          selectionBackground: '#33467c',
          black: '#15161e',
          red: '#f7768e',
          green: '#9ece6a',
          yellow: '#e0af68',
          blue: '#7aa2f7',
          magenta: '#bb9af7',
          cyan: '#7dcfff',
          white: '#a9b1d6',
          brightBlack: '#414868',
          brightRed: '#f7768e',
          brightGreen: '#9ece6a',
          brightYellow: '#e0af68',
          brightBlue: '#7aa2f7',
          brightMagenta: '#bb9af7',
          brightCyan: '#7dcfff',
          brightWhite: '#c0caf5',
        },
        allowProposedApi: true,
      });

      const fitAddon = new FitAddon();
      newTerm.loadAddon(fitAddon);

      // Try WebGL addon for performance, fall back to canvas
      try {
        const { WebglAddon } = await import('@xterm/addon-webgl');
        const webglAddon = new WebglAddon();
        webglAddon.onContextLoss(() => {
          webglAddon.dispose();
        });
        newTerm.loadAddon(webglAddon);
      } catch {
        // WebGL not available, xterm.js uses canvas by default
      }

      // Create a per-tab wrapper div
      const wrapperDiv = document.createElement('div');
      wrapperDiv.setAttribute('data-tab-id', String(tabId));
      wrapperDiv.style.width = '100%';
      wrapperDiv.style.height = '100%';
      terminalContainer.appendChild(wrapperDiv);

      newTerm.open(wrapperDiv);

      requestAnimationFrame(() => {
        try {
          fitAddon.fit();
        } catch {
          // Container might not be visible yet
        }
      });

      termInstances.set(tabId, { term: newTerm, fitAddon, wrapperDiv });
      initializingTabs.delete(tabId);
      xtermLoaded = true;

      // Hide all other tabs, show this one
      showTab(tabId);

      // Now decide how to spawn the PTY
      if (isTauri) {
        const wsId = route.params?.id;
        const savedPath = workspacePathStore.path;

        if (savedPath) {
          // Path exists — spawn PTY and check ws.toml
          await spawnPtyForTab(tabId, savedPath);
          checkWsToml(savedPath);
        } else if (wsId) {
          // In a workspace but no folder set — show folder picker
          tabsNeedingFolder = new Set([...tabsNeedingFolder, tabId]);
        } else {
          // No workspace context — fall back to home dir
          let cwd;
          try {
            const { homeDir } = await import('@tauri-apps/api/path');
            cwd = await homeDir();
          } catch {
            // Fallback — no cwd
          }
          await spawnPtyForTab(tabId, cwd);
        }
      } else {
        // Browser-only mode: show message
        newTerm.writeln('\x1b[1;34mWindshift Terminal\x1b[0m');
        newTerm.writeln('');
        newTerm.writeln('Terminal requires the Windshift Desktop app (Tauri).');
        newTerm.writeln('In browser mode, drag & drop preview is available.');
        newTerm.writeln('');
        newTerm.onData((data) => {
          if (data === '\r') {
            newTerm.writeln('');
          } else if (data === '\x7f') {
            newTerm.write('\b \b');
          } else {
            newTerm.write(data);
          }
        });
      }

    } catch (err) {
      error = err.message;
      console.error('Failed to initialize terminal:', err);
      initializingTabs.delete(tabId);
    }
  }

  /**
   * Spawn a PTY process for a tab with the given working directory.
   */
  async function spawnPtyForTab(tabId, cwd) {
    const entry = termInstances.get(tabId);
    if (!entry) return;

    // Don't spawn if PTY already exists for this tab
    if (ptyInstances.has(tabId)) return;

    try {
      const { spawn } = await import('tauri-pty');
      const shell = getDefaultShell();

      const spawnOpts = {
        cols: entry.term.cols,
        rows: entry.term.rows,
        name: 'xterm-256color',
      };
      if (cwd) spawnOpts.cwd = cwd;

      const newPty = spawn(shell, [], spawnOpts);

      // Wire PTY I/O
      newPty.onData((data) => entry.term.write(data));
      entry.term.onData((data) => newPty.write(data));

      // Handle PTY exit
      newPty.onExit(({ exitCode }) => {
        entry.term.writeln(`\r\n[Process exited with code ${exitCode}]`);
      });

      ptyInstances.set(tabId, newPty);
    } catch (err) {
      console.error('Failed to spawn PTY:', err);
      entry.term.writeln('Failed to spawn terminal process: ' + err.message);
      entry.term.writeln('Running in browser-only mode.');
    }
  }

  /**
   * Check if ws.toml exists in the given folder path.
   */
  async function checkWsToml(folderPath) {
    if (!folderPath) return;
    try {
      workspacePathStore.wsTomlStatus = 'checking';
      const { exists } = await import('@tauri-apps/plugin-fs');
      const found = await exists(`${folderPath}/ws.toml`);
      workspacePathStore.wsTomlStatus = found ? 'found' : 'missing';
    } catch {
      workspacePathStore.wsTomlStatus = 'unknown';
    }
  }

  /**
   * Open folder picker and set the workspace folder path.
   */
  async function pickFolder(tabId) {
    try {
      const { open } = await import('@tauri-apps/plugin-dialog');
      const selected = await open({ directory: true, title: 'Select project folder' });

      if (selected) {
        const wsId = route.params?.id;
        if (wsId) {
          workspacePathStore.setPath(wsId, selected);
        }

        // Remove from pending set
        tabsNeedingFolder = new Set([...tabsNeedingFolder].filter(id => id !== tabId));

        // Spawn PTY with selected folder
        await spawnPtyForTab(tabId, selected);
        checkWsToml(selected);
      }
    } catch (err) {
      console.error('Failed to open folder picker:', err);
    }
  }

  function getDefaultShell() {
    const platform = navigator.platform?.toLowerCase() || '';
    if (platform.includes('win')) return 'powershell.exe';
    if (platform.includes('mac')) return '/bin/zsh';
    return '/bin/bash';
  }

  function destroyTerminal(tabId) {
    const termEntry = termInstances.get(tabId);
    if (termEntry) {
      termEntry.term?.dispose();
      termEntry.wrapperDiv?.remove();
      termInstances.delete(tabId);
    }
    const ptyEntry = ptyInstances.get(tabId);
    if (ptyEntry) {
      try {
        ptyEntry.kill();
      } catch {
        // Already dead
      }
      ptyInstances.delete(tabId);
    }
    tabsNeedingFolder = new Set([...tabsNeedingFolder].filter(id => id !== tabId));
  }

  function switchTab(tabId) {
    terminalStore.setActiveTab(tabId);
    showTab(tabId);
  }

  function addTab() {
    const newId = terminalStore.addTab();
    initTerminal(newId);
  }

  function closeTab(tabId) {
    destroyTerminal(tabId);
    terminalStore.removeTab(tabId);
    const currentStore = $terminalStore;
    showTab(currentStore.activeTabId);
  }

  // Fit the active terminal when the container resizes
  function fitActiveTerminal() {
    const currentStore = $terminalStore;
    const entry = termInstances.get(currentStore.activeTabId);
    if (!entry) return;
    try {
      entry.fitAddon.fit();
      const ptyEntry = ptyInstances.get(currentStore.activeTabId);
      if (ptyEntry && entry.term.cols && entry.term.rows) {
        ptyEntry.resize(entry.term.cols, entry.term.rows);
      }
    } catch {
      // Ignore resize errors during teardown
    }
  }

  // Handle visibility: init the first tab when panel becomes visible
  $effect(() => {
    const { visible, activeTabId } = store;
    if (visible && terminalContainer && !termInstances.has(activeTabId)) {
      const timeout = setTimeout(() => initTerminal(activeTabId), 50);
      return () => clearTimeout(timeout);
    }
  });

  // React to workspace changes: re-evaluate folder/PTY for the active tab
  $effect(() => {
    const wsId = route.params?.id;
    const { visible, activeTabId } = store;

    if (!visible || !isTauri || !termInstances.has(activeTabId)) return;

    // Only act when workspace ID actually changes
    if (wsId === prevEffectWsId) return;
    const isFirstRun = prevEffectWsId === undefined;
    prevEffectWsId = wsId;

    // Skip first run — initTerminal() already handled the initial PTY spawn
    if (isFirstRun) return;

    // Read path only when we're going to act (avoids tracking it as dependency on no-op runs)
    const savedPath = workspacePathStore.path;

    // Kill existing PTY for this tab (workspace changed, old PTY is wrong dir)
    const oldPty = ptyInstances.get(activeTabId);
    if (oldPty) {
      try { oldPty.kill(); } catch {}
      ptyInstances.delete(activeTabId);
    }

    if (savedPath) {
      tabsNeedingFolder = new Set([...tabsNeedingFolder].filter(id => id !== activeTabId));
      spawnPtyForTab(activeTabId, savedPath);
      checkWsToml(savedPath);
    } else if (wsId) {
      tabsNeedingFolder = new Set([...tabsNeedingFolder, activeTabId]);
    } else {
      tabsNeedingFolder = new Set([...tabsNeedingFolder].filter(id => id !== activeTabId));
      import('@tauri-apps/api/path').then(({ homeDir }) => homeDir()).then(cwd => {
        spawnPtyForTab(activeTabId, cwd);
      }).catch(() => {
        spawnPtyForTab(activeTabId, undefined);
      });
    }
  });

  onMount(() => {
    function handleTerminalWrite(event) {
      const { text } = event.detail;
      const currentStore = $terminalStore;
      const activePty = ptyInstances.get(currentStore.activeTabId);
      if (activePty) {
        activePty.write(text);
      } else {
        const activeTermEntry = termInstances.get(currentStore.activeTabId);
        if (activeTermEntry) {
          activeTermEntry.term.write(text);
        }
      }
    }

    window.addEventListener('terminal-write', handleTerminalWrite);

    if (terminalContainer) {
      containerResizeObserver = new ResizeObserver(() => {
        fitActiveTerminal();
      });
      containerResizeObserver.observe(terminalContainer);
    }

    return () => {
      window.removeEventListener('terminal-write', handleTerminalWrite);
      containerResizeObserver?.disconnect();
      for (const tabId of [...termInstances.keys()]) {
        destroyTerminal(tabId);
      }
    };
  });

  // Setup drop target on the terminal container
  $effect(() => {
    if (!terminalContainer) return;

    const cleanup = dropTargetForElements({
      element: terminalContainer,
      canDrop: ({ source }) => source.data.type === 'work-item',
      onDragEnter: () => {
        isDropTarget = true;
      },
      onDragLeave: () => {
        isDropTarget = false;
      },
      onDrop: ({ source }) => {
        isDropTarget = false;
        const item = source.data.item;
        if (item) {
          const prompt = formatItemAsPrompt(item);
          terminalStore.writeToTerminal(prompt);
        }
      },
    });

    return cleanup;
  });

  function formatItemAsPrompt(item) {
    const key = item.workspace_key && item.workspace_item_number
      ? `${item.workspace_key}-${item.workspace_item_number}`
      : item.title;

    const lines = [`Work on ${key}: ${item.title}`];

    if (item.description) {
      const desc = stripHtml(item.description).substring(0, 500);
      if (desc.trim()) {
        lines.push('', `Description: ${desc}`);
      }
    }

    if (item.priority_name) lines.push(`Priority: ${item.priority_name}`);
    if (item.status_name) lines.push(`Status: ${item.status_name}`);
    if (item.assignee_name) lines.push(`Assignee: ${item.assignee_name}`);
    if (item.due_date) lines.push(`Due: ${item.due_date}`);
    if (item.milestone_name) lines.push(`Milestone: ${item.milestone_name}`);
    if (item.iteration_name) lines.push(`Iteration: ${item.iteration_name}`);

    if (item.label_names?.length) {
      lines.push(`Labels: ${item.label_names.join(', ')}`);
    }

    return lines.join('\n');
  }

  function stripHtml(html) {
    if (!html) return '';
    const tmp = document.createElement('div');
    tmp.innerHTML = html;
    return tmp.textContent || tmp.innerText || '';
  }

  function handleProvisionerComplete() {
    showProvisioner = false;
    if (workspacePathStore.path) {
      checkWsToml(workspacePathStore.path);
    }
  }
</script>

<div class="terminal-panel flex flex-col h-full" style="background-color: #1a1b26;">
  <!-- Tab Bar -->
  <div class="terminal-tab-bar flex items-center gap-0.5 px-2 py-1 border-b" style="background-color: #16161e; border-color: #292e42;" role="tablist">
    {#each store.tabs as tab (tab.id)}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="terminal-tab flex items-center gap-1.5 px-3 py-1 text-xs rounded-t cursor-pointer transition-colors {tab.id === store.activeTabId ? 'active' : ''}"
        onclick={() => switchTab(tab.id)}
        onkeydown={(e) => { if (e.key === 'Enter') switchTab(tab.id); }}
        role="tab"
        tabindex="0"
        aria-selected={tab.id === store.activeTabId}
      >
        <IconTerminal class="w-3 h-3" />
        <span>{tab.title}</span>
        {#if store.tabs.length > 1}
          <button
            class="terminal-tab-close ml-1 rounded hover:bg-white/10 p-0.5 cursor-pointer"
            onclick={(e) => { e.stopPropagation(); closeTab(tab.id); }}
            aria-label="Close tab"
          >
            <IconX class="w-3 h-3" />
          </button>
        {/if}
      </div>
    {/each}
    <button
      class="terminal-tab-add p-1 rounded hover:bg-white/10 ml-1 cursor-pointer"
      onclick={addTab}
      aria-label="New terminal"
    >
      <IconPlus class="w-3.5 h-3.5" style="color: #565f89;" />
    </button>

    <!-- Right side: workspace info + ws.toml status -->
    {#if workspace && route.params?.id}
      <div class="ml-auto flex items-center gap-2 px-2">
        <span class="text-xs" style="color: #565f89;">{workspace.name}</span>
        {#if workspacePathStore.wsTomlStatus === 'found'}
          <span class="flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: rgba(158, 206, 106, 0.15); color: #9ece6a;">
            <IconCheck class="w-3 h-3" />
            ws.toml
          </span>
        {:else if workspacePathStore.wsTomlStatus === 'missing'}
          <button
            class="flex items-center gap-1 px-2 py-0.5 rounded text-xs cursor-pointer transition-colors"
            style="background-color: rgba(224, 175, 104, 0.15); color: #e0af68;"
            onclick={() => { showProvisioner = true; }}
          >
            <IconAlertTriangle class="w-3 h-3" />
            ws.toml
          </button>
        {/if}
      </div>
    {/if}

    <!-- Close terminal panel -->
    <button
      class="p-1 rounded hover:bg-white/10 cursor-pointer {workspace && route.params?.id ? '' : 'ml-auto'}"
      onclick={() => terminalStore.hide()}
      aria-label="Close terminal"
    >
      <IconX class="w-3.5 h-3.5" style="color: #565f89;" />
    </button>
  </div>

  <!-- Terminal Container -->
  <div
    class="terminal-container flex-1 relative overflow-hidden"
    class:drop-active={isDropTarget}
    bind:this={terminalContainer}
  >
    {#if isDropTarget}
      <div class="drop-overlay absolute inset-0 flex items-center justify-center z-10 pointer-events-none">
        <div class="drop-text px-4 py-2 rounded-lg text-sm font-medium" style="background-color: rgba(122, 162, 247, 0.2); color: #7aa2f7; border: 1px solid #7aa2f7;">
          Drop to create prompt
        </div>
      </div>
    {/if}

    {#if error}
      <div class="flex items-center justify-center h-full text-red-400 text-sm p-4">
        Failed to load terminal: {error}
      </div>
    {/if}

    <!-- Folder picker overlay for tabs waiting on a folder -->
    {#if tabsNeedingFolder.has(store.activeTabId)}
      <div class="folder-picker-overlay absolute inset-0 z-15 flex flex-col items-center justify-center gap-4" style="background-color: rgba(26, 27, 38, 0.95);">
        <IconFolder class="w-10 h-10" style="color: #565f89;" />
        <p class="text-sm" style="color: #c0caf5;">
          Set a project folder for <strong style="color: #7aa2f7;">{workspace?.name || 'this workspace'}</strong>
        </p>
        <button
          class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-colors"
          style="background-color: #7aa2f7; color: #1a1b26;"
          onclick={() => pickFolder(store.activeTabId)}
        >
          <IconFolder class="w-4 h-4" />
          Choose Folder
        </button>
      </div>
    {/if}

    <!-- ws.toml provisioner overlay -->
    {#if showProvisioner && workspacePathStore.path}
      <WsTomlProvisioner
        folderPath={workspacePathStore.path}
        workspaceKey={workspace?.key || ''}
        serverUrl={typeof window !== 'undefined' ? window.location.origin : ''}
        onComplete={handleProvisionerComplete}
        onCancel={() => { showProvisioner = false; }}
      />
    {/if}
  </div>
</div>

<style>
  .terminal-panel {
    min-height: 0;
  }

  .terminal-container {
    padding: 4px;
  }

  .terminal-container.drop-active {
    box-shadow: inset 0 0 0 2px #7aa2f7;
  }

  .terminal-tab {
    color: #565f89;
  }

  .terminal-tab.active {
    background-color: #1a1b26;
    color: #c0caf5;
  }

  .terminal-tab:not(.active):hover {
    background-color: rgba(255, 255, 255, 0.05);
  }

  /* Override xterm.js defaults to fill container */
  .terminal-container :global(.xterm) {
    height: 100%;
    padding: 4px;
  }

  .terminal-container :global(.xterm-viewport) {
    overflow-y: auto !important;
  }

  .terminal-container :global(.xterm-screen) {
    height: 100% !important;
  }

  .drop-overlay {
    background-color: rgba(26, 27, 38, 0.7);
  }
</style>
