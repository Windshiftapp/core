import { expect, type Page } from '@playwright/test';

/**
 * Page Object for Workspace Management
 */
export class WorkspacePage {
  constructor(private page: Page) {}

  // Selectors
  readonly workspacesLink = 'a:has-text("Workspaces")';
  readonly createButton = 'button:has-text("Add Workspace")';
  readonly workspaceModal = 'div[role="dialog"]';
  readonly nameInput = '#workspace-name';
  readonly keyInput = '#workspace-key';
  readonly descriptionInput = '#workspace-description';
  readonly saveButton = 'button:has-text("Create Workspace"), button:has-text("Save")';
  readonly cancelButton = 'button:has-text("Cancel")';
  readonly workspaceRow = 'tbody tr';
  readonly editButton = 'button:has-text("Edit")';
  readonly deleteButton = 'button:has-text("Delete")';
  readonly confirmDeleteButton = 'button:has-text("Confirm"), button:has-text("Delete")';
  readonly successToast =
    'text=created successfully, text=updated successfully, text=deleted successfully';
  readonly errorToast = '.error, .error-message, [role="alert"]';

  /**
   * Navigate to workspaces page
   */
  async goto() {
    await this.page.goto('/workspaces');
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Navigate via menu
   */
  async navigateViaMenu() {
    await this.page.click(this.workspacesLink);
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Click create workspace button
   */
  async clickCreate() {
    await this.page.click(this.createButton);
    await this.page.waitForSelector(this.workspaceModal, { timeout: 5000 });
  }

  /**
   * Fill workspace form
   */
  async fillForm(data: { name: string; key: string; description: string }) {
    await this.page.fill(this.nameInput, data.name);
    await this.page.fill(this.keyInput, data.key);
    await this.page.fill(this.descriptionInput, data.description);
  }

  /**
   * Click save button
   */
  async clickSave() {
    await this.page.click(this.saveButton);
    await this.page.waitForTimeout(1000);
  }

  /**
   * Create a new workspace
   */
  async createWorkspace(data: { name: string; key: string; description: string }) {
    await this.goto();
    await this.clickCreate();
    await this.fillForm(data);
    await this.clickSave();

    // Wait for modal to close
    await this.page.waitForSelector(this.workspaceModal, { state: 'hidden', timeout: 5000 });

    // Wait for the workspace to appear in the list
    await this.verifyWorkspaceExists(data.name);
  }

  /**
   * Find workspace by name
   */
  async findWorkspaceByName(name: string) {
    // Find table row containing the workspace name
    return this.page.locator(`${this.workspaceRow}:has-text("${name}")`).first();
  }

  /**
   * Verify workspace exists
   */
  async verifyWorkspaceExists(name: string) {
    const workspace = await this.findWorkspaceByName(name);
    await expect(workspace).toBeVisible({ timeout: 10000 });
  }

  /**
   * Click on a workspace to view details
   */
  async clickWorkspace(name: string) {
    const workspace = await this.findWorkspaceByName(name);
    await workspace.click();
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Edit a workspace
   */
  async editWorkspace(
    currentName: string,
    newData: {
      name?: string;
      key?: string;
      description?: string;
    }
  ) {
    await this.goto();

    // Find and click edit button for the workspace
    const workspace = await this.findWorkspaceByName(currentName);
    await workspace.locator(this.editButton).click();

    // Wait for modal
    await this.page.waitForSelector(this.workspaceModal, { timeout: 5000 });

    // Update fields
    if (newData.name) {
      await this.page.fill(this.nameInput, newData.name);
    }
    if (newData.key) {
      await this.page.fill(this.keyInput, newData.key);
    }
    if (newData.description) {
      await this.page.fill(this.descriptionInput, newData.description);
    }

    await this.clickSave();

    // Wait for modal to close
    await this.page.waitForSelector(this.workspaceModal, { state: 'hidden', timeout: 5000 });

    // If name was updated, verify new name exists
    if (newData.name) {
      await this.verifyWorkspaceExists(newData.name);
    }
  }

  /**
   * Delete a workspace
   */
  async deleteWorkspace(name: string) {
    await this.goto();

    // Find and click delete button for the workspace
    const workspace = await this.findWorkspaceByName(name);
    await workspace.locator(this.deleteButton).click();

    // Confirm deletion
    await this.page.waitForSelector(this.confirmDeleteButton, { timeout: 5000 });
    await this.page.click(this.confirmDeleteButton);

    // Wait for deletion to complete
    await this.page.waitForTimeout(2000);
  }

  /**
   * Verify workspace does not exist
   */
  async verifyWorkspaceDoesNotExist(name: string) {
    const workspace = this.page.locator(`${this.workspaceRow}:has-text("${name}")`);
    await expect(workspace).not.toBeVisible({ timeout: 5000 });
  }

  /**
   * Get workspace count
   */
  async getWorkspaceCount(): Promise<number> {
    const workspaces = await this.page.locator(this.workspaceRow).count();
    return workspaces;
  }

  /**
   * Search for workspace
   */
  async searchWorkspace(query: string) {
    const searchInput = this.page.locator('input[type="search"], input[placeholder*="Search"]');
    await searchInput.fill(query);
    await this.page.waitForTimeout(500);
  }
}
