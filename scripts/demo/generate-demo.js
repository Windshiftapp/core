#!/usr/bin/env node

/**
 * Demo Content Generator
 *
 * Generates comprehensive demo content including:
 * - Multiple workspaces (Software Dev, Support, Marketing)
 * - Custom fields and screens
 * - Demo users with different roles
 * - Hierarchical work items with realistic data
 * - Projects and priorities
 *
 * All seeding goes through the current v2 API (`/api/v2/...`, `{data: ...}`
 * envelopes). A few management endpoints are still session-auth legacy
 * surface (`/api/users`, milestone categories, iteration types, customer
 * organisations, personal workspace, item scheduling) and use the same
 * session cookie.
 *
 * Usage:
 *   node generate-demo.js [options]
 *
 * Options:
 *   --port <number>        Server port (default: 8080)
 *   --db <path>            Database file path (default: demo.db)
 *   --binary <path>        Path to server binary to build/run (default: ../../$BINARY_NAME or ../../windshift)
 *   --no-build             Skip frontend/backend build before starting the local server
 *   --no-server            Don't start server (assumes server is already running)
 *   --base-url <url>       Base URL if server is already running (default: http://localhost:8080)
 *   --keep-server          Don't stop server after completion
 *   --admin-user <user>    Admin username for login (default: admin)
 *   --admin-password <pw>  Admin password for login (default: admin)
 *   --help                 Show this help message
 */

import * as http from 'http';
import * as https from 'https';
import * as path from 'path';
import { fileURLToPath } from 'url';
import { spawn } from 'child_process';
import * as fs from 'fs';
import * as crypto from 'crypto';
import {
  demoUsers,
  workspaces,
  projects,
  customFields,
  screens,
  priorities,
  workItems,
  milestoneCategories,
  milestones,
  iterations,
  timeCustomers,
  workLogs,
  testLabels,
  testFolders,
  testCases,
  testSets,
  testRunTemplates,
  testCaseLinks,
  assetSets,
  assetTypes,
  assetCategories,
  assetTypeFields,
  assets,
  personalTasks
} from './demo-data.js';

// Date utility functions for relative date generation
function getWeekStart() {
  const now = new Date();
  const day = now.getDay();
  const diff = now.getDate() - day + (day === 0 ? -6 : 1); // Adjust for Monday
  const weekStart = new Date(now);
  weekStart.setDate(diff);
  weekStart.setHours(0, 0, 0, 0);
  return weekStart;
}

function formatDate(date) {
  return date.toISOString().split('T')[0];
}

function getRelativeDate(daysFromMonday) {
  const weekStart = getWeekStart();
  const result = new Date(weekStart);
  result.setDate(result.getDate() + daysFromMonday);
  return formatDate(result);
}

// Full ISO timestamp; the v2 item API decodes due_date/start_date as
// time.Time and rejects date-only strings with a misleading decode error.
function toISODate(dateStr) {
  return dateStr.includes('T') ? dateStr : `${dateStr}T00:00:00Z`;
}

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PROJECT_ROOT = path.join(__dirname, '../..');

// Configuration from environment variables
const APP_NAME = process.env.APP_NAME || 'WINDSHIFT';
const BINARY_NAME = process.env.BINARY_NAME || 'windshift';

// ANSI color codes for pretty output
const colors = {
  reset: '\x1b[0m',
  bright: '\x1b[1m',
  dim: '\x1b[2m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  red: '\x1b[31m'
};

// Parse command line arguments
function parseArgs() {
  const args = process.argv.slice(2);
  const options = {
    port: 8080,
    db: 'demo.db',
    binary: path.join(PROJECT_ROOT, BINARY_NAME),
    build: true,
    startServer: true,
    baseURL: null,
    keepServer: true,
    cleanDb: false,
    challenge: false,
    scale: false,
    adminUser: 'admin',
    adminPassword: 'admin'
  };

  for (let i = 0; i < args.length; i++) {
    switch (args[i]) {
      case '--port':
        options.port = parseInt(args[++i], 10);
        break;
      case '--db':
        options.db = args[++i];
        break;
      case '--binary':
        options.binary = args[++i];
        break;
      case '--no-build':
        options.build = false;
        break;
      case '--no-server':
        options.startServer = false;
        break;
      case '--base-url':
        options.baseURL = args[++i];
        options.startServer = false;
        break;
      case '--keep-server':
        options.keepServer = true;
        break;
      case '--stop-server':
        options.keepServer = false;
        break;
      case '--clean':
        options.cleanDb = true;
        break;
      case '--challenge':
        options.challenge = true;
        break;
      case '--scale':
        options.scale = true;
        break;
      case '--admin-user':
        options.adminUser = args[++i];
        break;
      case '--admin-password':
        options.adminPassword = args[++i];
        break;
      case '--help':
        console.log(`
${colors.bright}${APP_NAME} Demo Content Generator${colors.reset}

${colors.cyan}Usage:${colors.reset}
  node generate-demo.js [options]

${colors.cyan}Options:${colors.reset}
  --port <number>        Server port (default: 8080)
  --db <path>            Database file path (default: demo.db)
  --binary <path>        Path to server binary to build/run (default: ../../${BINARY_NAME})
  --no-build             Skip frontend/backend build before starting the local server
  --no-server            Don't start server (assumes server is already running)
  --base-url <url>       Base URL if server is already running (default: http://localhost:8080)
  --keep-server          Keep server running after completion (default)
  --stop-server          Stop server after completion (for CI/e2e tests)
  --clean                Delete existing database before generating demo
  --challenge            Include edge-case and security test data
  --scale                Generate large-scale dataset (22 workspaces, 10,000+ items)
  --admin-user <user>    Admin username for login (default: admin)
  --admin-password <pw>  Admin password for login (default: admin)
  --help                 Show this help message

${colors.cyan}Examples:${colors.reset}
  ${colors.dim}# Generate demo with default settings${colors.reset}
  node generate-demo.js

  ${colors.dim}# Use existing server (will warn if setup exists)${colors.reset}
  node generate-demo.js --no-server --base-url http://localhost:3000

  ${colors.dim}# Clean existing database with external server${colors.reset}
  node generate-demo.js --no-server --base-url http://localhost:3000 --clean

  ${colors.dim}# Custom database and keep server running${colors.reset}
  node generate-demo.js --db my-demo.db --keep-server

  ${colors.dim}# Use an already-built binary${colors.reset}
  node generate-demo.js --no-build --binary /path/to/${BINARY_NAME}

  ${colors.dim}# Connect to existing instance with custom credentials${colors.reset}
  node generate-demo.js --no-server --base-url https://example.com --admin-user myuser --admin-password mypass
`);
        process.exit(0);
      default:
        console.error(`${colors.red}Unknown option: ${args[i]}${colors.reset}`);
        process.exit(1);
    }
  }

  if (!path.isAbsolute(options.binary)) {
    options.binary = path.resolve(process.cwd(), options.binary);
  }

  if (!options.baseURL) {
    options.baseURL = `http://localhost:${options.port}`;
  }

  return options;
}

// Logging helpers
function log(message, color = colors.reset) {
  console.log(`${color}${message}${colors.reset}`);
}

function logSection(title) {
  console.log(`\n${colors.bright}${colors.cyan}=== ${title} ===${colors.reset}`);
}

function logSuccess(message) {
  log(`${colors.green}✓${colors.reset} ${message}`);
}

function logError(message) {
  log(`${colors.red}✗${colors.reset} ${message}`, colors.red);
}

function logInfo(message) {
  log(`${colors.blue}ℹ${colors.reset} ${message}`, colors.dim);
}

// HTTP request helper
function makeRequest(method, url, data = null, headers = {}) {
  return new Promise((resolve, reject) => {
    const urlObj = new URL(url);
    const isHttps = urlObj.protocol === 'https:';
    const options = {
      hostname: urlObj.hostname,
      port: urlObj.port || (isHttps ? 443 : 80),
      path: urlObj.pathname + urlObj.search,
      method: method,
      headers: {
        'Content-Type': 'application/json',
        ...headers
      }
    };

    const httpModule = isHttps ? https : http;
    const req = httpModule.request(options, (res) => {
      let body = '';
      res.on('data', (chunk) => body += chunk);
      res.on('end', () => {
        try {
          const jsonData = body ? JSON.parse(body) : null;
          resolve({
            status: res.statusCode,
            data: jsonData,
            headers: res.headers,
            cookies: res.headers['set-cookie'] || []
          });
        } catch (e) {
          resolve({
            status: res.statusCode,
            data: body,
            headers: res.headers,
            cookies: res.headers['set-cookie'] || []
          });
        }
      });
    });

    req.on('error', reject);

    if (data) {
      req.write(JSON.stringify(data));
    }

    req.end();
  });
}

// Wait for server to be ready
async function waitForServer(baseURL, timeout = 30000) {
  const startTime = Date.now();
  const checkURL = `${baseURL}/api/setup/status`;

  while (Date.now() - startTime < timeout) {
    try {
      const response = await makeRequest('GET', checkURL);
      if (response.status < 500) {
        return true;
      }
    } catch (e) {
      // Server not ready yet
    }
    await new Promise(resolve => setTimeout(resolve, 500));
  }
  return false;
}

// Check if port is in use
async function isPortInUse(port) {
  return new Promise((resolve) => {
    const server = http.createServer();

    server.once('error', (err) => {
      if (err.code === 'EADDRINUSE') {
        resolve(true);
      } else {
        resolve(false);
      }
    });

    server.once('listening', () => {
      server.close();
      resolve(false);
    });

    server.listen(port);
  });
}

// Clean database files
function cleanDatabase(dbPath) {
  if (fs.existsSync(dbPath)) {
    logInfo(`Removing existing database: ${dbPath}`);
    fs.unlinkSync(dbPath);
  }
  // Also remove WAL files
  if (fs.existsSync(dbPath + '-shm')) {
    fs.unlinkSync(dbPath + '-shm');
  }
  if (fs.existsSync(dbPath + '-wal')) {
    fs.unlinkSync(dbPath + '-wal');
  }
}

function runCommand(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    logInfo(`$ ${command} ${args.join(' ')}`);
    const child = spawn(command, args, {
      cwd: options.cwd || PROJECT_ROOT,
      stdio: 'inherit',
      env: { ...process.env, ...(options.env || {}) },
      shell: false
    });

    child.on('error', reject);
    child.on('exit', (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`${command} ${args.join(' ')} failed${signal ? ` with signal ${signal}` : ` with exit code ${code}`}`));
    });
  });
}

async function buildApplication(options) {
  logSection('Building Frontend and Backend');

  const frontendDir = path.join(PROJECT_ROOT, 'frontend');
  if (!fs.existsSync(path.join(frontendDir, 'node_modules'))) {
    logInfo('frontend/node_modules not found; installing frontend dependencies first...');
    await runCommand('npm', ['install'], { cwd: frontendDir });
  }

  // Build frontend before Go so //go:embed captures a current frontend/dist/index.html.
  await runCommand('npm', ['run', 'build'], { cwd: frontendDir });

  // Match the production Makefile build: exclude test code and embed the freshly
  // built frontend dist into the binary that this script is about to run.
  await runCommand('go', ['build', '-tags', '!test', '-ldflags', '-s -w', '-o', options.binary, '.'], { cwd: PROJECT_ROOT });

  logSuccess(`Built ${options.binary}`);
}

// Start server
async function startServer(options) {
  logSection('Starting Server');

  // Check if port is already in use
  if (await isPortInUse(options.port)) {
    logError(`Port ${options.port} is already in use!`);
    logInfo('Either:');
    logInfo(`  1. Stop the existing server: lsof -ti:${options.port} | xargs kill`);
    logInfo(`  2. Use a different port: --port <number>`);
    logInfo(`  3. Use existing server: --no-server --base-url http://localhost:${options.port}`);
    process.exit(1);
  }

  // Check if binary exists
  if (!fs.existsSync(options.binary)) {
    logError(`${APP_NAME} binary not found at: ${options.binary}`);
    logInfo(`Run without --no-build so this script can build it, or build manually: go build -o ${BINARY_NAME}`);
    process.exit(1);
  }

  // Delete existing database if it exists
  cleanDatabase(options.db);

  logInfo(`Starting server on port ${options.port}...`);

  // Generate a random session secret for the demo server
  const sessionSecret = crypto.randomBytes(32).toString('hex');

  const serverProcess = spawn(options.binary, [
    '-db', options.db,
    '-p', options.port.toString(),
    '-no-csrf',
    '--allowed-hosts', 'localhost,127.0.0.1'
  ], {
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: false,
    env: {
      ...process.env,
      SESSION_SECRET: sessionSecret
    }
  });

  // The Go slog default writes INFO/WARN/ERROR to stderr. Surface it without
  // painting normal startup/audit messages as demo-generator failures.
  serverProcess.stderr.on('data', (data) => {
    const msg = data.toString().trim();
    if (!msg) return;
    if (msg.includes('ERROR')) {
      logError(`Server: ${msg}`);
    } else if (msg.includes('WARN')) {
      log(`Server: ${msg}`, colors.yellow);
    } else {
      log(`Server: ${msg}`, colors.dim);
    }
  });

  serverProcess.on('error', (err) => {
    logError(`Failed to start server: ${err.message}`);
  });

  serverProcess.on('exit', (code, signal) => {
    if (code !== null && code !== 0) {
      logError(`Server exited with code ${code}`);
    }
  });

  // Wait for server to be ready
  logInfo('Waiting for server to be ready...');
  const ready = await waitForServer(options.baseURL);

  if (!ready) {
    logError('Server failed to start within 30 seconds');
    serverProcess.kill();
    process.exit(1);
  }

  logSuccess(`Server started at ${options.baseURL}`);

  return serverProcess;
}

// Authenticated session for both API surfaces.
//
// The v2 surface (`/api/v2/*`) and the remaining legacy session-auth routes
// (`/api/users`, `/api/milestone-categories`, ...) share the session cookie.
// API tokens (`crw_*`) are only valid on `/rest/api/v1/*` and cannot be used
// here. CSRF validation on both surfaces accepts the `Sec-Fetch-Site:
// same-origin` header, which we send on every request; servers we spawn
// additionally run with `-no-csrf`.
async function makeAuthRequest(baseURL, method, endpoint, data, sessionCookie, extraHeaders = {}) {
  return makeRequest(method, `${baseURL}${endpoint}`, data, {
    'Cookie': sessionCookie,
    'Sec-Fetch-Site': 'same-origin',
    ...extraHeaders
  });
}

// v2 request returning the unwrapped `data` payload. Throws on non-2xx with
// the status and the server's error message so failures are diagnosable.
async function v2(baseURL, sessionCookie, method, endpoint, data, extraHeaders = {}) {
  const response = await makeAuthRequest(baseURL, method, endpoint, data, sessionCookie, extraHeaders);
  if (response.status < 200 || response.status >= 300) {
    // v2 errors use {error: {code, message, details?}}; older surfaces use flat strings.
    const err = response.data?.error;
    const detail = err?.message || response.data?.message || JSON.stringify(response.data)?.slice(0, 300);
    throw new Error(`${method} ${endpoint} failed: ${response.status} - ${detail}`);
  }
  const body = response.data;
  return body && typeof body === 'object' && 'data' in body ? body.data : body;
}

// Merge-patch JSON is the canonical partial-update content type on v2.
async function v2Patch(baseURL, sessionCookie, endpoint, data) {
  return v2(baseURL, sessionCookie, 'PATCH', endpoint, data, {
    'Content-Type': 'application/merge-patch+json'
  });
}

// Complete initial setup. Safe to call against an already-configured
// instance; required for fresh databases even when the server was started
// externally (the Docker seeder runs with --no-server).
async function completeSetup(baseURL, options = {}) {
  logSection('Completing Initial Setup');

  const statusResponse = await makeRequest('GET', `${baseURL}/api/setup/status`);
  if (statusResponse.status === 200 && statusResponse.data?.setup_completed) {
    logInfo('Setup already completed');
    return true;
  }

  const setupData = {
    admin_user: {
      email: 'admin@demo.com',
      username: options.adminUser || 'admin',
      password: options.adminPassword || 'admin', // Plaintext; hashed server-side
      first_name: 'Admin',
      last_name: 'User'
    },
    module_settings: {
      time_tracking_enabled: true,
      test_management_enabled: true
    }
  };

  try {
    const response = await makeRequest('POST', `${baseURL}/api/setup/complete`, setupData, {
      'Sec-Fetch-Site': 'same-origin'
    });

    if (response.status === 200 || response.status === 201) {
      logSuccess('Initial setup completed');
      return true;
    } else if (response.status === 400 && JSON.stringify(response.data)?.includes('already been completed')) {
      logInfo('Setup already completed');
      return true;
    } else {
      logError(`Setup failed: ${response.status} - ${JSON.stringify(response.data)}`);
      return false;
    }
  } catch (error) {
    logError(`Setup error: ${error.message}`);
    return false;
  }
}

// Get a session cookie for both API surfaces.
async function getSessionCookie(baseURL, options = {}) {
  logSection('Getting Session Cookie');

  const adminUser = options.adminUser || 'admin';
  const adminPassword = options.adminPassword || 'admin';

  try {
    logInfo(`Logging in as ${adminUser}...`);
    const loginData = {
      email_or_username: adminUser,
      password: adminPassword
    };

    const loginResponse = await makeRequest('POST', `${baseURL}/api/auth/login`, loginData, {
      'Sec-Fetch-Site': 'same-origin'
    });

    if (loginResponse.status !== 200) {
      logError(`Login failed: ${loginResponse.status} - ${JSON.stringify(loginResponse.data)}`);
      return null;
    }

    // Extract session cookie
    const cookies = loginResponse.cookies;
    if (!cookies || cookies.length === 0) {
      logError('No session cookie received from login');
      return null;
    }

    let sessionCookie = null;
    for (const cookie of cookies) {
      if (cookie.includes('session') || cookie.includes('windshift_session')) {
        sessionCookie = cookie.split(';')[0]; // Get just the name=value part
        break;
      }
    }

    if (!sessionCookie) {
      logError('No session cookie found in response');
      return null;
    }

    logInfo('Session cookie obtained');

    return sessionCookie;
  } catch (error) {
    logError(`Authentication error: ${error.message}`);
    if (error.stack) {
      logInfo(error.stack);
    }
    return null;
  }
}

// Backwards-compatible export name for callers that imported this helper
// before `/api/*` switched to session-only auth.
const getBearerToken = getSessionCookie;

// Create demo users.
// POST /api/users is still legacy session-auth surface; accounts can be
// created directly active. Re-runs against an existing instance resolve the
// already-present accounts from GET /api/v2/users.
async function createUsers(baseURL, token, usersData = demoUsers) {
  logSection('Creating Demo Users');

  const createdUsers = {};

  // Fetch existing users so we can resolve IDs for already-created ones
  let existingUsers = [];
  try {
    existingUsers = await v2(baseURL, token, 'GET', '/api/v2/users?page_size=1000');
    if (!Array.isArray(existingUsers)) existingUsers = [];
  } catch (_) { /* ignore */ }

  const findExisting = (username) => existingUsers.find(u => u.username === username);

  for (const user of usersData) {
    try {
      const response = await makeAuthRequest(baseURL, 'POST', '/api/users', { ...user, is_active: true }, token);

      if (response.status === 200 || response.status === 201) {
        const created = response.data?.data ?? response.data;
        createdUsers[user.username] = {
          id: created.id,
          name: `${user.first_name} ${user.last_name}`.trim()
        };
        logSuccess(`Created user: ${user.first_name} ${user.last_name} (${user.role})`);
      } else if (response.status === 409) {
        // User already exists - find them in the existing list
        const existing = findExisting(user.username);
        if (existing) {
          createdUsers[user.username] = {
            id: existing.id,
            name: `${existing.first_name} ${existing.last_name}`.trim()
          };
          logInfo(`User already exists: ${user.username} (id: ${existing.id})`);
        } else {
          logError(`User ${user.username} exists but could not find in user list`);
        }
      } else {
        logError(`Failed to create user ${user.username}: ${response.status} - ${JSON.stringify(response.data)?.slice(0, 200)}`);
      }
    } catch (error) {
      logError(`Error creating user ${user.username}: ${error.message}`);
    }
  }

  // Accounts that already existed may be inactive (older seeds created them
  // that way); activate them so they are assignable.
  for (const user of usersData) {
    const resolved = createdUsers[user.username];
    const existing = findExisting(user.username);
    if (resolved && existing && existing.is_active === false) {
      try {
        await makeAuthRequest(baseURL, 'POST', `/api/users/${resolved.id}/activate`, null, token);
        logInfo(`Activated existing user: ${user.username}`);
      } catch (_) { /* best effort */ }
    }
  }

  return createdUsers;
}

// Create workspaces
async function createWorkspaces(baseURL, token, workspacesData = workspaces) {
  logSection('Creating Workspaces');

  const createdWorkspaces = {};

  // Fetch existing workspaces so we can resolve IDs for already-created ones
  let existingWorkspaces = [];
  try {
    existingWorkspaces = await v2(baseURL, token, 'GET', '/api/v2/workspaces?page_size=1000');
    if (!Array.isArray(existingWorkspaces)) existingWorkspaces = [];
  } catch (_) { /* ignore */ }

  for (const workspace of workspacesData) {
    // Check if workspace already exists by key
    const existing = existingWorkspaces.find(w => w.key === workspace.key);
    if (existing) {
      createdWorkspaces[workspace.key] = existing.id;
      logInfo(`Workspace already exists: ${workspace.name} (${workspace.key}, id: ${existing.id})`);
      continue;
    }

    try {
      const created = await v2(baseURL, token, 'POST', '/api/v2/workspaces', workspace);
      createdWorkspaces[workspace.key] = created.id;
      logSuccess(`Created workspace: ${workspace.name} (${workspace.key})`);
    } catch (error) {
      logError(`Error creating workspace ${workspace.key}: ${error.message}`);
    }
  }

  return createdWorkspaces;
}

// Create time tracking projects (per workspace, tied to a customer)
async function createProjects(baseURL, token, workspaceMap, customerMap, projectsData = projects) {
  logSection('Creating Time Tracking Projects');

  const createdProjects = {};

  for (const project of projectsData) {
    const customerId = customerMap[project.customerName];
    if (!customerId) {
      logError(`Customer ${project.customerName} not found for project ${project.name}`);
      continue;
    }

    try {
      const projectData = {
        customer_id: customerId,
        name: project.name,
        description: project.description
      };

      const created = await v2(baseURL, token, 'POST', '/api/v2/time/projects', projectData);
      const key = `${project.workspaceKey}:${project.name}`;
      createdProjects[key] = created.id;
      logSuccess(`Created project: ${project.name} for ${project.customerName}`);
    } catch (error) {
      logError(`Error creating project ${project.name}: ${error.message}`);
    }
  }

  return createdProjects;
}

// Parse a select field's options payload. The v2 catalog returns the parsed
// object; the mutation DTO returns a JSON string in the normalized format.
function parseSelectOptions(options) {
  if (!options) return [];
  const parsed = typeof options === 'string' ? JSON.parse(options) : options;
  return Array.isArray(parsed?.items) ? parsed.items : [];
}

// Create custom fields.
// Returns [fieldIdMap, selectOptionMap]: fieldIdMap maps field name → ID,
// selectOptionMap maps select field name → { label → option id }. Select
// values in item payloads must be numeric option IDs, not labels.
async function createCustomFields(baseURL, token) {
  logSection('Creating Custom Fields');

  const createdFields = {};
  const selectOptions = {};

  const recordField = (field, payload) => {
    createdFields[field.name] = payload.id;
    if (field.field_type === 'select' || field.field_type === 'multiselect') {
      const options = {};
      for (const item of parseSelectOptions(payload.options)) {
        options[item.label] = item.id;
      }
      selectOptions[field.name] = options;
    }
  };

  for (const field of customFields) {
    try {
      const response = await v2(baseURL, token, 'POST', '/api/v2/custom-fields', {
        name: field.name,
        field_type: field.field_type,
        description: field.description || '',
        required: field.required || false,
        options: field.options || ''
      });
      recordField(field, response.custom_field ?? response);
      logSuccess(`Created custom field: ${field.name} (${field.field_type})`);
    } catch (error) {
      logError(`Error creating field ${field.name}: ${error.message}`);
    }
  }

  // Resolve fields that already existed (re-runs against a populated instance)
  try {
    const existing = await v2(baseURL, token, 'GET', '/api/v2/custom-fields?page_size=1000');
    for (const field of Array.isArray(existing) ? existing : []) {
      const definition = customFields.find(f => f.name === field.name);
      if (definition && !createdFields[field.name]) {
        recordField(definition, field);
        logInfo(`Custom field already exists: ${field.name} (id: ${field.id})`);
      }
    }
  } catch (_) { /* ignore */ }

  return [createdFields, selectOptions];
}

// Create screens
async function createScreens(baseURL, token, fieldMap) {
  logSection('Creating Screens');

  const createdScreens = {};

  for (const screen of screens) {
    try {
      const created = await v2(baseURL, token, 'POST', '/api/v2/screens', {
        name: screen.name,
        description: screen.description
      });

      createdScreens[screen.name] = created.id;
      logSuccess(`Created screen: ${screen.name}`);

      // Add fields to screen. v2 screen fields identify custom fields with
      // field_type "custom" and the field ID as a string identifier.
      if (screen.fields && screen.fields.length > 0) {
        const fieldsData = screen.fields
          .map(fieldName => fieldMap[fieldName])
          .filter(id => id !== undefined)
          .map((fieldId, index) => ({
            field_type: 'custom',
            field_identifier: String(fieldId),
            display_order: index + 1,
            is_required: false
          }));

        if (fieldsData.length > 0) {
          try {
            await v2(baseURL, token, 'PUT', `/api/v2/screens/${created.id}/fields`, { fields: fieldsData });
            logInfo(`  Added ${fieldsData.length} fields to screen`);
          } catch (error) {
            logError(`  Failed to add fields to screen: ${error.message}`);
          }
        }
      }
    } catch (error) {
      logError(`Error creating screen ${screen.name}: ${error.message}`);
    }
  }

  return createdScreens;
}

// Create priorities
async function createPriorities(baseURL, token) {
  logSection('Creating Priorities');

  const createdPriorities = {};

  // Fetch existing priorities; a fresh database already has the builtin set.
  let existing = [];
  try {
    existing = await v2(baseURL, token, 'GET', '/api/v2/priorities?page_size=1000');
    if (!Array.isArray(existing)) existing = [];
  } catch (_) { /* ignore */ }

  for (const priority of priorities) {
    const match = existing.find(p => p.name === priority.name);
    if (match) {
      createdPriorities[priority.name] = match.id;
      logInfo(`Priority already exists: ${priority.name} (id: ${match.id})`);
      continue;
    }

    try {
      const created = await v2(baseURL, token, 'POST', '/api/v2/priorities', priority);
      createdPriorities[priority.name] = created.id;
      logSuccess(`Created priority: ${priority.name} ${priority.icon}`);
    } catch (error) {
      logError(`Failed to create priority ${priority.name}: ${error.message}`);
    }
  }

  return createdPriorities;
}

// Create milestone categories (global; still a session-auth legacy surface)
async function createMilestoneCategories(baseURL, token) {
  logSection('Creating Milestone Categories');

  const categoryMap = {};

  // Fetch existing categories
  let existingCategories = [];
  try {
    const listResp = await makeAuthRequest(baseURL, 'GET', '/api/milestone-categories', null, token);
    if (listResp.status === 200) {
      const body = listResp.data?.data ?? listResp.data;
      if (Array.isArray(body)) existingCategories = body;
    }
  } catch (_) { /* ignore */ }

  for (const category of milestoneCategories) {
    // Check if already exists
    const existing = existingCategories.find(c => c.name === category.name);
    if (existing) {
      categoryMap[category.name] = existing.id;
      logInfo(`Milestone category already exists: ${category.name} (id: ${existing.id})`);
      continue;
    }

    try {
      const response = await makeAuthRequest(baseURL, 'POST', '/api/milestone-categories', {
        name: category.name,
        color: category.color,
        description: category.description
      }, token);

      if (response.status === 200 || response.status === 201) {
        const created = response.data?.data ?? response.data;
        categoryMap[category.name] = created.id;
        logSuccess(`Created milestone category: ${category.name}`);
      } else {
        logError(`Failed to create milestone category ${category.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating milestone category ${category.name}: ${error.message}`);
    }
  }

  return categoryMap;
}

// Create milestones. Milestones are workspace-scoped on v2; global milestones
// use the unscoped route, workspace milestones the nested route.
async function createMilestones(baseURL, token, workspaceMap, categoryMap = {}, milestonesData = milestones) {
  logSection('Creating Milestones');

  const createdMilestones = {};

  for (const milestone of milestonesData) {
    const endpoint = milestone.is_global
      ? '/api/v2/milestones'
      : `/api/v2/workspaces/${workspaceMap[milestone.workspaceKey]}/milestones`;

    if (!milestone.is_global && !workspaceMap[milestone.workspaceKey]) {
      logError(`Workspace ${milestone.workspaceKey} not found for milestone ${milestone.name}`);
      continue;
    }

    try {
      const targetDate = getRelativeDate(milestone.daysFromMonday);
      const milestoneData = {
        name: milestone.name,
        description: milestone.description,
        target_date: targetDate,
        status: milestone.status
      };

      // Add category_id if milestone has a categoryName and category exists
      if (milestone.categoryName && categoryMap[milestone.categoryName]) {
        milestoneData.category_id = categoryMap[milestone.categoryName];
      }

      const created = await v2(baseURL, token, 'POST', endpoint, milestoneData);

      // Key format: global milestones use just name, local use workspace:name
      const key = milestone.is_global
        ? milestone.name
        : `${milestone.workspaceKey}:${milestone.name}`;
      createdMilestones[key] = created.id;
      const scope = milestone.is_global ? '(global)' : `(${milestone.workspaceKey})`;
      logSuccess(`Created milestone: ${milestone.name} ${scope} (${targetDate})`);
    } catch (error) {
      logError(`Error creating milestone ${milestone.name}: ${error.message}`);
    }
  }

  return createdMilestones;
}

// Get iteration types from the legacy catalog surface (returns name → id map)
async function getIterationTypes(baseURL, token) {
  try {
    const response = await makeAuthRequest(baseURL, 'GET', '/api/iteration-types', null, token);

    if (response.status === 200) {
      const body = response.data?.data ?? response.data;
      if (Array.isArray(body)) {
        const typeMap = {};
        for (const type of body) {
          typeMap[type.name] = type.id;
        }
        return typeMap;
      }
    }

    logError(`Failed to fetch iteration types: ${response.status}`);
    return {};
  } catch (error) {
    logError(`Error fetching iteration types: ${error.message}`);
    return {};
  }
}

// Create iterations. Like milestones, iterations are workspace-scoped on v2.
async function createIterations(baseURL, token, workspaceMap, iterationTypeMap = {}, iterationsData = iterations) {
  // Backwards compatibility for callers using the old signature:
  // createIterations(baseURL, token, workspaceMap, iterationsData)
  if (Array.isArray(iterationTypeMap)) {
    iterationsData = iterationTypeMap;
    iterationTypeMap = {};
  }

  logSection('Creating Iterations');

  if (Object.keys(iterationTypeMap).length === 0) {
    iterationTypeMap = await getIterationTypes(baseURL, token);
  }

  const createdIterations = {};

  for (const iteration of iterationsData) {
    const endpoint = iteration.is_global
      ? '/api/v2/iterations'
      : `/api/v2/workspaces/${workspaceMap[iteration.workspaceKey]}/iterations`;

    if (!iteration.is_global && !workspaceMap[iteration.workspaceKey]) {
      logError(`Workspace ${iteration.workspaceKey} not found for iteration ${iteration.name}`);
      continue;
    }

    try {
      const startDate = getRelativeDate(iteration.daysFromMonday);
      const endDate = getRelativeDate(iteration.daysFromMonday + iteration.durationDays);

      const iterationData = {
        name: iteration.name,
        description: iteration.description,
        start_date: startDate,
        end_date: endDate,
        status: iteration.status,
        type_id: iterationTypeMap[iteration.type] || iterationTypeMap['Sprint'] || null
      };

      const created = await v2(baseURL, token, 'POST', endpoint, iterationData);

      // Key format: global iterations use just name, local use workspace:name
      const key = iteration.is_global
        ? iteration.name
        : `${iteration.workspaceKey}:${iteration.name}`;
      createdIterations[key] = created.id;
      const scope = iteration.is_global ? '(global)' : `(${iteration.workspaceKey})`;
      logSuccess(`Created iteration: ${iteration.name} [${iteration.type}] ${scope} (${startDate} - ${endDate})`);
    } catch (error) {
      logError(`Error creating iteration ${iteration.name}: ${error.message}`);
    }
  }

  return createdIterations;
}

// Get link types from the API (returns name → id map)
async function getLinkTypes(baseURL, token) {
  try {
    const linkTypes = await v2(baseURL, token, 'GET', '/api/v2/link-types');
    const result = {};
    for (const linkType of Array.isArray(linkTypes) ? linkTypes : []) {
      result[linkType.name] = linkType.id;
    }
    return result;
  } catch (error) {
    logError(`Error fetching link types: ${error.message}`);
    return {};
  }
}

// Get item types from the API
async function getItemTypes(baseURL, token) {
  try {
    const itemTypes = await v2(baseURL, token, 'GET', '/api/v2/item-types?page_size=1000');
    const result = {};
    for (const itemType of Array.isArray(itemTypes) ? itemTypes : []) {
      result[itemType.name] = itemType.id;
    }
    return result;
  } catch (error) {
    logError(`Error fetching item types: ${error.message}`);
    return {};
  }
}

// Get statuses from the API (returns name → id map)
async function getStatuses(baseURL, token) {
  try {
    const statuses = await v2(baseURL, token, 'GET', '/api/v2/statuses?page_size=1000');
    const result = {};
    for (const status of Array.isArray(statuses) ? statuses : []) {
      result[status.name] = status.id;
    }
    return result;
  } catch (error) {
    logError(`Error fetching statuses: ${error.message}`);
    return {};
  }
}

// Determine appropriate item type based on item characteristics
function determineItemType(item, depth, itemTypes) {
  const title = item.title.toLowerCase();
  const description = (item.description || '').toLowerCase();
  const isBugRelated = title.includes('bug') || title.includes('fix') ||
    description.includes('bug') || description.includes('defect');

  if (depth === 0) {
    // Top-level: use Bug if detected, otherwise Epic
    if (isBugRelated && itemTypes['Bug']) {
      return itemTypes['Bug'];
    }
    return itemTypes['Epic'] || null;
  } else if (depth === 1) {
    // Children of Epic (level 1) — must be Story (level 2) for hierarchy
    return itemTypes['Story'] || null;
  } else if (depth === 2) {
    // Children of Story (level 2) — Bug (level 3) or Task (level 3) both valid
    if (isBugRelated && itemTypes['Bug']) {
      return itemTypes['Bug'];
    }
    return itemTypes['Task'] || null;
  } else {
    // Deep nesting (depth 3+) - Sub-tasks
    return itemTypes['Sub-task'] || itemTypes['Task'] || null;
  }
}

// Create time tracking customers (still a session-auth legacy surface)
async function createTimeCustomers(baseURL, token, customersData = timeCustomers) {
  logSection('Creating Time Tracking Customers');

  const createdCustomers = {};

  for (const customer of customersData) {
    try {
      const customerData = {
        name: customer.name,
        email: customer.email,
        description: customer.description,
        active: customer.active
      };

      const response = await makeAuthRequest(baseURL, 'POST', '/api/customer-organisations', customerData, token);

      if (response.status === 200 || response.status === 201) {
        const created = response.data?.data ?? response.data;
        createdCustomers[customer.name] = created.id;
        logSuccess(`Created time customer: ${customer.name}`);
      } else if (response.status === 409) {
        logInfo(`Time customer already exists: ${customer.name}`);
      } else {
        logError(`Failed to create customer ${customer.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating customer ${customer.name}: ${error.message}`);
    }
  }

  // Resolve IDs for customers that already existed
  try {
    const listResp = await makeAuthRequest(baseURL, 'GET', '/api/customer-organisations', null, token);
    if (listResp.status === 200) {
      const body = listResp.data?.data ?? listResp.data;
      for (const customer of Array.isArray(body) ? body : []) {
        if (timeCustomers.some(c => c.name === customer.name) && !createdCustomers[customer.name]) {
          createdCustomers[customer.name] = customer.id;
        }
      }
    }
  } catch (_) { /* ignore */ }

  return createdCustomers;
}

// Create work logs for items
async function createWorkLogs(baseURL, token, itemMap, projectMap) {
  logSection('Creating Work Logs');

  let createdCount = 0;

  for (const logEntry of workLogs) {
    // Find the item by title and workspace
    const itemKey = `${logEntry.workspaceKey}:${logEntry.itemTitle}`;
    const itemId = itemMap[itemKey];

    if (!itemId) {
      logError(`Item "${logEntry.itemTitle}" not found in workspace ${logEntry.workspaceKey}`);
      continue;
    }

    // Find the project ID
    const projectKey = `${logEntry.workspaceKey}:${logEntry.projectName}`;
    const projectId = projectMap[projectKey];

    if (!projectId) {
      logError(`Project "${logEntry.projectName}" not found in workspace ${logEntry.workspaceKey}`);
      continue;
    }

    try {
      const logData = {
        project_id: projectId,
        item_id: itemId,
        description: logEntry.description,
        date: logEntry.date,
        duration: logEntry.duration
      };

      await v2(baseURL, token, 'POST', '/api/v2/time/worklogs', logData);
      createdCount++;
      logSuccess(`Created work log: ${logEntry.duration} on "${logEntry.itemTitle}" in ${logEntry.projectName}`);
    } catch (error) {
      logError(`Error creating work log for ${logEntry.itemTitle}: ${error.message}`);
    }
  }

  return createdCount;
}

// Map custom field names in the data module to custom field IDs; the v2 item
// API keys custom_field_values by numeric field ID and silently drops other
// keys. Select values are translated from labels to option IDs.
function mapCustomFieldValues(customFieldsByName, fieldMap, selectOptionMap = {}) {
  if (!customFieldsByName) return undefined;
  const mapped = {};
  for (const [name, value] of Object.entries(customFieldsByName)) {
    const fieldId = fieldMap[name];
    if (fieldId === undefined) continue;
    mapped[String(fieldId)] = selectOptionMap[name]?.[value] ?? value;
  }
  return Object.keys(mapped).length > 0 ? mapped : undefined;
}

// Create work items recursively
async function createWorkItem(baseURL, token, item, workspaceId, workspaceKey, itemMap, parentId = null, projectMap = {}, priorityMap = {}, itemTypes = {}, milestoneMap = {}, iterationMap = {}, statusMap = {}, fieldMap = {}, selectOptionMap = {}, depth = 0) {
  try {
    const indent = '  '.repeat(depth);

    // Resolve project ID if specified
    let projectId = null;
    if (item.project && projectMap[item.project]) {
      projectId = projectMap[item.project];
    }

    // Resolve priority ID if specified
    let priorityId = null;
    if (item.priority && priorityMap[item.priority]) {
      priorityId = priorityMap[item.priority];
    }

    // Resolve milestone IDs if specified (try workspace-specific first, then
    // global). v2 items link to planning via the milestone_ids array.
    let milestoneIds;
    if (item.milestoneName) {
      const localKey = `${workspaceKey}:${item.milestoneName}`;
      const milestoneId = milestoneMap[localKey] || milestoneMap[item.milestoneName];
      if (milestoneId) milestoneIds = [milestoneId];
    }

    // Resolve iteration ID if specified (try workspace-specific first, then global)
    let iterationId = null;
    if (item.iterationName) {
      const localKey = `${workspaceKey}:${item.iterationName}`;
      iterationId = iterationMap[localKey] || iterationMap[item.iterationName] || null;
    }

    // Determine item type based on depth and characteristics
    const itemTypeId = determineItemType(item, depth, itemTypes);

    const itemData = {
      workspace_id: workspaceId,
      parent_id: parentId,
      item_type_id: itemTypeId,
      title: item.title,
      description: item.description || '',
      status_id: item.status_name ? (statusMap[item.status_name] || null) : (item.status_id || null),
      is_task: item.is_task || false,
      project_id: projectId,
      priority_id: priorityId,
      iteration_id: iterationId,
      custom_field_values: mapCustomFieldValues(item.custom_fields, fieldMap, selectOptionMap)
    };
    if (milestoneIds) itemData.milestone_ids = milestoneIds;

    // The v2 API decodes due_date as time.Time; send RFC3339 rather than a
    // bare YYYY-MM-DD (which fails decode with a misleading "invalid JSON").
    if (item.due_date) {
      itemData.due_date = toISODate(item.due_date);
    }
    if (item.start_date) {
      itemData.start_date = toISODate(item.start_date);
    }

    const created = await v2(baseURL, token, 'POST', '/api/v2/items', itemData);
    const itemId = created.id;
    const icon = item.is_task ? '☐' : (item.children ? '📁' : '📄');
    const milestoneInfo = milestoneIds ? ` [M:${item.milestoneName}]` : '';
    const iterationInfo = iterationId ? ` [I:${item.iterationName}]` : '';
    logSuccess(`${indent}${icon} Created: ${item.title}${milestoneInfo}${iterationInfo}`);

    // Track this item in the map for work logs
    const key = `${workspaceKey}:${item.title}`;
    itemMap[key] = itemId;

    // Create children recursively
    if (item.children && item.children.length > 0) {
      for (const child of item.children) {
        await createWorkItem(baseURL, token, child, workspaceId, workspaceKey, itemMap, itemId, projectMap, priorityMap, itemTypes, milestoneMap, iterationMap, statusMap, fieldMap, selectOptionMap, depth + 1);
      }
    }

    return itemId;
  } catch (error) {
    const indent = '  '.repeat(depth);
    logError(`${indent}Error creating item "${item.title}": ${error.message}`);
    return null;
  }
}

// Create all work items for all workspaces
async function createWorkItems(baseURL, token, workspaceMap, projectMap, priorityMap, itemTypes, milestoneMap = {}, iterationMap = {}, statusMap = {}, fieldMap = {}, selectOptionMap = {}, workItemsData = workItems) {
  logSection('Creating Work Items');

  const itemMap = {};

  for (const [workspaceKey, items] of Object.entries(workItemsData)) {
    const workspaceId = workspaceMap[workspaceKey];
    if (!workspaceId) {
      logError(`Workspace ${workspaceKey} not found`);
      continue;
    }

    log(`\n${colors.bright}${workspaceKey}:${colors.reset}`);

    // Create project name to ID map for this workspace
    const wsProjectMap = {};
    for (const [key, projectId] of Object.entries(projectMap)) {
      if (key.startsWith(workspaceKey + ':')) {
        const projectName = key.substring(workspaceKey.length + 1);
        wsProjectMap[projectName] = projectId;
      }
    }

    for (const item of items) {
      await createWorkItem(baseURL, token, item, workspaceId, workspaceKey, itemMap, null, wsProjectMap, priorityMap, itemTypes, milestoneMap, iterationMap, statusMap, fieldMap, selectOptionMap, 0);
    }
  }

  return itemMap;
}

// Create test labels
async function createTestLabels(baseURL, token, workspaceId, labelsData = testLabels) {
  logSection('Creating Test Labels');

  const createdLabels = {};

  for (const label of labelsData) {
    try {
      const created = await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-labels`, label);
      createdLabels[label.name] = created.id;
      logSuccess(`Created test label: ${label.name}`);
    } catch (error) {
      logError(`Error creating label ${label.name}: ${error.message}`);
    }
  }

  return createdLabels;
}

// Create test folders recursively
async function createTestFolder(baseURL, token, workspaceId, folder, parentId = null, folderMap = {}, depth = 0) {
  const indent = '  '.repeat(depth);

  try {
    const created = await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-folders`, {
      name: folder.name,
      description: folder.description || '',
      parent_id: parentId
    });

    const folderId = created.id;
    logSuccess(`${indent}Created folder: ${folder.name}`);

    // Store folder path in map
    const folderPath = parentId
      ? `${Object.keys(folderMap).find(key => folderMap[key] === parentId)}/${folder.name}`
      : folder.name;
    folderMap[folderPath] = folderId;

    // Create children recursively
    if (folder.children && folder.children.length > 0) {
      for (const child of folder.children) {
        await createTestFolder(baseURL, token, workspaceId, child, folderId, folderMap, depth + 1);
      }
    }

    return folderId;
  } catch (error) {
    logError(`${indent}Error creating folder "${folder.name}": ${error.message}`);
    return null;
  }
}

// Create all test folders
async function createTestFolders(baseURL, token, workspaceId, foldersData = testFolders) {
  logSection('Creating Test Folders');

  const folderMap = {};

  for (const folder of foldersData) {
    await createTestFolder(baseURL, token, workspaceId, folder, null, folderMap, 0);
  }

  return folderMap;
}

// Create test cases with steps
async function createTestCases(baseURL, token, workspaceId, folderMap, labelMap, testCasesData = testCases) {
  logSection('Creating Test Cases');

  const testCaseMap = {};

  for (const testCase of testCasesData) {
    try {
      // Find folder ID from path
      const folderId = folderMap[testCase.folderPath];
      if (!folderId) {
        logError(`Folder not found for path: ${testCase.folderPath}`);
        continue;
      }

      // Create test case
      const created = await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-cases`, {
        title: testCase.title,
        preconditions: testCase.preconditions || '',
        folder_id: folderId
      });
      const testCaseId = created.id;
      const key = `${testCase.folderPath}:${testCase.title}`;
      testCaseMap[key] = testCaseId;
      logSuccess(`Created test case: ${testCase.title}`);

      // Create test steps
      if (testCase.steps && testCase.steps.length > 0) {
        for (let i = 0; i < testCase.steps.length; i++) {
          const step = testCase.steps[i];
          try {
            await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`, {
              action: step.action,
              data: step.data || '',
              expected: step.expected || ''
            });
          } catch (error) {
            logError(`  Failed to create step ${i + 1} for test case "${testCase.title}": ${error.message}`);
          }
        }
        logInfo(`  Added ${testCase.steps.length} steps`);
      }

      // Add labels to test case
      if (testCase.labels && testCase.labels.length > 0) {
        for (const labelName of testCase.labels) {
          const labelId = labelMap[labelName];
          if (labelId) {
            try {
              await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`, { label_id: labelId });
            } catch (error) {
              logError(`  Failed to add label "${labelName}" to test case "${testCase.title}": ${error.message}`);
            }
          }
        }
        logInfo(`  Added ${testCase.labels.length} labels`);
      }
    } catch (error) {
      logError(`Error creating test case "${testCase.title}": ${error.message}`);
    }
  }

  return testCaseMap;
}

// Create test plans (formerly test sets; sets were renamed to plans)
async function createTestSets(baseURL, token, workspaceId, milestoneMap, testCaseMap, labelMap) {
  logSection('Creating Test Plans');

  const testSetMap = {};

  for (const testSet of testSets) {
    try {
      // Resolve milestone ID
      let milestoneId = null;
      if (testSet.milestone && milestoneMap[testSet.milestone]) {
        milestoneId = milestoneMap[testSet.milestone];
      }

      const created = await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-plans`, {
        name: testSet.name,
        description: testSet.description,
        milestone_id: milestoneId
      });

      const testSetId = created.id;
      testSetMap[testSet.name] = testSetId;
      logSuccess(`Created test plan: ${testSet.name}`);

      // Add test cases to plan based on label filter
      let addedCount = 0;
      if (testSet.labelFilter) {
        // Find all test cases with this label
        for (const [key, testCaseId] of Object.entries(testCaseMap)) {
          // Find the original test case definition
          const originalTestCase = testCases.find(tc => {
            const tcKey = `${tc.folderPath}:${tc.title}`;
            return tcKey === key;
          });

          if (originalTestCase && originalTestCase.labels && originalTestCase.labels.includes(testSet.labelFilter)) {
            try {
              await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-plans/${testSetId}/test-cases`, { test_case_id: testCaseId });
              addedCount++;
            } catch (error) {
              logError(`  Failed to add test case to plan: ${error.message}`);
            }
          }
        }
        logInfo(`  Added ${addedCount} test cases with label "${testSet.labelFilter}"`);
      }
    } catch (error) {
      logError(`Error creating test plan "${testSet.name}": ${error.message}`);
    }
  }

  return testSetMap;
}

// Create test run templates
async function createTestRunTemplates(baseURL, token, workspaceId, testSetMap) {
  logSection('Creating Test Run Templates');

  const templateMap = {};

  for (const template of testRunTemplates) {
    try {
      // Find test plan ID (templates take plan_id since sets → plans)
      const testSetId = testSetMap[template.testSet];
      if (!testSetId) {
        logError(`Test set not found: ${template.testSet}`);
        continue;
      }

      const created = await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-run-templates`, {
        plan_id: testSetId,
        name: template.name,
        description: template.description
      });

      templateMap[template.name] = created.id;
      logSuccess(`Created test run template: ${template.name}`);
    } catch (error) {
      logError(`Error creating template "${template.name}": ${error.message}`);
    }
  }

  return templateMap;
}

// Record test run results. Result updates are merge-patch on v2, and the run
// only renders recorded results once it has been ended.
async function recordRunResults(baseURL, token, workspaceId, testRunId, results, outcomes) {
  let failedCount = 0;
  for (let i = 0; i < results.length; i++) {
    const status = outcomes(results[i], i, failedCount);
    if (status === 'failed') failedCount++;
    try {
      await v2Patch(baseURL, token, `/api/v2/workspaces/${workspaceId}/test-runs/${testRunId}/results/${results[i].id}`, {
        status,
        actual_result: status === 'passed'
          ? 'Test passed successfully'
          : 'Test failed - unexpected behavior detected'
      });
    } catch (error) {
      logError(`  Failed to record result for run: ${error.message}`);
    }
  }
  try {
    await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-runs/${testRunId}/end`, {});
  } catch (error) {
    logError(`  Failed to end run: ${error.message}`);
  }
  return failedCount;
}

// Execute test run templates and update results
async function executeTestRuns(baseURL, token, workspaceId, templateMap, itemMap) {
  logSection('Executing Test Runs');

  const testRunMap = {};

  const executeTemplate = async (templateName, runKey, outcomes) => {
    try {
      const run = await v2(baseURL, token, 'POST', `/api/v2/workspaces/${workspaceId}/test-run-templates/${templateMap[templateName]}/execute`, {});
      const testRunId = run.id;
      testRunMap[runKey] = testRunId;
      logSuccess(`Executed template: ${templateName}`);

      const results = await v2(baseURL, token, 'GET', `/api/v2/workspaces/${workspaceId}/test-runs/${testRunId}/results`);
      const failedCount = await recordRunResults(baseURL, token, workspaceId, testRunId, Array.isArray(results) ? results : [], outcomes);
      const passedCount = (Array.isArray(results) ? results.length : 0) - failedCount;
      logSuccess(`  ${passedCount} passed, ${failedCount} failed, run completed`);
    } catch (error) {
      logError(`Error executing ${templateName}: ${error.message}`);
    }
  };

  // Execute "Daily Smoke Tests" template - all passing
  if (templateMap['Daily Smoke Tests']) {
    await executeTemplate('Daily Smoke Tests', 'Daily Smoke Tests - 2025-01-15', () => 'passed');
  }

  // Execute "Weekly Regression" template - mostly passing with some failures
  if (templateMap['Weekly Regression']) {
    const maxFailures = 2;
    let failedCount = 0;
    await executeTemplate('Weekly Regression', 'Sprint Regression - 2025-01-18', () => {
      if (failedCount < maxFailures && Math.random() < 0.1) {
        failedCount++;
        return 'failed';
      }
      return 'passed';
    });
  }

  return testRunMap;
}

// Create links between test cases and requirements
async function createTestCaseLinks(baseURL, token, itemMap, testCaseMap, linkTypeMap = {}) {
  logSection('Creating Test Case to Requirement Links');

  const testsLinkTypeId = linkTypeMap['Tests'];
  if (!testsLinkTypeId) {
    logError('Tests link type not found - skipping test case requirement links');
    return 0;
  }

  let createdCount = 0;

  for (const link of testCaseLinks) {
    try {
      // Find test case ID
      const testCaseKey = `${link.testCasePath}:${link.testCaseTitle}`;
      const testCaseId = testCaseMap[testCaseKey];

      // Find requirement ID
      const requirementKey = `${link.requirementWorkspace}:${link.requirementTitle}`;
      const requirementId = itemMap[requirementKey];

      if (!testCaseId) {
        logError(`Test case not found: ${link.testCaseTitle}`);
        continue;
      }

      if (!requirementId) {
        logError(`Requirement not found: ${link.requirementTitle}`);
        continue;
      }

      // Create link: test_case (source) → item (target).
      // Resolve the system "Tests" link type via the catalog instead of
      // assuming a database ID; IDs can differ across existing/demo instances.
      await v2(baseURL, token, 'POST', '/api/v2/links', {
        link_type_id: testsLinkTypeId,
        source_type: "test_case",
        source_id: testCaseId,
        target_type: "item",
        target_id: requirementId
      });
      createdCount++;
      logSuccess(`Linked: "${link.testCaseTitle}" tests "${link.requirementTitle}"`);
    } catch (error) {
      logError(`Error creating link: ${error.message}`);
    }
  }

  return createdCount;
}

// Create asset management sets
async function createAssetSets(baseURL, token) {
  logSection('Creating Asset Management Sets');

  const createdSets = {};

  for (const set of assetSets) {
    try {
      const created = await v2(baseURL, token, 'POST', '/api/v2/asset-sets', set);
      createdSets[set.name] = created.id;
      logSuccess(`Created asset set: ${set.name}`);
    } catch (error) {
      logError(`Error creating asset set ${set.name}: ${error.message}`);
    }
  }

  return createdSets;
}

// Create asset types for each set
async function createAssetTypes(baseURL, token, setMap) {
  logSection('Creating Asset Types');

  const createdTypes = {};

  for (const [setName, types] of Object.entries(assetTypes)) {
    const setId = setMap[setName];
    if (!setId) {
      logError(`Asset set ${setName} not found`);
      continue;
    }

    for (const type of types) {
      try {
        const created = await v2(baseURL, token, 'POST', `/api/v2/asset-sets/${setId}/types`, type);
        const key = `${setName}:${type.name}`;
        createdTypes[key] = created.id;
        logSuccess(`Created asset type: ${type.name} (${setName})`);
      } catch (error) {
        logError(`Error creating asset type ${type.name}: ${error.message}`);
      }
    }
  }

  return createdTypes;
}

// Create asset categories recursively
async function createAssetCategory(baseURL, token, setId, setName, category, parentId = null, categoryMap = {}, parentPath = '', depth = 0) {
  const indent = '  '.repeat(depth);

  try {
    const created = await v2(baseURL, token, 'POST', `/api/v2/asset-sets/${setId}/categories`, {
      name: category.name,
      description: category.description || '',
      parent_id: parentId
    });

    const categoryId = created.id;
    const categoryPath = parentPath ? `${parentPath}/${category.name}` : category.name;
    const key = `${setName}:${categoryPath}`;
    categoryMap[key] = categoryId;
    logSuccess(`${indent}Created category: ${category.name}`);

    // Create children recursively
    if (category.children && category.children.length > 0) {
      for (const child of category.children) {
        await createAssetCategory(baseURL, token, setId, setName, child, categoryId, categoryMap, categoryPath, depth + 1);
      }
    }

    return categoryId;
  } catch (error) {
    logError(`${indent}Error creating category "${category.name}": ${error.message}`);
    return null;
  }
}

// Create all asset categories
async function createAssetCategories(baseURL, token, setMap) {
  logSection('Creating Asset Categories');

  const categoryMap = {};

  for (const [setName, categories] of Object.entries(assetCategories)) {
    const setId = setMap[setName];
    if (!setId) {
      logError(`Asset set ${setName} not found`);
      continue;
    }

    log(`\n${colors.bright}${setName}:${colors.reset}`);

    for (const category of categories) {
      await createAssetCategory(baseURL, token, setId, setName, category, null, categoryMap, '', 0);
    }
  }

  return categoryMap;
}

// Assign custom fields to asset types
async function createAssetTypeFields(baseURL, token, typeMap, fieldMap) {
  logSection('Assigning Custom Fields to Asset Types');

  for (const [setName, typeFields] of Object.entries(assetTypeFields)) {
    for (const [typeName, fields] of Object.entries(typeFields)) {
      const typeKey = `${setName}:${typeName}`;
      const typeId = typeMap[typeKey];
      if (!typeId) {
        logError(`Asset type ${typeName} not found in ${setName}`);
        continue;
      }

      const fieldData = fields.map((fieldName, i) => {
        const fieldId = fieldMap[fieldName];
        if (!fieldId) {
          logError(`Custom field ${fieldName} not found`);
          return null;
        }
        return {
          custom_field_id: fieldId,
          is_required: false,
          display_order: i + 1
        };
      }).filter(Boolean);

      if (fieldData.length === 0) continue;

      try {
        await v2(baseURL, token, 'PUT', `/api/v2/asset-types/${typeId}/fields`, { fields: fieldData });
        logSuccess(`Assigned ${fields.join(', ')} to ${typeName}`);
      } catch (error) {
        logError(`Error assigning fields to ${typeName}: ${error.message}`);
      }
    }
  }
}

// Create assets
async function createAssets(baseURL, token, setMap, typeMap, categoryMap, userMap = {}, fieldMap = {}, assetsData = assets) {
  logSection('Creating Assets');

  let createdCount = 0;

  for (const [setName, assetList] of Object.entries(assetsData)) {
    const setId = setMap[setName];
    if (!setId) {
      logError(`Asset set ${setName} not found`);
      continue;
    }

    log(`\n${colors.bright}${setName}:${colors.reset}`);

    for (const asset of assetList) {
      try {
        // Resolve type ID
        const typeKey = `${setName}:${asset.type}`;
        const typeId = typeMap[typeKey];
        if (!typeId) {
          logError(`Asset type ${asset.type} not found in ${setName}`);
          continue;
        }

        // Resolve category ID
        const categoryKey = `${setName}:${asset.category}`;
        const categoryId = categoryMap[categoryKey];
        if (!categoryId) {
          logError(`Asset category ${asset.category} not found in ${setName}`);
          continue;
        }

        const assetData = {
          title: asset.title,
          description: asset.description || '',
          asset_type_id: typeId,
          category_id: categoryId,
          asset_tag: asset.asset_tag || ''
        };

        // Add custom field values if asset has ownerUsername
        if (asset.ownerUsername && userMap[asset.ownerUsername]) {
          const ownerFieldId = fieldMap['Owner'];
          if (ownerFieldId) {
            const user = userMap[asset.ownerUsername];
            assetData.custom_field_values = {
              [String(ownerFieldId)]: user.id  // Just store the user ID, backend will enrich it
            };
          }
        }

        await v2(baseURL, token, 'POST', `/api/v2/asset-sets/${setId}/assets`, assetData);
        createdCount++;
        logSuccess(`Created asset: ${asset.title} (${asset.type})`);
      } catch (error) {
        logError(`Error creating asset ${asset.title}: ${error.message}`);
      }
    }
  }

  return createdCount;
}

// Get current user info, including personal workspace.
// /api/auth/me now returns { user, session } and no longer includes
// personal_workspace_id; we fetch it via GET /api/workspaces/personal.
async function getCurrentUser(baseURL, token) {
  try {
    const meResponse = await makeAuthRequest(baseURL, 'GET', '/api/auth/me', null, token);
    if (meResponse.status !== 200) {
      logError(`/api/auth/me status ${meResponse.status}: ${JSON.stringify(meResponse.data)}`);
      return null;
    }
    const user = meResponse.data.user ?? meResponse.data;
    // GET /api/workspaces/personal is "get-or-create": first call returns 201
    // (just created), subsequent calls return 200.
    const pwResponse = await makeAuthRequest(baseURL, 'GET', '/api/workspaces/personal', null, token);
    if ((pwResponse.status === 200 || pwResponse.status === 201) && pwResponse.data?.id) {
      user.personal_workspace_id = pwResponse.data.id;
    }
    return user;
  } catch (error) {
    logError(`Error getting current user: ${error.message}`);
    return null;
  }
}

// Create personal tasks for admin user
async function createPersonalTasks(baseURL, token, personalTasksData = personalTasks) {
  logSection('Creating Personal Tasks');

  // Get admin user info to get personal workspace ID
  const adminUser = await getCurrentUser(baseURL, token);
  if (!adminUser || !adminUser.personal_workspace_id) {
    logError('Could not get admin user personal workspace ID');
    return 0;
  }

  const personalWorkspaceId = adminUser.personal_workspace_id;
  const userId = adminUser.id;
  let createdCount = 0;
  let scheduledCount = 0;

  logInfo(`Using personal workspace ID: ${personalWorkspaceId}`);
  logInfo(`Week start (Monday): ${getRelativeDate(0)}`);

  for (const taskData of personalTasksData) {
    try {
      // Prepare item data
      const itemData = {
        workspace_id: personalWorkspaceId,
        title: taskData.title,
        description: taskData.description || '',
        is_task: true
      };

      // Add due date if specified. The v2 API decodes due_date as time.Time,
      // which expects RFC3339 — a bare YYYY-MM-DD is rejected with 400.
      if (taskData.dueDaysFromMonday !== undefined) {
        itemData.due_date = getRelativeDate(taskData.dueDaysFromMonday) + 'T00:00:00Z';
      }

      // Create the task item
      const created = await v2(baseURL, token, 'POST', '/api/v2/items', itemData);
      const itemId = created.id;
      createdCount++;
      logSuccess(`Created task: ${taskData.title}`);

      // Schedule on calendar if has scheduled time
      if (taskData.scheduledTime && taskData.daysFromMonday !== undefined) {
        const scheduleData = {
          user_id: userId,
          workspace_id: personalWorkspaceId,
          scheduled_date: getRelativeDate(taskData.daysFromMonday),
          scheduled_time: taskData.scheduledTime,
          duration_minutes: taskData.durationMinutes || 30
        };

        try {
          await makeAuthRequest(baseURL, 'POST', `/api/items/${itemId}/schedule`, scheduleData, token);
          scheduledCount++;
          logInfo(`  Scheduled for ${scheduleData.scheduled_date} at ${scheduleData.scheduled_time}`);
        } catch (error) {
          logError(`  Failed to schedule: ${error.message}`);
        }
      }
    } catch (error) {
      logError(`Error creating task "${taskData.title}": ${error.message}`);
    }
  }

  logSuccess(`Created ${createdCount} personal tasks (${scheduledCount} scheduled on calendar)`);
  return createdCount;
}

// Create comments on items (scale mode)
async function createComments(baseURL, token, itemMap, userMap, scaleModule) {
  logSection('Creating Comments (Scale)');

  const rng = scaleModule.createRNG(99999);
  const commentData = scaleModule.generateCommentsForItems(itemMap, userMap, rng);

  let createdCount = 0;
  let errorCount = 0;

  for (const { itemKey, comments } of commentData) {
    const itemId = itemMap[itemKey];
    if (!itemId) continue;

    for (const comment of comments) {
      try {
        // Comments are attributed to the authenticated principal on v2.
        await v2(baseURL, token, 'POST', `/api/v2/items/${itemId}/comments`, {
          content: comment.content,
          is_private: comment.is_private
        });
        createdCount++;
      } catch (err) {
        errorCount++;
      }

      if (createdCount > 0 && createdCount % 100 === 0) {
        logInfo(`  ...created ${createdCount} comments so far`);
      }
    }
  }

  if (errorCount > 0) {
    logError(`Failed to create ${errorCount} comments`);
  }
  logSuccess(`Created ${createdCount} comments on items`);
  return createdCount;
}

// Main execution
async function main() {
  const options = parseArgs();

  log(`${colors.bright}${colors.magenta}
╔═══════════════════════════════════════════════════════╗
║                                                       ║
║        ${APP_NAME} Demo Content Generator               ║
║                                                       ║
╚═══════════════════════════════════════════════════════╝
${colors.reset}`);

  // Conditionally load challenge data
  let challengeData = null;
  if (options.challenge) {
    challengeData = await import('./challenge-data.js');
    logInfo('Challenge mode enabled - including edge-case and security test data');
  }

  // Conditionally load scale data
  let scaleData = null;
  if (options.scale) {
    scaleData = await import('./scale-data.js');
    logInfo('Scale mode enabled - generating large-scale dataset (10,000+ items)');
  }

  // Helper to merge normal + challenge + scale data
  function getMergedData(normalData, challengeKey, scaleKey) {
    let result = normalData;

    // Merge challenge data
    if (challengeData && challengeData[challengeKey]) {
      if (Array.isArray(result)) {
        result = [...result, ...challengeData[challengeKey]];
      } else {
        const merged = { ...result };
        for (const [key, items] of Object.entries(challengeData[challengeKey])) {
          merged[key] = [...(merged[key] || []), ...items];
        }
        result = merged;
      }
    }

    // Merge scale data
    if (scaleData && scaleKey && scaleData[scaleKey]) {
      if (Array.isArray(result)) {
        result = [...result, ...scaleData[scaleKey]];
      } else {
        const merged = { ...result };
        for (const [key, items] of Object.entries(scaleData[scaleKey])) {
          merged[key] = [...(merged[key] || []), ...items];
        }
        result = merged;
      }
    }

    return result;
  }

  let serverProcess = null;

  try {
    // Clean database if requested (even when not starting server)
    if (options.cleanDb && !options.startServer) {
      logSection('Cleaning Database');
      cleanDatabase(options.db);
      logSuccess('Database cleaned');
    }

    // Start server if needed
    if (options.startServer) {
      if (options.build) {
        await buildApplication(options);
      } else {
        logInfo('Skipping frontend/backend build (--no-build)');
      }

      serverProcess = await startServer(options);
      // Wait a bit for migrations
      await new Promise(resolve => setTimeout(resolve, 2000));

      // Complete setup only for fresh local instances
      const setupSuccess = await completeSetup(options.baseURL, options);
      if (!setupSuccess) {
        throw new Error('Setup failed');
      }
    } else {
      logInfo(`Using existing server at ${options.baseURL}`);
      // A fresh database started externally (the Docker seeder does this)
      // still needs the admin user before anything can be seeded.
      const setupSuccess = await completeSetup(options.baseURL, options);
      if (!setupSuccess) {
        throw new Error('Setup failed');
      }
    }

    // Login once and use the session cookie for both API surfaces.
    const token = await getSessionCookie(options.baseURL, options);
    if (!token) {
      throw new Error('Failed to get session cookie');
    }

    // Create all demo content (merge with challenge data if enabled)
    const users = await createUsers(options.baseURL, token, getMergedData(demoUsers, 'challengeUsers', 'scaleUsers'));
    const workspaceMap = await createWorkspaces(options.baseURL, token, getMergedData(workspaces, 'challengeWorkspaces', 'scaleWorkspaces'));
    const customerMap = await createTimeCustomers(options.baseURL, token, getMergedData(timeCustomers, 'challengeTimeCustomers', 'scaleTimeCustomers'));
    const projectMap = await createProjects(options.baseURL, token, workspaceMap, customerMap, getMergedData(projects, 'challengeProjects', 'scaleProjects'));
    const [fieldMap, selectOptionMap] = await createCustomFields(options.baseURL, token);
    const screenMap = await createScreens(options.baseURL, token, fieldMap);
    const priorityMap = await createPriorities(options.baseURL, token);
    const categoryMap = await createMilestoneCategories(options.baseURL, token);
    const milestoneMap = await createMilestones(options.baseURL, token, workspaceMap, categoryMap, getMergedData(milestones, 'challengeMilestones', 'scaleMilestones'));
    const iterationTypeMap = await getIterationTypes(options.baseURL, token);
    const iterationMap = await createIterations(options.baseURL, token, workspaceMap, iterationTypeMap, getMergedData(iterations, 'challengeIterations', 'scaleIterations'));
    const itemTypes = await getItemTypes(options.baseURL, token);
    const statusMap = await getStatuses(options.baseURL, token);
    const linkTypeMap = await getLinkTypes(options.baseURL, token);
    const itemMap = await createWorkItems(options.baseURL, token, workspaceMap, projectMap, priorityMap, itemTypes, milestoneMap, iterationMap, statusMap, fieldMap, selectOptionMap, getMergedData(workItems, 'challengeWorkItems', 'scaleWorkItems'));
    const worklogCount = await createWorkLogs(options.baseURL, token, itemMap, projectMap);

    // Create comments (scale mode only)
    let commentCount = 0;
    if (options.scale && scaleData) {
      commentCount = await createComments(options.baseURL, token, itemMap, users, scaleData);
    }

    // Create test management data
    // Get Software Development workspace ID for test data (use key 'SOFT', not name)
    const softwareDevWorkspaceId = workspaceMap['SOFT'];
    let labelMap = {};
    let folderMap = {};
    let testCaseMap = {};
    let testSetMap = {};
    let templateMap = {};
    let testRunMap = {};
    let linkCount = 0;

    if (!softwareDevWorkspaceId) {
      logError('Software Development workspace not found - skipping test data creation');
    } else {
      labelMap = await createTestLabels(options.baseURL, token, softwareDevWorkspaceId, getMergedData(testLabels, 'challengeTestLabels'));
      folderMap = await createTestFolders(options.baseURL, token, softwareDevWorkspaceId, getMergedData(testFolders, 'challengeTestFolders'));
      testCaseMap = await createTestCases(options.baseURL, token, softwareDevWorkspaceId, folderMap, labelMap, getMergedData(testCases, 'challengeTestCases'));
      testSetMap = await createTestSets(options.baseURL, token, softwareDevWorkspaceId, milestoneMap, testCaseMap, labelMap);
      templateMap = await createTestRunTemplates(options.baseURL, token, softwareDevWorkspaceId, testSetMap);
      testRunMap = await executeTestRuns(options.baseURL, token, softwareDevWorkspaceId, templateMap, itemMap);
      linkCount = await createTestCaseLinks(options.baseURL, token, itemMap, testCaseMap, linkTypeMap);
    }

    // Asset Management
    const assetSetMap = await createAssetSets(options.baseURL, token);
    const assetTypeMap = await createAssetTypes(options.baseURL, token, assetSetMap);
    await createAssetTypeFields(options.baseURL, token, assetTypeMap, fieldMap);
    const assetCategoryMap = await createAssetCategories(options.baseURL, token, assetSetMap);
    const assetCount = await createAssets(options.baseURL, token, assetSetMap, assetTypeMap, assetCategoryMap, users, fieldMap, getMergedData(assets, 'challengeAssets'));

    // Personal Tasks for admin user (with calendar scheduling)
    const personalTaskCount = await createPersonalTasks(options.baseURL, token, getMergedData(personalTasks, 'challengePersonalTasks'));

    // Summary
    logSection('Summary');
    logSuccess(`Created ${Object.keys(users).length} users`);
    logSuccess(`Created ${Object.keys(workspaceMap).length} workspaces`);
    logSuccess(`Created ${Object.keys(projectMap).length} projects`);
    logSuccess(`Created ${Object.keys(fieldMap).length} custom fields`);
    logSuccess(`Created ${Object.keys(screenMap).length} screens`);
    logSuccess(`Created ${Object.keys(priorityMap).length} priorities`);
    logSuccess(`Created ${Object.keys(categoryMap).length} milestone categories`);
    logSuccess(`Created ${Object.keys(milestoneMap).length} milestones (global + local)`);
    logSuccess(`Created ${Object.keys(iterationMap).length} iterations (global + local)`);
    logSuccess(`Created ${Object.keys(customerMap).length} time customers`);

    const mergedWorkItems = getMergedData(workItems, 'challengeWorkItems', 'scaleWorkItems');
    let totalItems = 0;
    for (const items of Object.values(mergedWorkItems)) {
      const countItems = (itemList) => {
        let count = itemList.length;
        for (const item of itemList) {
          if (item.children) {
            count += countItems(item.children);
          }
        }
        return count;
      };
      totalItems += countItems(items);
    }
    logSuccess(`Created ${totalItems} work items`);
    logSuccess(`Created ${worklogCount} work logs`);
    if (commentCount > 0) {
      logSuccess(`Created ${commentCount} comments`);
    }
    logSuccess(`Created ${Object.keys(labelMap).length} test labels`);
    logSuccess(`Created ${Object.keys(folderMap).length} test folders`);
    logSuccess(`Created ${Object.keys(testCaseMap).length} test cases`);
    logSuccess(`Created ${Object.keys(testSetMap).length} test plans`);
    logSuccess(`Created ${Object.keys(templateMap).length} test run templates`);
    logSuccess(`Created ${Object.keys(testRunMap).length} test run executions`);
    logSuccess(`Created ${linkCount} test case to requirement links`);
    logSuccess(`Created ${Object.keys(assetSetMap).length} asset sets`);
    logSuccess(`Created ${Object.keys(assetTypeMap).length} asset types`);
    logSuccess(`Created ${Object.keys(assetCategoryMap).length} asset categories`);
    logSuccess(`Created ${assetCount} assets`);
    logSuccess(`Created ${personalTaskCount} personal tasks`);

    const modeText = options.scale ? ' (with scale data)' : (options.challenge ? ' (with challenge data)' : '');
    log(`\n${colors.bright}${colors.green}✓ Demo content generated successfully${modeText}!${colors.reset}\n`);
    logInfo(`Access the application at: ${options.baseURL}`);
    logInfo(`Login with: ${options.adminUser} / ${'*'.repeat(options.adminPassword.length)}`);

    if (serverProcess && options.keepServer) {
      log(`\n${colors.yellow}Server is still running. Press Ctrl+C to stop.${colors.reset}\n`);
      // Keep process alive
      await new Promise(() => { });
    }

  } catch (error) {
    logError(`\nFatal error: ${error.message}`);
    if (error.stack) {
      logInfo(error.stack);
    }
    process.exit(1);
  } finally {
    // Cleanup
    if (serverProcess && !options.keepServer) {
      logInfo('Stopping server...');
      serverProcess.kill();
      // Wait a bit for graceful shutdown
      await new Promise(resolve => setTimeout(resolve, 1000));
    }
  }
}

// Run if called directly
if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch(error => {
    logError(`Unhandled error: ${error.message}`);
    process.exit(1);
  });
}

// Export for use as module
export {
  makeRequest,
  buildApplication,
  completeSetup,
  getSessionCookie,
  getBearerToken,
  createUsers,
  createWorkspaces,
  createProjects,
  createCustomFields,
  createScreens,
  createPriorities,
  createMilestones,
  getIterationTypes,
  createIterations,
  getStatuses,
  getLinkTypes,
  createWorkItems,
  createComments,
  createPersonalTasks,
  createAssetSets,
  createAssetTypes,
  createAssetCategories,
  createAssets
};
