<script>
	import { Check, Copy } from '@lucide/svelte';
	import { cn } from '../utils/cn.js';
	import { copyToClipboard } from '../utils/clipboard.js';

	let {
		/** Static text to copy. Ignored if `getText` is provided. */
		text = '',
		/** Lazy getter used when the value isn't known until click time. */
		getText = null,
		size = 'md', // 'sm' | 'md' | 'lg'
		title = 'Copy',
		/** Optional label rendered next to the icon. */
		label = null,
		/** Optional label shown while in the copied state. Falls back to `label`. */
		copiedLabel = null,
		/** Duration (ms) to show the "copied" state before reverting. */
		resetAfterMs = 2000,
		/** Invoked after a successful copy. */
		onCopy = null,
		/** Invoked on clipboard failure. */
		onError = null,
		disabled = false,
		class: className = ''
	} = $props();

	let copied = $state(false);
	let resetTimer;

	const hasLabel = $derived(Boolean(label || copiedLabel));
	const iconSize = $derived(
		{ sm: 'w-3.5 h-3.5', md: 'w-4 h-4', lg: 'w-5 h-5' }[size] ?? 'w-4 h-4'
	);
	const paddingClass = $derived(
		hasLabel
			? { sm: 'px-2.5 py-1', md: 'px-3 py-1.5', lg: 'px-3.5 py-2' }[size] ?? 'px-3 py-1.5'
			: { sm: 'p-1', md: 'p-2', lg: 'p-2.5' }[size] ?? 'p-2'
	);

	async function handleClick() {
		const value = getText ? getText() : text;
		const ok = await copyToClipboard(value);
		if (ok) {
			copied = true;
			clearTimeout(resetTimer);
			resetTimer = setTimeout(() => (copied = false), resetAfterMs);
			onCopy?.(value);
		} else {
			onError?.();
		}
	}
</script>

<button
	type="button"
	class={cn(
		'inline-flex items-center justify-center rounded transition-colors',
		'hover:bg-[var(--ds-background-neutral-hovered,#f3f4f6)]',
		'disabled:opacity-50 disabled:cursor-not-allowed',
		paddingClass,
		className
	)}
	style="color: var(--ds-text-subtle)"
	{title}
	{disabled}
	aria-label={title}
	onclick={handleClick}
>
	{#if copied}
		<Check class="{iconSize} text-green-600" aria-hidden="true" />
	{:else}
		<Copy class={iconSize} aria-hidden="true" />
	{/if}
	{#if label || copiedLabel}
		<span class="ml-1 text-xs">{copied ? (copiedLabel ?? label) : label}</span>
	{/if}
</button>
