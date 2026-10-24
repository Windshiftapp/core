import { workspaceSCM } from '../api/scm.js';
/** Load providers, connections and all per-connection auth summaries in two requests. */
export async function loadWorkspaceSCMOverview(workspaceId) {
  const [availableProviders, connections] = await Promise.all([
    workspaceSCM.getAvailableProviders(workspaceId),
    workspaceSCM.getConnectionsOverview(workspaceId, { includeAuthStatus: true }),
  ]);
  const normalizedConnections = Array.isArray(connections) ? connections : [];
  return {
    availableProviders: Array.isArray(availableProviders) ? availableProviders : [],
    connections: normalizedConnections,
    authStatuses: Object.fromEntries(
      normalizedConnections
        .filter((connection) => connection?.auth_status)
        .map((connection) => [connection.id, connection.auth_status])
    ),
  };
}
