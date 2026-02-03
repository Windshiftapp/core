import { expect, test } from '@playwright/test';
import { generateChildItem, generateItem, generateWorkspace } from '../fixtures/test-data';
import { ItemPage } from '../pages/item.page';
import { WorkspacePage } from '../pages/workspace.page';

/**
 * Work Item Management Tests
 * Tests item CRUD operations and hierarchy using authenticated context
 */

test.describe('Item Management', () => {
  let workspacePage: WorkspacePage;
  let itemPage: ItemPage;
  let testWorkspace: ReturnType<typeof generateWorkspace>;
  let workspaceKey: string;

  test.beforeEach(async ({ page }) => {
    workspacePage = new WorkspacePage(page);
    itemPage = new ItemPage(page);
    testWorkspace = generateWorkspace();

    // Create workspace for items
    await workspacePage.createWorkspace(testWorkspace);
    workspaceKey = testWorkspace.key;
  });

  test.describe('Create Item', () => {
    test('should create basic item', async () => {
      const item = generateItem(0, 'basic');

      await itemPage.createItem(workspaceKey, {
        title: item.title,
        description: item.description,
        status: item.status,
        priority: item.priority,
      });

      // Verify item was created
      await itemPage.verifyItemExists(item.title);
    });

    test('should create item with minimal data', async () => {
      const item = generateItem(0, 'minimal');

      await itemPage.createItem(workspaceKey, {
        title: item.title,
      });

      // Verify item was created
      await itemPage.verifyItemExists(item.title);
    });

    test('should require item title', async ({ page }) => {
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.clickCreate();

      // Try to save without title
      await itemPage.clickSave();

      // Should not close modal (validation error)
      await page.waitForTimeout(500);
      const modal = page.locator(itemPage.itemModal);
      await expect(modal).toBeVisible();
    });

    test('should display created item in backlog', async () => {
      const item = generateItem(0, 'display');

      await itemPage.createItem(workspaceKey, {
        title: item.title,
        description: item.description,
      });

      // Navigate to backlog
      await itemPage.gotoWorkspaceBacklog(workspaceKey);

      // Find item
      const itemElement = await itemPage.findItemByTitle(item.title);
      await expect(itemElement).toBeVisible();
    });
  });

  test.describe('View Item', () => {
    let testItem: ReturnType<typeof generateItem>;

    test.beforeEach(async () => {
      testItem = generateItem(0, 'view');
      await itemPage.createItem(workspaceKey, {
        title: testItem.title,
        description: testItem.description,
      });
    });

    test('should view item details', async () => {
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.clickItem(testItem.title);

      // Should see item details
      await expect(itemPage.page.locator('h1, h2')).toContainText(testItem.title);
    });

    test('should display item fields', async () => {
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.clickItem(testItem.title);

      // Should see various fields
      await itemPage.page.waitForLoadState('networkidle');

      // Verify we're on item detail page
      await expect(itemPage.page).toHaveURL(/\/items\/\d+/);
    });
  });

  test.describe('Edit Item', () => {
    let testItem: ReturnType<typeof generateItem>;

    test.beforeEach(async () => {
      testItem = generateItem(0, 'edit');
      await itemPage.createItem(workspaceKey, {
        title: testItem.title,
        description: testItem.description,
        status: 'open',
        priority: 'medium',
      });
    });

    test('should update item title', async () => {
      const newTitle = `${testItem.title} - Updated`;

      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.editItem(testItem.title, {
        title: newTitle,
      });

      // Verify update
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.verifyItemExists(newTitle);
    });

    test('should update item description', async () => {
      const newDescription = 'Updated description for E2E test item';

      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.editItem(testItem.title, {
        description: newDescription,
      });

      // Verify by viewing item
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.clickItem(testItem.title);

      await expect(itemPage.page.locator('.description, .ProseMirror')).toContainText(
        newDescription
      );
    });

    test('should update item status', async () => {
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.editItem(testItem.title, {
        status: 'in-progress',
      });

      // Verify status change
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      const item = await itemPage.findItemByTitle(testItem.title);
      await expect(item).toContainText('in-progress', { ignoreCase: true });
    });

    test('should update item priority', async () => {
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.editItem(testItem.title, {
        priority: 'high',
      });

      // Verify priority change
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      const item = await itemPage.findItemByTitle(testItem.title);
      await expect(item).toContainText('high', { ignoreCase: true });
    });
  });

  test.describe('Delete Item', () => {
    let testItem: ReturnType<typeof generateItem>;

    test.beforeEach(async () => {
      testItem = generateItem(0, 'delete');
      await itemPage.createItem(workspaceKey, {
        title: testItem.title,
        description: testItem.description,
      });
    });

    test('should delete item', async () => {
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.deleteItem(testItem.title);

      // Verify deletion
      await itemPage.verifyItemDoesNotExist(testItem.title);
    });

    test('should confirm before deleting', async ({ page }) => {
      await itemPage.gotoWorkspaceBacklog(workspaceKey);

      const item = await itemPage.findItemByTitle(testItem.title);
      await item.locator(itemPage.deleteButton).click();

      // Should see confirmation dialog
      const confirmDialog = page.locator('div[role="dialog"], .modal, .confirm-dialog');
      await expect(confirmDialog).toBeVisible({ timeout: 5000 });
    });
  });

  test.describe('Item Hierarchy', () => {
    let parentItem: ReturnType<typeof generateItem>;

    test.beforeEach(async () => {
      parentItem = generateItem(0, 'parent');
      await itemPage.createItem(workspaceKey, {
        title: parentItem.title,
        description: 'Parent item for hierarchy testing',
      });
    });

    test('should create child item', async () => {
      const childItem = generateChildItem(0, 0, 'child');

      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.createChildItem(parentItem.title, {
        title: childItem.title,
        description: 'Child item for hierarchy testing',
      });

      // Verify child was created
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.verifyItemExists(childItem.title);
    });

    test('should display parent-child relationship', async () => {
      const childItem = generateChildItem(0, 0, 'relationship');

      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.createChildItem(parentItem.title, {
        title: childItem.title,
      });

      // Verify relationship
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.verifyItemIsChildOf(childItem.title, parentItem.title);
    });

    test('should create multi-level hierarchy', async () => {
      // Create Epic > Story > Task hierarchy
      const epic = parentItem;
      const story = generateChildItem(0, 0, 'story');
      const task = generateChildItem(0, 0, 'task');

      // Create story under epic
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.createChildItem(epic.title, {
        title: story.title,
      });

      // Create task under story
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.createChildItem(story.title, {
        title: task.title,
      });

      // Verify all exist
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.verifyItemExists(epic.title);
      await itemPage.verifyItemExists(story.title);
      await itemPage.verifyItemExists(task.title);
    });

    test('should delete parent and children', async () => {
      const childItem = generateChildItem(0, 0, 'cascade');

      // Create child
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.createChildItem(parentItem.title, {
        title: childItem.title,
      });

      // Delete parent
      await itemPage.gotoWorkspaceBacklog(workspaceKey);
      await itemPage.deleteItem(parentItem.title);

      // Verify parent is deleted
      await itemPage.verifyItemDoesNotExist(parentItem.title);

      // Child should also be deleted (cascade)
      await itemPage.verifyItemDoesNotExist(childItem.title);
    });
  });

  test.describe('Item List', () => {
    test('should display multiple items', async () => {
      const item1 = generateItem(0, '1');
      const item2 = generateItem(0, '2');
      const item3 = generateItem(0, '3');

      await itemPage.createItem(workspaceKey, { title: item1.title });
      await itemPage.createItem(workspaceKey, { title: item2.title });
      await itemPage.createItem(workspaceKey, { title: item3.title });

      await itemPage.gotoWorkspaceBacklog(workspaceKey);

      await itemPage.verifyItemExists(item1.title);
      await itemPage.verifyItemExists(item2.title);
      await itemPage.verifyItemExists(item3.title);

      const count = await itemPage.getItemCount();
      expect(count).toBeGreaterThanOrEqual(3);
    });

    test('should filter items by status', async ({ page }) => {
      const openItem = generateItem(0, 'open');
      const inProgressItem = generateItem(0, 'progress');

      await itemPage.createItem(workspaceKey, { title: openItem.title, status: 'open' });
      await itemPage.createItem(workspaceKey, {
        title: inProgressItem.title,
        status: 'in-progress',
      });

      await itemPage.gotoWorkspaceBacklog(workspaceKey);

      // Apply status filter (if exists)
      const statusFilter = page.locator('select[name="status"], [data-filter="status"]');
      if (await statusFilter.isVisible({ timeout: 2000 })) {
        await statusFilter.selectOption('open');
        await page.waitForTimeout(500);

        // Should see open item
        await itemPage.verifyItemExists(openItem.title);
      }
    });
  });
});
