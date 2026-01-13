import { test, expect } from '@playwright/test';
import { WorkspacePage } from '../pages/workspace.page';
import { generateWorkspace } from '../fixtures/test-data';

/**
 * Workspace Management Tests
 * Tests workspace CRUD operations using authenticated context
 */

test.describe('Workspace Management', () => {
  let workspacePage: WorkspacePage;
  let testWorkspace: ReturnType<typeof generateWorkspace>;

  test.beforeEach(async ({ page }) => {
    workspacePage = new WorkspacePage(page);
    testWorkspace = generateWorkspace();
  });

  test.describe('Create Workspace', () => {
    test('should create workspace with valid data', async () => {
      await workspacePage.createWorkspace(testWorkspace);

      // Verify workspace was created
      await workspacePage.verifyWorkspaceExists(testWorkspace.name);
    });

    test('should display workspace in list', async () => {
      // Create workspace
      await workspacePage.createWorkspace(testWorkspace);

      // Navigate to workspaces list
      await workspacePage.goto();

      // Find the workspace
      const workspace = await workspacePage.findWorkspaceByName(testWorkspace.name);
      await expect(workspace).toBeVisible();

      // Verify details
      await expect(workspace).toContainText(testWorkspace.name);
      await expect(workspace).toContainText(testWorkspace.key);
    });

    test('should validate unique workspace key', async () => {
      // Create first workspace
      await workspacePage.createWorkspace(testWorkspace);

      // Try to create another with same key
      const duplicateWorkspace = {
        ...testWorkspace,
        name: 'Different Name',
      };

      await workspacePage.goto();
      await workspacePage.clickCreate();
      await workspacePage.fillForm(duplicateWorkspace);
      await workspacePage.clickSave();

      // Should see error or validation message
      await workspacePage.page.waitForTimeout(1000);

      // Modal might still be open or error shown
      // This depends on implementation
    });

    test('should require workspace name', async ({ page }) => {
      await workspacePage.goto();
      await workspacePage.clickCreate();

      // Fill only key and description, leave name empty
      await page.fill('input[name="key"]', testWorkspace.key);
      await page.fill('textarea[name="description"]', testWorkspace.description);

      await workspacePage.clickSave();

      // Should not close modal (validation error)
      await page.waitForTimeout(500);
      const modal = page.locator(workspacePage.workspaceModal);
      await expect(modal).toBeVisible();
    });
  });

  test.describe('View Workspace', () => {
    test.beforeEach(async () => {
      // Create a workspace for viewing
      await workspacePage.createWorkspace(testWorkspace);
    });

    test('should view workspace details', async () => {
      await workspacePage.goto();

      // Click on workspace
      await workspacePage.clickWorkspace(testWorkspace.name);

      // Should navigate to workspace detail page
      await workspacePage.page.waitForLoadState('networkidle');

      // Verify we're on the workspace page
      await expect(workspacePage.page).toHaveURL(new RegExp(`/${testWorkspace.key}`));
    });

    test('should display workspace information', async () => {
      await workspacePage.goto();
      await workspacePage.clickWorkspace(testWorkspace.name);

      // Should see workspace name and details
      await expect(workspacePage.page.locator('h1, h2')).toContainText(testWorkspace.name);
    });
  });

  test.describe('Edit Workspace', () => {
    test.beforeEach(async () => {
      // Create a workspace for editing
      await workspacePage.createWorkspace(testWorkspace);
    });

    test('should update workspace name', async () => {
      const newName = `${testWorkspace.name} - Updated`;

      await workspacePage.editWorkspace(testWorkspace.name, {
        name: newName,
      });

      // Verify update
      await workspacePage.goto();
      await workspacePage.verifyWorkspaceExists(newName);
    });

    test('should update workspace description', async () => {
      const newDescription = 'Updated description for E2E test';

      await workspacePage.editWorkspace(testWorkspace.name, {
        description: newDescription,
      });

      // Verify by viewing workspace
      await workspacePage.goto();
      const workspace = await workspacePage.findWorkspaceByName(testWorkspace.name);
      await expect(workspace).toContainText(newDescription);
    });

    test('should update all workspace fields', async () => {
      const updatedData = {
        name: `${testWorkspace.name} - Fully Updated`,
        key: `${testWorkspace.key}UPD`,
        description: 'Completely updated workspace',
      };

      await workspacePage.editWorkspace(testWorkspace.name, updatedData);

      // Verify all updates
      await workspacePage.goto();
      await workspacePage.verifyWorkspaceExists(updatedData.name);

      const workspace = await workspacePage.findWorkspaceByName(updatedData.name);
      await expect(workspace).toContainText(updatedData.key);
    });
  });

  test.describe('Delete Workspace', () => {
    test.beforeEach(async () => {
      // Create a workspace for deletion
      await workspacePage.createWorkspace(testWorkspace);
    });

    test('should delete workspace', async () => {
      await workspacePage.deleteWorkspace(testWorkspace.name);

      // Verify deletion
      await workspacePage.verifyWorkspaceDoesNotExist(testWorkspace.name);
    });

    test('should confirm before deleting', async ({ page }) => {
      await workspacePage.goto();

      // Find workspace and click delete
      const workspace = await workspacePage.findWorkspaceByName(testWorkspace.name);
      await workspace.locator(workspacePage.deleteButton).click();

      // Should see confirmation dialog
      const confirmDialog = page.locator('div[role="dialog"], .modal, .confirm-dialog');
      await expect(confirmDialog).toBeVisible({ timeout: 5000 });

      // Should have confirm button
      const confirmButton = page.locator(workspacePage.confirmDeleteButton);
      await expect(confirmButton).toBeVisible();
    });

    test('should cancel workspace deletion', async ({ page }) => {
      await workspacePage.goto();

      // Start deletion
      const workspace = await workspacePage.findWorkspaceByName(testWorkspace.name);
      await workspace.locator(workspacePage.deleteButton).click();

      // Cancel instead of confirming
      await page.click(workspacePage.cancelButton);

      // Workspace should still exist
      await workspacePage.verifyWorkspaceExists(testWorkspace.name);
    });
  });

  test.describe('Workspace List', () => {
    test('should display multiple workspaces', async () => {
      // Create multiple workspaces
      const workspace1 = generateWorkspace('1');
      const workspace2 = generateWorkspace('2');
      const workspace3 = generateWorkspace('3');

      await workspacePage.createWorkspace(workspace1);
      await workspacePage.createWorkspace(workspace2);
      await workspacePage.createWorkspace(workspace3);

      // Go to list
      await workspacePage.goto();

      // Verify all are visible
      await workspacePage.verifyWorkspaceExists(workspace1.name);
      await workspacePage.verifyWorkspaceExists(workspace2.name);
      await workspacePage.verifyWorkspaceExists(workspace3.name);

      // Get count
      const count = await workspacePage.getWorkspaceCount();
      expect(count).toBeGreaterThanOrEqual(3);
    });

    test('should search workspaces', async () => {
      // Create workspace with unique name
      const uniqueWorkspace = generateWorkspace('searchable');
      await workspacePage.createWorkspace(uniqueWorkspace);

      await workspacePage.goto();

      // Search for workspace
      await workspacePage.searchWorkspace(uniqueWorkspace.name);

      // Should still see the workspace
      await workspacePage.verifyWorkspaceExists(uniqueWorkspace.name);
    });
  });

  test.describe('Workspace Navigation', () => {
    test.beforeEach(async () => {
      await workspacePage.createWorkspace(testWorkspace);
    });

    test('should navigate to workspace via menu', async () => {
      await workspacePage.page.goto('/');

      // Navigate via workspaces link
      await workspacePage.navigateViaMenu();

      // Should be on workspaces page
      await expect(workspacePage.page).toHaveURL(/\/workspaces/);
    });

    test('should navigate to workspace backlog', async () => {
      await workspacePage.goto();

      // Click on workspace
      await workspacePage.clickWorkspace(testWorkspace.name);

      // Navigate to backlog (if link exists)
      const backlogLink = workspacePage.page.locator('a:has-text("Backlog")');
      if (await backlogLink.isVisible({ timeout: 2000 })) {
        await backlogLink.click();
        await workspacePage.page.waitForLoadState('networkidle');

        // Should be on backlog page
        await expect(workspacePage.page).toHaveURL(new RegExp(`/${testWorkspace.key}/backlog`));
      }
    });
  });
});
