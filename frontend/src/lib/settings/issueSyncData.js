import { issueSync, workspaceSCM } from '../api/scm.js';
/** Load every linked repository in a workspace through one enriched list. */
export async function loadIssueSyncLinkedRepositories(workspaceId) {
  const connections = await workspaceSCM.getConnectionsOverview(workspaceId, {
    includeRepositories: true,
  });
  return (Array.isArray(connections) ? connections : []).flatMap((connection) =>
    Array.isArray(connection?.repositories) ? connection.repositories : []
  );
}

/** Load Issue Sync setup data concurrently while reusing workspace references. */
export async function loadIssueSyncPageData(referenceStore, workspaceId) {
  const configRequest = issueSync.getConfig(workspaceId).catch((error) => {
    if (error?.status === 404) return null;
    throw error;
  });
  const [config, linkedRepositories] = await Promise.all([
    configRequest,
    loadIssueSyncLinkedRepositories(workspaceId),
    referenceStore.initialize(workspaceId),
  ]);
  return {
    config,
    linkedRepositories,
    itemTypes: Array.isArray(referenceStore.itemTypes) ? referenceStore.itemTypes : [],
    priorities: Array.isArray(referenceStore.priorities) ? referenceStore.priorities : [],
    users: Array.isArray(referenceStore.users) ? referenceStore.users : [],
    milestones: Array.isArray(referenceStore.milestones) ? referenceStore.milestones : [],
  };
}
