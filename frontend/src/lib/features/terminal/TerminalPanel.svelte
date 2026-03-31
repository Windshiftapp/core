<script>
  import { onMount } from 'svelte';
  import { terminalStore } from '../../stores/terminalStore.svelte.js';
  import { dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { IconPlus, IconX, IconTerminal } from '@tabler/icons-svelte-runes';

  let terminalContainer = $state(null);
  let term = $state(null);
  let pty = $state(null);
  let isDropTarget = $state(false);
  let xtermLoaded = $state(false);
  let error = $state(null);

  // Track active PTYs per tab
  let ptyInstances = new Map();
  let termInstances = new Map();

  // Detect if running inside Tauri
  const isTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;

  let store = $derived($terminalStore);

  async function initTerminal(tabId) {
    if (!terminalContainer) return;

    // Clean up existing terminal in this container
    terminalContainer.innerHTML = '';

    try {
      // Dynamic imports for code splitting
      const { Terminal } = await import('@xterm/xterm');
      const { FitAddon } = await import('@xterm/addon-fit');

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

      newTerm.open(terminalContainer);

      // Small delay to ensure container is sized before fitting
      requestAnimationFrame(() => {
        try {
          fitAddon.fit();
        } catch {
          // Container might not be visible yet
        }
      });

      termInstances.set(tabId, { term: newTerm, fitAddon });
      term = newTerm;
      xtermLoaded = true;

      // Spawn PTY if running in Tauri
      if (isTauri) {
        try {
          const { spawn } = await import('tauri-pty');
          const shell = getDefaultShell();
          const newPty = spawn(shell, [], {
            cols: newTerm.cols,
            rows: newTerm.rows,
          });

          // Wire PTY I/O
          newPty.onData((data) => newTerm.write(data));
          newTerm.onData((data) => newPty.write(data));

          // Handle PTY exit
          newPty.onExit(({ exitCode }) => {
            newTerm.writeln(`\r\n[Process exited with code ${exitCode}]`);
          });

          ptyInstances.set(tabId, newPty);
          pty = newPty;
        } catch (err) {
          console.error('Failed to spawn PTY:', err);
          newTerm.writeln('Failed to spawn terminal process: ' + err.message);
          newTerm.writeln('Running in browser-only mode.');
        }
      } else {
        // Browser-only mode: show message
        newTerm.writeln('\x1b[1;34mWindshift Terminal\x1b[0m');
        newTerm.writeln('');
        newTerm.writeln('Terminal requires the Windshift Desktop app (Tauri).');
        newTerm.writeln('In browser mode, drag & drop preview is available.');
        newTerm.writeln('');
        // Echo typed input for preview purposes
        newTerm.onData((data) => {
          // Echo printable characters
          if (data === '\r') {
            newTerm.writeln('');
          } else if (data === '\x7f') {
            // Backspace
            newTerm.write('\b \b');
          } else {
            newTerm.write(data);
          }
        });
      }

      // Resize observer for the container
      const resizeObserver = new ResizeObserver(() => {
        try {
          fitAddon.fit();
          const currentPty = ptyInstances.get(tabId);
          if (currentPty && newTerm.cols && newTerm.rows) {
            currentPty.resize(newTerm.cols, newTerm.rows);
          }
        } catch {
          // Ignore resize errors during teardown
        }
      });
      resizeObserver.observe(terminalContainer);

      // Store cleanup ref
      termInstances.get(tabId).resizeObserver = resizeObserver;

    } catch (err) {
      error = err.message;
      console.error('Failed to initialize terminal:', err);
    }
  }

  function getDefaultShell() {
    const platform = navigator.platform?.toLowerCase() || '';
    if (platform.includes('win')) return 'powershell.exe';
    // Try to get user's default shell from env, fallback to zsh on macOS, bash otherwise
    if (platform.includes('mac')) return '/bin/zsh';
    return '/bin/bash';
  }

  function destroyTerminal(tabId) {
    const termEntry = termInstances.get(tabId);
    if (termEntry) {
      termEntry.resizeObserver?.disconnect();
      termEntry.term?.dispose();
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
  }

  function switchTab(tabId) {
    terminalStore.setActiveTab(tabId);
  }

  function addTab() {
    const newId = terminalStore.addTab();
    // Terminal will be initialized by the $effect below
  }

  function closeTab(tabId) {
    destroyTerminal(tabId);
    terminalStore.removeTab(tabId);
  }

  // Initialize terminal when container mounts or active tab changes
  $effect(() => {
    const activeId = store.activeTabId;
    if (terminalContainer && store.visible) {
      // Slight delay to ensure DOM is ready
      const timeout = setTimeout(() => initTerminal(activeId), 50);
      return () => clearTimeout(timeout);
    }
  });

  // Listen for terminal-write events (from drag & drop)
  onMount(() => {
    function handleTerminalWrite(event) {
      const { text } = event.detail;
      const activeId = store.activeTabId;
      const activePty = ptyInstances.get(activeId);
      if (activePty) {
        activePty.write(text);
      } else {
        // Browser mode: just write to xterm for preview
        const activeTermEntry = termInstances.get(activeId);
        if (activeTermEntry) {
          activeTermEntry.term.write(text);
        }
      }
    }

    window.addEventListener('terminal-write', handleTerminalWrite);

    return () => {
      window.removeEventListener('terminal-write', handleTerminalWrite);
      // Cleanup all terminals on unmount
      for (const tabId of termInstances.keys()) {
        destroyTerminal(tabId);
      }
    };
  });

  // Setup drop target
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
