import { untrack } from 'svelte';
import { hydrateCurrentWorkspaceFromSharedData } from '../services/authenticatedShellUI.js';
import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
import {
  clearWorkspaceGradient,
  loadWorkspaceGradient,
} from '../stores/workspaceGradient.svelte.js';
import { currentWorkspace, workspacesStore } from '../stores/workspaces.svelte.js';
import { resolveMainAppWorkspaceContext } from './mainAppWorkspaceContext.js';

export function useMainAppWorkspaceContext(getRoute, getPersonalWorkspaceId) {
  const context = $derived(resolveMainAppWorkspaceContext(getRoute(), getPersonalWorkspaceId()));
  const contextKey = $derived(
    context.kind === 'workspace' ? `workspace:${Number(context.workspaceId)}` : context.kind
  );

  // View/query changes keep the same context. Clear a different scope before paint.
  $effect.pre(() => {
    void contextKey;
    untrack(() => {
      clearWorkspaceGradient();
      if (context.kind === 'workspace') {
        void hydrateCurrentWorkspaceFromSharedData(context.workspaceId);
        void loadWorkspaceGradient(context.workspaceId);
      } else if (context.kind === 'personal-pending') {
        void workspacesStore.loadPersonalWorkspace();
      } else if (context.kind === 'global-collection') {
        currentWorkspace.clear();
        void workspaceDataStore.initializeGlobal();
      } else {
        currentWorkspace.clear();
        workspaceDataStore.reset();
      }
    });
    return () => clearWorkspaceGradient();
  });
}
