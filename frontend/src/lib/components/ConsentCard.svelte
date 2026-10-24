<script>
	// Shared CLI/OAuth consent shell for flow content, scopes, errors, and choices.

	import Check from '@lucide/svelte/icons/check';
	import Button from './Button.svelte';

	let {
		icon: Icon = null,
		title = 'Authorize',
		// Plain text is escaped; use subtitleSnippet for rich markup.
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
	data-testid="consent-page"
>
	<div
		class="w-full max-w-lg rounded-lg border p-6 shadow-sm"
		style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
	>
		<div class="flex items-center gap-3 mb-5">
			{#if Icon}
				<div
					class="p-2 rounded-md"
					style="background-color: var(--ds-surface); color: var(--ds-text-subtle);"
				>
					<Icon size={22} />
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
							<li class="flex items-center gap-2 text-sm" style="color: var(--ds-text);" data-testid="consent-scope">
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
					data-testid="consent-error"
				>
					{error}
				</div>
			{/if}

			<div class="flex items-center justify-end gap-2 pt-2">
				<Button variant="default" dataTestid="consent-deny" onclick={onDeny} disabled={loading || disabled}>
					{denyLabel}
				</Button>
				<Button variant="primary" dataTestid="consent-allow" onclick={onAllow} disabled={loading || disabled}>
					{#if loading}Authorising…{:else}{allowLabel}{/if}
				</Button>
			</div>
		</div>
	</div>
</div>
