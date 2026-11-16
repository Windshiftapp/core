import { backlogStore } from './backlogStore.svelte.js';
import { collectionStore } from './collectionContext.svelte.js';
import { collectionEditorOptions } from './collectionEditorOptions.svelte.js';
import { collectionFieldLinks } from './collectionFieldLinks.svelte.js';
import { itemDetailStore } from './itemDetailStore.svelte.js';
import { itemTestCaseLinksStore } from './itemTestCaseLinksStore.svelte.js';
import { milestonesStore } from './milestones.js';
import { referenceDisplayCache } from './referenceDisplayCache.svelte.js';
import { viewSettingsStore } from './viewSettings.svelte.js';
import { workItemFormStore } from './workItemFormStore.svelte.js';
import { workspaceCollectionCatalog } from './workspaceCollectionCatalog.svelte.js';
import { workspaceDataStore } from './workspaceDataStore.svelte.js';
import { clearWorkspaceGradient } from './workspaceGradient.svelte.js';
import { currentWorkspace } from './workspaces.svelte.js';

/** Reset read projections only; session state and captured writes outlive navigation. */
export function resetScopedNavigation() {
  collectionStore.reset();
  workspaceCollectionCatalog.reset();
  backlogStore.reset();
  milestonesStore.reset();
  workspaceDataStore.reset();
  currentWorkspace.clear();
  clearWorkspaceGradient();
  itemDetailStore.reset();
  workItemFormStore.reset();
  collectionEditorOptions.reset();
  collectionFieldLinks.reset();
  itemTestCaseLinksStore.reset();
  referenceDisplayCache.reset();
  viewSettingsStore.reset();
}
