<script>
  import AlertBox from '../../../lib/AlertBox.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">AlertBox</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Alert component for displaying error, warning, info, and success messages.
    Uses a card-style design with a colored left border to indicate message type.
  </p>

  <!-- Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <div class="max-w-xl space-y-4">
      <AlertBox variant="error" message="This is an error message. Something went wrong." />
      <AlertBox variant="warning" message="This is a warning message. Please be careful." />
      <AlertBox variant="info" message="This is an info message. Here's some useful information." />
      <AlertBox variant="success" message="This is a success message. Everything worked!" />
    </div>
  </section>

  <!-- Without Icons -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Without Icons</h2>
    <div class="max-w-xl space-y-4">
      <AlertBox variant="error" message="Error without icon" showIcon={false} />
      <AlertBox variant="warning" message="Warning without icon" showIcon={false} />
      <AlertBox variant="info" message="Info without icon" showIcon={false} />
      <AlertBox variant="success" message="Success without icon" showIcon={false} />
    </div>
  </section>

  <!-- Long Messages -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Long Messages</h2>
    <div class="max-w-xl space-y-4">
      <AlertBox
        variant="warning"
        message="This is a longer warning message that spans multiple lines. It demonstrates how the AlertBox component handles larger amounts of text while maintaining proper alignment with the icon."
      />
      <AlertBox
        variant="info"
        message="Here's another long message with important information that you should read carefully. The component is designed to handle text of varying lengths gracefully."
      />
    </div>
  </section>

  <!-- Common Use Cases -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Use Cases</h2>
    <div class="max-w-xl space-y-4">
      <AlertBox variant="error" message="Invalid email address. Please enter a valid email." />
      <AlertBox variant="warning" message="Your session will expire in 5 minutes. Please save your work." />
      <AlertBox variant="info" message="Tip: You can use keyboard shortcuts to navigate faster." />
      <AlertBox variant="success" message="Changes saved successfully. Your settings have been updated." />
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
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'error' | 'warning' | 'info' | 'success'</td>
            <td class="p-2">'error'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>message</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showIcon</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
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

  <!-- Design Pattern -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Design Pattern</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        AlertBox uses a card-style design with a <strong>4px colored left border</strong> to indicate message type,
        while maintaining a neutral background and text color. This provides clear visual hierarchy
        without overwhelming the user with fully-colored backgrounds.
      </p>
      <ul class="text-sm mt-3 space-y-1" style="color: var(--ds-text-subtle);">
        <li>• <span style="color: var(--ds-icon-danger);">Red</span> left border for errors</li>
        <li>• <span style="color: var(--ds-icon-warning);">Amber</span> left border for warnings</li>
        <li>• <span style="color: var(--ds-icon-information);">Blue</span> left border for info</li>
        <li>• <span style="color: var(--ds-icon-success);">Green</span> left border for success</li>
      </ul>
    </div>
  </section>
</div>
