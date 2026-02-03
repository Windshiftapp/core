/**
 * Test data generators for consistent test data creation
 * Uses timestamps to ensure unique identifiers
 */

export interface TestWorkspace {
  name: string;
  key: string;
  description: string;
}

export interface TestItem {
  title: string;
  description: string;
  workspace_id?: number;
  parent_id?: number;
  status?: string;
  priority?: string;
}

export interface TestUser {
  email: string;
  username: string;
  first_name: string;
  last_name: string;
  password_hash: string;
}

export interface TestCustomField {
  name: string;
  field_type: string;
  description: string;
  required: boolean;
  options?: string;
}

/**
 * Generate unique workspace data
 */
export function generateWorkspace(suffix?: string): TestWorkspace {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    name: `E2E Test Workspace ${uniqueSuffix}`,
    key: `E2E${timestamp}`,
    description: `Test workspace created by E2E tests at ${new Date().toISOString()}`,
  };
}

/**
 * Generate unique item data
 */
export function generateItem(workspaceId: number, suffix?: string): TestItem {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    title: `E2E Test Item ${uniqueSuffix}`,
    description: `Test item created by E2E tests at ${new Date().toISOString()}`,
    workspace_id: workspaceId,
    status: 'open',
    priority: 'medium',
  };
}

/**
 * Generate parent-child item structure
 */
export function generateChildItem(
  workspaceId: number,
  parentId: number,
  suffix?: string
): TestItem {
  const item = generateItem(workspaceId, suffix);
  return {
    ...item,
    parent_id: parentId,
    title: `Child ${item.title}`,
  };
}

/**
 * Generate unique user data
 */
export function generateUser(suffix?: string): TestUser {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    email: `e2e.user.${uniqueSuffix}@test.com`,
    username: `e2euser${uniqueSuffix}`,
    first_name: 'E2E',
    last_name: `User ${uniqueSuffix}`,
    password_hash: 'TestPass123!',
  };
}

/**
 * Generate custom field data
 */
export function generateCustomField(type: string = 'text', suffix?: string): TestCustomField {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  const baseField: TestCustomField = {
    name: `E2E ${type} Field ${uniqueSuffix}`,
    field_type: type,
    description: `Test ${type} field created at ${new Date().toISOString()}`,
    required: false,
  };

  // Add options for select fields
  if (type === 'select') {
    baseField.options = JSON.stringify(['Option 1', 'Option 2', 'Option 3']);
  }

  return baseField;
}

/**
 * Generate workflow data
 */
export function generateWorkflow(suffix?: string) {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    name: `E2E Test Workflow ${uniqueSuffix}`,
    description: `Test workflow created at ${new Date().toISOString()}`,
  };
}

/**
 * Generate status category data
 */
export function generateStatusCategory(suffix?: string) {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    name: `E2E Status Category ${uniqueSuffix}`,
    color: '#3b82f6',
    description: `Test status category created at ${new Date().toISOString()}`,
    is_default: false,
  };
}

/**
 * Generate status data
 */
export function generateStatus(categoryId: number, suffix?: string) {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    name: `E2E Status ${uniqueSuffix}`,
    description: `Test status created at ${new Date().toISOString()}`,
    category_id: categoryId,
    is_default: false,
  };
}

/**
 * Generate screen data
 */
export function generateScreen(suffix?: string) {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    name: `E2E Test Screen ${uniqueSuffix}`,
    description: `Test screen created at ${new Date().toISOString()}`,
  };
}

/**
 * Generate configuration set data
 */
export function generateConfigurationSet(
  workflowId?: number,
  createScreenId?: number,
  editScreenId?: number,
  suffix?: string
) {
  const timestamp = Date.now();
  const uniqueSuffix = suffix || `${timestamp}`;

  return {
    name: `E2E Config Set ${uniqueSuffix}`,
    description: `Test configuration set created at ${new Date().toISOString()}`,
    workflow_id: workflowId,
    create_screen_id: createScreenId,
    edit_screen_id: editScreenId,
    is_default: false,
  };
}

/**
 * Wait helper for animations or async operations
 */
export async function waitFor(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Generate random string of specified length
 */
export function randomString(length: number = 10): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}
