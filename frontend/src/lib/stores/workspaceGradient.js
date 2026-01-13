import { writable } from 'svelte/store';
import { api } from '../api.js';
import { gradients } from '../utils/gradients.js';

// Store for workspace gradient settings
export const workspaceGradientIndex = writable(0); // Default to 0 (None)
export const applyToAllViews = writable(false);

// Load gradient settings from workspace homepage layout
export async function loadWorkspaceGradient(workspaceId) {
    try {
        const layout = await api.workspaces.getHomepageLayout(workspaceId);
        // Default to 0 (None) if not set
        workspaceGradientIndex.set(layout?.gradient ?? 0);
        applyToAllViews.set(layout?.applyToAllViews ?? false);
    } catch (error) {
        console.error('Failed to load workspace gradient:', error);
        // Default to no gradient
        workspaceGradientIndex.set(0);
        applyToAllViews.set(false);
    }
}

// Get gradient CSS value from index
export function getGradientStyle(index) {
    // Index 0 is "None", no gradient
    if (index === 0 || !gradients[index]) {
        return null;
    }
    return gradients[index].value;
}
