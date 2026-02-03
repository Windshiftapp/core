import { expect, type Page } from '@playwright/test';

/**
 * Page Object for Work Item Management
 */
export class ItemPage {
  constructor(private page: Page) {}

  // Selectors
  readonly createItemButton =
    'button:has-text("Create"), button:has-text("New Item"), button:has-text("Add Item"), [data-testid="create-item"]';
  readonly itemModal = 'div[role="dialog"], .modal, .item-modal';
  readonly titleInput = 'input[name="title"], input[placeholder*="title"]';
  readonly descriptionInput =
    'textarea[name="description"], textarea[placeholder*="description"], .ProseMirror';
  readonly statusSelect = 'select[name="status"], [name="status"]';
  readonly prioritySelect = 'select[name="priority"], [name="priority"]';
  readonly assigneeSelect = 'select[name="assignee"], [name="assignee"]';
  readonly parentSelect = 'select[name="parent"], [name="parent_id"]';
  readonly saveButton = 'button[type="submit"], button:has-text("Save"), button:has-text("Create")';
  readonly cancelButton = 'button:has-text("Cancel"), button:has-text("Close")';
  readonly itemRow = '.item-row, tr, [data-testid="item-row"]';
  readonly itemCard = '.item-card, [data-testid="item-card"]';
  readonly itemKey = '.item-key, [data-testid="item-key"]';
  readonly editButton = 'button:has-text("Edit"), [aria-label="Edit"]';
  readonly deleteButton = 'button:has-text("Delete"), [aria-label="Delete"]';
  readonly confirmDeleteButton = 'button:has-text("Confirm"), button:has-text("Delete")';
  readonly backlogLink = 'a:has-text("Backlog"), nav a[href*="/backlog"]';

  /**
   * Navigate to workspace backlog
   */
  async gotoWorkspaceBacklog(workspaceKey: string) {
    await this.page.goto(`/workspaces/${workspaceKey}/backlog`);
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Click create item button
   */
  async clickCreate() {
    await this.page.click(this.createItemButton);
    await this.page.waitForSelector(this.itemModal, { timeout: 5000 });
  }

  /**
   * Fill item form
   */
  async fillForm(data: {
    title: string;
    description?: string;
    status?: string;
    priority?: string;
    parent?: string;
  }) {
    await this.page.fill(this.titleInput, data.title);

    if (data.description) {
      // Try different description input types
      const descInput = this.page.locator(this.descriptionInput).first();
      await descInput.click();
      await descInput.fill(data.description);
    }

    if (data.status) {
      await this.page.selectOption(this.statusSelect, data.status);
    }

    if (data.priority) {
      await this.page.selectOption(this.prioritySelect, data.priority);
    }

    if (data.parent) {
      await this.page.selectOption(this.parentSelect, data.parent);
    }
  }

  /**
   * Click save button
   */
  async clickSave() {
    await this.page.click(this.saveButton);
    await this.page.waitForTimeout(1000);
  }

  /**
   * Create a new item
   */
  async createItem(
    workspaceKey: string,
    data: {
      title: string;
      description?: string;
      status?: string;
      priority?: string;
      parent?: string;
    }
  ) {
    await this.gotoWorkspaceBacklog(workspaceKey);
    await this.clickCreate();
    await this.fillForm(data);
    await this.clickSave();

    // Wait for creation
    await this.page.waitForTimeout(2000);
  }

  /**
   * Find item by title
   */
  async findItemByTitle(title: string) {
    return this.page
      .locator(`${this.itemRow}:has-text("${title}"), ${this.itemCard}:has-text("${title}")`)
      .first();
  }

  /**
   * Find item by key (e.g., "E2E-123")
   */
  async findItemByKey(key: string) {
    return this.page.locator(`${this.itemKey}:has-text("${key}")`).first();
  }

  /**
   * Verify item exists
   */
  async verifyItemExists(title: string) {
    const item = await this.findItemByTitle(title);
    await expect(item).toBeVisible({ timeout: 10000 });
  }

  /**
   * Click on an item to view details
   */
  async clickItem(title: string) {
    const item = await this.findItemByTitle(title);
    await item.click();
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Edit an item
   */
  async editItem(
    currentTitle: string,
    newData: {
      title?: string;
      description?: string;
      status?: string;
      priority?: string;
    }
  ) {
    // Find and click the item
    await this.clickItem(currentTitle);

    // Click edit button on the item detail page
    await this.page.click(this.editButton);

    // Wait for form/modal
    await this.page.waitForTimeout(500);

    // Update fields
    if (newData.title) {
      await this.page.fill(this.titleInput, newData.title);
    }
    if (newData.description) {
      const descInput = this.page.locator(this.descriptionInput).first();
      await descInput.fill(newData.description);
    }
    if (newData.status) {
      await this.page.selectOption(this.statusSelect, newData.status);
    }
    if (newData.priority) {
      await this.page.selectOption(this.prioritySelect, newData.priority);
    }

    await this.clickSave();

    // Wait for update
    await this.page.waitForTimeout(2000);
  }

  /**
   * Delete an item
   */
  async deleteItem(title: string) {
    const item = await this.findItemByTitle(title);
    await item.locator(this.deleteButton).click();

    // Confirm deletion
    await this.page.waitForSelector(this.confirmDeleteButton, { timeout: 5000 });
    await this.page.click(this.confirmDeleteButton);

    // Wait for deletion
    await this.page.waitForTimeout(2000);
  }

  /**
   * Verify item does not exist
   */
  async verifyItemDoesNotExist(title: string) {
    const item = this.page.locator(
      `${this.itemRow}:has-text("${title}"), ${this.itemCard}:has-text("${title}")`
    );
    await expect(item).not.toBeVisible({ timeout: 5000 });
  }

  /**
   * Get item count
   */
  async getItemCount(): Promise<number> {
    const items = await this.page.locator(`${this.itemRow}, ${this.itemCard}`).count();
    return items;
  }

  /**
   * Update item status via inline editing
   */
  async updateStatus(title: string, newStatus: string) {
    const item = await this.findItemByTitle(title);
    const statusCell = item.locator('[data-field="status"], .status');
    await statusCell.click();

    // Select new status
    await this.page.selectOption('select, [role="combobox"]', newStatus);

    // Wait for update
    await this.page.waitForTimeout(1000);
  }

  /**
   * Create child item
   */
  async createChildItem(
    parentTitle: string,
    childData: {
      title: string;
      description?: string;
      status?: string;
      priority?: string;
    }
  ) {
    // Navigate to parent item
    await this.clickItem(parentTitle);

    // Click create child button
    await this.page.click('button:has-text("Create Child"), button:has-text("Add Child")');

    // Fill form
    await this.fillForm(childData);
    await this.clickSave();

    // Wait for creation
    await this.page.waitForTimeout(2000);
  }

  /**
   * Verify item hierarchy
   */
  async verifyItemIsChildOf(childTitle: string, parentTitle: string) {
    const child = await this.findItemByTitle(childTitle);
    const parentInfo = child.locator('.parent, [data-field="parent"]');
    await expect(parentInfo).toContainText(parentTitle);
  }
}
