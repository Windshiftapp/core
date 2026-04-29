<script>
	// Shared consent card used by both /cli/authorize and /oauth/authorize.
	// Renders a centered card with a header (icon + title + subtitle), a slot
	// for flow-specific content (identity panel, agent panel, etc.), the
	// scopes list, an inline error display, and the Allow/Deny button row.
	//
	// Both flows have the same visual language below the headline so users
	// see one consistent "an external thing wants to act on my behalf" UX
	// regardless of which transport is in use.

	import { Check } from 'lucide-svelte';
	import Button from './Button.svelte';

	let {
		icon = null,
		title = 'Authorize',
		// Plain-text subtitle. Rendered safely via Svelte's default escaping.
		// For richer markup (a bolded client/host name in the middle of a
		// sentence) callers should pass the `subtitleSnippet` snippet instead;
		// `subtitle` is still respected if no snippet is supplied.
		subtitle = '',
		subtitleSnippet = null,
		scopes = /** @type {string[]} */ ([]),
		scopeDescriptions = /** @type {Record<string,string>} */ ({}),
		error = '',
		onAllow = () => {},
		onDeny = () => {},
		loading = false,
		disabled = false,
		denyLabel = 'Deny',
		allowLabel = 'Allow',
		children = null,
	} = $props();

	function describeScope(scope) {
		return scopeDescriptions[scope] || scope;
	}
</script>

<div
	class="min-h-screen flex items-center justify-center px-4 py-10"
	style="background-color: var(--ds-surface);"
>
	<div
		class="w-full max-w-lg rounded-lg border p-6 shadow-sm"
		style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
	>
		<div class="flex items-center gap-3 mb-5">
			{#if icon}
				<div
					class="p-2 rounded-md"
					style="background-color: var(--ds-surface); color: var(--ds-text-subtle);"
				>
					<svelte:component this={icon} size={22} />
				</div>
			{/if}
			<div>
				<h1 class="text-lg font-semibold" style="color: var(--ds-text);">
					{title}
				</h1>
				{#if subtitleSnippet}
					<p class="text-sm" style="color: var(--ds-text-subtle);">
						{@render subtitleSnippet()}
					</p>
				{:else if subtitle}
					<p class="text-sm" style="color: var(--ds-text-subtle);">
						{subtitle}
					</p>
				{/if}
			</div>
		</div>

		<div class="space-y-4">
			{#if children}
				{@render children()}
			{/if}

			{#if scopes.length > 0}
				<div
					class="rounded-md border p-4"
					style="border-color: var(--ds-border); background-color: var(--ds-surface);"
				>
					<div class="text-xs uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">
						Permissions requested
					</div>
					<ul class="space-y-1.5">
						{#each scopes as scope}
							<li class="flex items-center gap-2 text-sm" style="color: var(--ds-text);">
								<Check size={14} style="color: var(--ds-text-subtle);" />
								<code class="text-xs" style="color: var(--ds-text-subtle);">{scope}</code>
								<span>— {describeScope(scope)}</span>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if error}
				<div
					class="text-sm rounded-md border p-3"
					style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
				>
					{error}
				</div>
			{/if}

			<div class="flex items-center justify-end gap-2 pt-2">
				<Button variant="default" onclick={onDeny} disabled={loading || disabled}>
					{denyLabel}
				</Button>
				<Button variant="primary" onclick={onAllow} disabled={loading || disabled}>
					{#if loading}Authorising…{:else}{allowLabel}{/if}
				</Button>
			</div>
		</div>
	</div>
</div>
