<script>
  import { Search } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  export let value = '';
  export let placeholder = '';
  export let disabled = false;
  export let className = '';
  export let size = 'medium'; // 'small', 'medium', 'large'
  export let hasGradient = false;
  export let on_input;
  export let on_keydown;

  function handleInput(event) {
    value = event.target.value;
    if (on_input) on_input(event);
  }

  function handleKeydown(event) {
    if (on_keydown) on_keydown(event);
  }

  const sizeClasses = {
    small: 'px-3 py-1.5 text-sm',
    medium: 'px-4 py-2 text-sm',
    large: 'px-4 py-3 text-base'
  };

  const iconSizeClasses = {
    small: 'w-3.5 h-3.5',
    medium: 'w-4 h-4',
    large: 'w-5 h-5'
  };

// Dynamic styles based on gradient and theme
  $: inputStyles = hasGradient 
    ? 'background-color: rgba(255, 255, 255, 0.98); backdrop-filter: blur(10px); border-color: rgba(255, 255, 255, 0.9); color: #111827;'
    : 'background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);';

  $: iconColorClass = hasGradient 
    ? 'text-gray-900' // Darkest color for maximum contrast against light gradient
    : 'text-gray-500'; // Standard color for normal input
</script>

<div class="relative {className}">
  <Search
    class="{iconSizeClasses[size]} absolute left-3 top-1/2 transform -translate-y-1/2 transition-colors z-10 {iconColorClass}"
    style={hasGradient ? 'color: #374151;' : ''}
  />
  <input
    type="text"
    bind:value
    placeholder={placeholder || t('common.search')}
    {disabled}
    class="pl-10 pr-4 {sizeClasses[size]} rounded border w-full transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed {hasGradient ? 'placeholder-gray-600' : ''}"
    style={inputStyles}
    oninput={handleInput}
    onkeydown={handleKeydown}
  />
</div>
