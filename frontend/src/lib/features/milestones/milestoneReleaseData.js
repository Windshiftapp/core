import { workspaceSCM } from '../../api/scm.js';
/** Load release-target SCM connections with one request for either scope. */
export async function loadMilestoneReleaseConnections(workspaceId) {
  if (workspaceId) {
    const connections = (await workspaceSCM.getConnections(workspaceId)) ?? [];
    return connections.map((connection) => ({
      ...connection,
      _workspaceName: null,
      _workspaceId: workspaceId,
    }));
  }

  const connections = (await workspaceSCM.getAccessibleConnections()) ?? [];
  return connections.map((connection) => ({
    ...connection,
    _workspaceName: connection.workspace_name ?? null,
    _workspaceId: connection.workspace_id,
  }));
}
