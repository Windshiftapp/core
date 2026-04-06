const STORAGE_PREFIX = 'windshift-workspace-path-';

class WorkspacePathStore {
  workspaceId = $state(null);
  path = $state(null);
  wsTomlStatus = $state('unknown'); // 'unknown' | 'checking' | 'found' | 'missing'

  setWorkspace(id) {
    if (this.workspaceId === id) return;
    this.workspaceId = id;
    this.wsTomlStatus = 'unknown';

    if (!id) {
      this.path = null;
      return;
    }

    // Load saved path from localStorage
    try {
      const saved = localStorage.getItem(STORAGE_PREFIX + id);
      this.path = saved || null;
    } catch {
      this.path = null;
    }
  }

  setPath(id, folderPath) {
    try {
      localStorage.setItem(STORAGE_PREFIX + id, folderPath);
    } catch {
      // ignore
    }
    if (id === this.workspaceId) {
      this.path = folderPath;
    }
  }

  clearPath(id) {
    try {
      localStorage.removeItem(STORAGE_PREFIX + id);
    } catch {
      // ignore
    }
    if (id === this.workspaceId) {
      this.path = null;
      this.wsTomlStatus = 'unknown';
    }
  }
}

export const workspacePathStore = new WorkspacePathStore();
