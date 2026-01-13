<script>
  import { AtSign, User } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">MentionPicker</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    A specialized autocomplete picker for @mentions in text editors. Appears at cursor position when typing "@" and filters users as you type.
  </p>

  <!-- API Dependency Notice -->
  <div
    class="mb-8 p-4 rounded-lg flex items-start gap-3"
    style="background-color: var(--ds-background-warning); border: 1px solid var(--ds-border-warning);"
  >
    <span style="color: var(--ds-icon-warning);">⚠️</span>
    <div>
      <p class="font-medium" style="color: var(--ds-text-warning);">API-Dependent Component</p>
      <p class="text-sm" style="color: var(--ds-text-warning);">
        MentionPicker fetches users from the backend API. Live demos require a running server with user data.
      </p>
    </div>
  </div>

  <!-- Visual Example -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Visual Example</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      The picker appears as a floating menu when triggered by "@" in a text editor.
    </p>
    <div
      class="relative inline-block p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <!-- Mock editor -->
      <div class="mb-4 p-3 rounded border" style="background-color: var(--ds-background-input); border-color: var(--ds-border);">
        <p style="color: var(--ds-text);">Hey <span style="color: var(--ds-link);">@</span><span style="background: var(--ds-background-selected); color: var(--ds-text-selected);" class="px-1 rounded">al</span></p>
      </div>

      <!-- Mock mention picker -->
      <div
        class="rounded-lg shadow-lg overflow-hidden"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border); width: 280px;"
      >
        {#each [
          { name: 'Alice Johnson', username: 'alice', initials: 'AJ' },
          { name: 'Alan Smith', username: 'alan', initials: 'AS' },
          { name: 'Alex Brown', username: 'alexb', initials: 'AB' },
        ] as user, i}
          <button
            class="w-full flex items-center gap-3 px-3 py-2 text-left transition-colors"
            style="background-color: {i === 0 ? 'var(--ds-background-neutral-hovered)' : 'transparent'};"
          >
            <div class="w-8 h-8 rounded-full flex items-center justify-center text-xs font-semibold" style="background: var(--ds-interactive); color: var(--ds-text-inverse);">
              {user.initials}
            </div>
            <div>
              <div class="font-medium text-sm" style="color: var(--ds-text);">{user.name}</div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">@{user.username}</div>
            </div>
          </button>
        {/each}
      </div>
    </div>
  </section>

  <!-- Features -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Features</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <ul class="text-sm space-y-2" style="color: var(--ds-text-subtle);">
        <li class="flex items-start gap-2">
          <span style="color: var(--ds-text);">•</span>
          <span><strong>Positioned at cursor:</strong> Appears at the text cursor position when typing "@"</span>
        </li>
        <li class="flex items-start gap-2">
          <span style="color: var(--ds-text);">•</span>
          <span><strong>Real-time filtering:</strong> Filters users as you type the name after "@"</span>
        </li>
        <li class="flex items-start gap-2">
          <span style="color: var(--ds-text);">•</span>
          <span><strong>Keyboard navigation:</strong> Arrow keys to navigate, Enter/Tab to select, Escape to cancel</span>
        </li>
        <li class="flex items-start gap-2">
          <span style="color: var(--ds-text);">•</span>
          <span><strong>Avatar display:</strong> Shows user avatar or initials</span>
        </li>
        <li class="flex items-start gap-2">
          <span style="color: var(--ds-text);">•</span>
          <span><strong>Personal workspace warning:</strong> Shows notice when mentions won't send notifications</span>
        </li>
        <li class="flex items-start gap-2">
          <span style="color: var(--ds-text);">•</span>
          <span><strong>Searchable fields:</strong> Searches first name, last name, username, and email</span>
        </li>
      </ul>
    </div>
  </section>

  <!-- Usage Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Usage Context</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      MentionPicker is used within the Milkdown rich text editor to enable @mentions. It's typically controlled by the editor's mention plugin.
    </p>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <pre class="text-sm overflow-x-auto" style="color: var(--ds-text);">{`<script>
  import MentionPicker from '$lib/pickers/MentionPicker.svelte'

  let mentionQuery = ''
  let mentionPosition = { x: 0, y: 0 }
  let showMentionPicker = false

  function handleMentionSelect(event) {
    const user = event.detail
    // Insert mention into editor
    insertMention(user)
    showMentionPicker = false
  }
</script>

<MentionPicker
  query={mentionQuery}
  position={mentionPosition}
  open={showMentionPicker}
  isPersonalWorkspace={false}
  on:select={handleMentionSelect}
  on:cancel={() => showMentionPicker = false}
/>`}</pre>
    </div>
  </section>

  <!-- Personal Workspace Warning -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Personal Workspace Warning</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      When <code>isPersonalWorkspace</code> is true, a warning banner appears at the bottom.
    </p>
    <div
      class="rounded-lg shadow-lg overflow-hidden"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border); width: 280px;"
    >
      <button
        class="w-full flex items-center gap-3 px-3 py-2 text-left"
        style="background-color: var(--ds-background-neutral-hovered);"
      >
        <div class="w-8 h-8 rounded-full flex items-center justify-center text-xs font-semibold" style="background: var(--ds-interactive); color: var(--ds-text-inverse);">
          AJ
        </div>
        <div>
          <div class="font-medium text-sm" style="color: var(--ds-text);">Alice Johnson</div>
          <div class="text-xs" style="color: var(--ds-text-subtle);">@alice</div>
        </div>
      </button>
      <div
        class="px-3 py-2 text-xs"
        style="background-color: var(--ds-background-warning); border-top: 1px solid var(--ds-border-warning); color: var(--ds-text-warning);"
      >
        No notification will be sent (personal task)
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
            <td class="p-2"><code>query</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>position</code></td>
            <td class="p-2">{`{ x: number, y: number }`}</td>
            <td class="p-2">{`{ x: 0, y: 0 }`}</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>open</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>isPersonalWorkspace</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Events Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Events</h2>
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
            <td class="p-2"><code>on:select</code></td>
            <td class="p-2">Fired when a user is selected. Contains the full user object.</td>
          </tr>
          <tr>
            <td class="p-2"><code>on:cancel</code></td>
            <td class="p-2">Fired when the picker is dismissed (Escape key).</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Keyboard Shortcuts -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Keyboard Shortcuts</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Key</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Action</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>↑</code> / <code>↓</code></td>
            <td class="p-2">Navigate through user list</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>Enter</code> / <code>Tab</code></td>
            <td class="p-2">Select highlighted user</td>
          </tr>
          <tr>
            <td class="p-2"><code>Escape</code></td>
            <td class="p-2">Cancel and close picker</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
