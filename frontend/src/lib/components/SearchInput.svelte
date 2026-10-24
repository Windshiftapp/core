<script>
  import Search from '@lucide/svelte/icons/search';
  import { t } from '../stores/i18n.svelte.js';
  import { controlSizeClasses } from '../utils/controlSizes.js';

  let {
    value = $bindable(''),
    placeholder = '',
    disabled = false,
    className = '',
    class: classProp = '',
    size = 'medium',
    dataTestid = undefined,
    on_input = undefined,
    on_keydown = undefined
  } = $props();
  const resolvedClass = $derived(classProp || className);

  function handleInput(event) {
    value = event.target.value;
    if (on_input) on_input(event);
  }

  function handleKeydown(event) {
    if (on_keydown) on_keydown(event);
  }

  const sizeClasses = $derived(controlSizeClasses(size));

  const iconSizeClasses = {
    small: 'w-3.5 h-3.5',
    medium: 'w-4 h-4',
    large: 'w-5 h-5'
  };
</script>

<div class="relative {resolvedClass}">
  <Search
    class="{iconSizeClasses[size]} absolute left-3 top-1/2 transform -translate-y-1/2 transition-colors z-10"
    style="color: var(--ds-text-subtle);"
  />
  <input
    type="text"
    bind:value
    placeholder={placeholder || t('common.search')}
    {disabled}
    data-testid={dataTestid}
    class="pl-10 pr-4 {sizeClasses} rounded border w-full transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-ds-border-focused focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
    style="background-color: var(--ctx-surface-raised, var(--ds-background-input)); border-color: var(--ctx-border, var(--ds-border)); color: var(--ds-text); backdrop-filter: var(--ctx-backdrop, none);"
    oninput={handleInput}
    onkeydown={handleKeydown}
  />
</div>
