<script>
	import { onMount, onDestroy } from 'svelte';
	import { createPluginBridge, MESSAGE_TYPES } from './PluginBridge.js';
	import Spinner from '../components/Spinner.svelte';
	import UserPicker from '../pickers/UserPicker.svelte';
	import Modal from '../dialogs/Modal.svelte';
	import ModalHeader from '../dialogs/ModalHeader.svelte';
	import { api } from '../api.js';
	import { themeStore } from '../stores/theme.svelte.js';

	let { pluginName = '', src = '' } = $props();

	let iframe = $state(null);
	let bridge = $state(null);
	let isReady = $state(false);
	let iframeHeight = $state(600); // Default height

	// User picker state
	let showUserPicker = $state(false);
	let userPickerValue = $state(null);

	function handleUserPickerOpen(currentUserId) {
		userPickerValue = currentUserId;
		showUserPicker = true;
	}

	async function handleUserSelected(userId) {
		showUserPicker = false;
		if (userId && bridge) {
			try {
				// Fetch user details to send back to plugin
				const user = await api.get(`/users/${userId}`);
				bridge.sendToPlugin({
					type: MESSAGE_TYPES.USER_PICKER_RESULT,
					user
				});
			} catch (error) {
				console.error('[IframePluginLoader] Error fetching user:', error);
			}
		}
	}

	onMount(() => {
		if (!iframe) return;

		// Create bridge for this iframe
		bridge = createPluginBridge(iframe, {
			pluginName,
			onReady: () => {
				isReady = true;
				console.log(`[IframePluginLoader] Plugin ${pluginName} is ready`);
			},
			onResize: (height) => {
				// Add some padding to avoid scrollbars
				iframeHeight = Math.max(height + 20, 400);
			},
			onShowUserPicker: handleUserPickerOpen
		});
	});

	// Watch for theme changes and notify plugin
	$effect(() => {
		themeStore.resolvedTheme; // Track dependency
		if (bridge && isReady) {
			bridge.sendThemeUpdate();
		}
	});

	onDestroy(() => {
		if (bridge) {
			bridge.destroy();
		}
	});
</script>

<div class="iframe-plugin-container">
	{#if !isReady}
		<div class="loading-state">
			<Spinner class="mx-auto mb-4" />
			<p style="color: var(--ds-text-subtle);">Loading {pluginName}...</p>
		</div>
	{/if}

	<iframe
		bind:this={iframe}
		{src}
		title={`${pluginName} plugin`}
		style="height: {iframeHeight}px; opacity: {isReady ? 1 : 0};"
		class="plugin-iframe"
	></iframe>
</div>

<!-- User Picker Modal -->
<Modal bind:isOpen={showUserPicker} maxWidth="max-w-md">
	<ModalHeader title="Select User" onclose={() => showUserPicker = false} />
	<div class="p-4">
		<UserPicker
			bind:value={userPickerValue}
			placeholder="Search users..."
			autoOpen={true}
			onSelect={(user) => handleUserSelected(user?.id)}
			onCancel={() => showUserPicker = false}
		/>
	</div>
</Modal>

<style>
	.iframe-plugin-container {
		position: relative;
		width: 100%;
		min-height: 400px;
	}

	.loading-state {
		position: absolute;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		text-align: center;
	}

	.plugin-iframe {
		width: 100%;
		border: none;
		display: block;
		background: transparent;
		transition: opacity 0.2s ease;
	}
</style>
