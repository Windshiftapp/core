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
 * Usage:
 *   node generate-demo.js [options]
 *
 * Options:
 *   --port <number>        Server port (default: 8080)
 *   --db <path>            Database file path (default: demo.db)
 *   --binary <path>        Path to server binary (default: ../../$BINARY_NAME or ../../windshift)
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

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

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
    binary: path.join(__dirname, '../..', BINARY_NAME),
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
  --binary <path>        Path to server binary (default: ../../${BINARY_NAME})
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

  ${colors.dim}# Connect to existing instance with custom credentials${colors.reset}
  node generate-demo.js --no-server --base-url https://example.com --admin-user myuser --admin-password mypass
`);
        process.exit(0);
      default:
        console.error(`${colors.red}Unknown option: ${args[i]}${colors.reset}`);
        process.exit(1);
    }
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
    logInfo(`Please build the binary first: go build -o ${BINARY_NAME}`);
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
    '--allowed-hosts', 'localhost,127.0.0.1'
  ], {
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: false,
    env: {
      ...process.env,
      SESSION_SECRET: sessionSecret
    }
  });

  // Log server errors to help debug startup issues
  serverProcess.stderr.on('data', (data) => {
    const msg = data.toString().trim();
    if (msg) logError(`Server: ${msg}`);
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

// Get CSRF token
async function getCSRFToken(baseURL, cookie = null) {
  const headers = {};
  if (cookie) {
    headers['Cookie'] = cookie;
  }

  const response = await makeRequest('GET', `${baseURL}/api/csrf-token`, null, headers);

  if (response.status !== 200) {
    throw new Error(`Failed to get CSRF token: ${response.status}`);
  }

  return response.data.csrf_token;
}

// Make authenticated request with bearer token
async function makeAuthRequest(baseURL, method, endpoint, data, token) {
  return makeRequest(method, `${baseURL}${endpoint}`, data, {
    'Authorization': `Bearer ${token}`
  });
}

// Complete initial setup
async function completeSetup(baseURL) {
  logSection('Completing Initial Setup');

  const setupData = {
    admin_user: {
      email: 'admin@demo.com',
      username: 'admin',
      password_hash: 'admin', // Will be hashed server-side
      first_name: 'Admin',
      last_name: 'User'
    },
    module_settings: {
      time_tracking_enabled: true,
      test_management_enabled: true
    }
  };

  try {
    // Get CSRF token for setup
    const csrfToken = await getCSRFToken(baseURL);

    const response = await makeRequest('POST', `${baseURL}/api/setup/complete`, setupData, {
      'X-CSRF-Token': csrfToken
    });

    if (response.status === 200 || response.status === 201) {
      logSuccess('Initial setup completed');
      return true;
    } else if (response.status === 400 && response.data.error?.includes('already completed')) {
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

// Get bearer token - following exact Go test pattern
async function getBearerToken(baseURL, options = {}) {
  logSection('Getting Bearer Token');

  const adminUser = options.adminUser || 'admin';
  const adminPassword = options.adminPassword || 'admin';

  try {
    // Step 1: Get CSRF token for login
    logInfo('Getting CSRF token for login...');
    const csrfToken1 = await getCSRFToken(baseURL);

    // Step 2: Login to get session cookie
    logInfo(`Logging in as ${adminUser}...`);
    const loginData = {
      email_or_username: adminUser,
      password: adminPassword
    };

    const loginResponse = await makeRequest('POST', `${baseURL}/api/auth/login`, loginData, {
      'X-CSRF-Token': csrfToken1
    });

    if (loginResponse.status !== 200) {
      logError(`Login failed: ${loginResponse.status} - ${JSON.stringify(loginResponse.data)}`);
      return null;
    }

    // Step 3: Extract session cookie
    const cookies = loginResponse.cookies;
    if (!cookies || cookies.length === 0) {
      logError('No session cookie received from login');
      return null;
    }

    // Find session cookie
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

    // Step 4: Get CSRF token with session cookie
    logInfo('Getting CSRF token with session...');
    const csrfToken2 = await getCSRFToken(baseURL, sessionCookie);

    // Step 5: Create bearer token with session cookie and CSRF token
    logInfo('Creating bearer token...');
    const tokenData = {
      name: 'Demo Generation Token',
      permissions: ['read', 'write', 'admin']
    };

    const tokenResponse = await makeRequest('POST', `${baseURL}/api/api-tokens`, tokenData, {
      'X-CSRF-Token': csrfToken2,
      'Cookie': sessionCookie
    });

    if (tokenResponse.status === 200 || tokenResponse.status === 201) {
      const token = tokenResponse.data.token;
      logSuccess('Bearer token created');
      return token;
    } else {
      logError(`Token creation failed: ${tokenResponse.status} - ${JSON.stringify(tokenResponse.data)}`);
      return null;
    }
  } catch (error) {
    logError(`Authentication error: ${error.message}`);
    if (error.stack) {
      logInfo(error.stack);
    }
    return null;
  }
}

// Create demo users
async function createUsers(baseURL, token, usersData = demoUsers) {
  logSection('Creating Demo Users');

  const createdUsers = {};

  for (const user of usersData) {
    try {
      const response = await makeAuthRequest(baseURL, 'POST', '/api/users', user, token);

      if (response.status === 200 || response.status === 201) {
        createdUsers[user.username] = {
          id: response.data.id,
          name: `${user.first_name} ${user.last_name}`.trim()
        };
        logSuccess(`Created user: ${user.first_name} ${user.last_name} (${user.role})`);
      } else {
        logError(`Failed to create user ${user.username}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating user ${user.username}: ${error.message}`);
    }
  }

  return createdUsers;
}

// Create workspaces
async function createWorkspaces(baseURL, token, workspacesData = workspaces) {
  logSection('Creating Workspaces');

  const createdWorkspaces = {};

  for (const workspace of workspacesData) {
    try {
      const response = await makeAuthRequest(baseURL, 'POST', '/api/workspaces', workspace, token);

      if (response.status === 200 || response.status === 201) {
        createdWorkspaces[workspace.key] = response.data.id;
        logSuccess(`Created workspace: ${workspace.name} (${workspace.key})`);
      } else {
        logError(`Failed to create workspace ${workspace.key}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating workspace ${workspace.key}: ${error.message}`);
    }
  }

  return createdWorkspaces;
}

// Create projects
async function createProjects(baseURL, token, workspaceMap, customerMap, projectsData = projects) {
  logSection('Creating Projects');

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
        description: project.description,
        active: project.active
      };

      const response = await makeAuthRequest(baseURL, 'POST', '/api/time/projects', projectData, token);

      if (response.status === 200 || response.status === 201) {
        const key = `${project.workspaceKey}:${project.name}`;
        createdProjects[key] = response.data.id;
        logSuccess(`Created project: ${project.name} for ${project.customerName}`);
      } else {
        logError(`Failed to create project ${project.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating project ${project.name}: ${error.message}`);
    }
  }

  return createdProjects;
}

// Create custom fields
async function createCustomFields(baseURL, token) {
  logSection('Creating Custom Fields');

  const createdFields = {};

  for (const field of customFields) {
    try {
      const response = await makeAuthRequest(baseURL, 'POST', '/api/custom-fields', field, token);

      if (response.status === 200 || response.status === 201) {
        createdFields[field.name] = response.data.id;
        logSuccess(`Created custom field: ${field.name} (${field.field_type})`);
      } else {
        logError(`Failed to create field ${field.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating field ${field.name}: ${error.message}`);
    }
  }

  return createdFields;
}

// Create screens
async function createScreens(baseURL, token, fieldMap) {
  logSection('Creating Screens');

  const createdScreens = {};

  for (const screen of screens) {
    try {
      const screenData = {
        name: screen.name,
        description: screen.description
      };

      const response = await makeAuthRequest(baseURL, 'POST', '/api/screens', screenData, token);

      if (response.status === 200 || response.status === 201) {
        createdScreens[screen.name] = response.data.id;
        logSuccess(`Created screen: ${screen.name}`);

        // Add fields to screen
        if (screen.fields && screen.fields.length > 0) {
          const fieldIds = screen.fields
            .map(fieldName => fieldMap[fieldName])
            .filter(id => id !== undefined);

          if (fieldIds.length > 0) {
            const fieldsData = fieldIds.map((fieldId, index) => ({
              custom_field_id: fieldId,
              display_order: index + 1,
              required: false
            }));

            const fieldsResponse = await makeAuthRequest(
              baseURL,
              'PUT',
              `/api/screens/${response.data.id}/fields`,
              { fields: fieldsData },
              token
            );

            if (fieldsResponse.status === 200) {
              logInfo(`  Added ${fieldIds.length} fields to screen`);
            }
          }
        }
      } else {
        logError(`Failed to create screen ${screen.name}: ${response.status}`);
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

  for (const priority of priorities) {
    try {
      const response = await makeAuthRequest(baseURL, 'POST', '/api/priorities', priority, token);

      if (response.status === 200 || response.status === 201) {
        createdPriorities[priority.name] = response.data.id;
        logSuccess(`Created priority: ${priority.name} ${priority.icon}`);
      } else if (response.status === 409) {
        // Priority already exists (from migrations) - fetch existing ID
        try {
          const allPriorities = await makeAuthRequest(baseURL, 'GET', '/api/priorities', null, token);
          const existingPriority = allPriorities.data.find(p => p.name === priority.name);
          if (existingPriority) {
            createdPriorities[priority.name] = existingPriority.id;
            logInfo(`Priority already exists: ${priority.name} ${priority.icon}`);
          }
        } catch (fetchError) {
          logError(`Failed to fetch existing priority ${priority.name}: ${fetchError.message}`);
        }
      } else {
        logError(`Failed to create priority ${priority.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating priority ${priority.name}: ${error.message}`);
    }
  }

  return createdPriorities;
}

// Create milestone categories (global)
async function createMilestoneCategories(baseURL, token) {
  logSection('Creating Milestone Categories');

  const categoryMap = {};

  for (const category of milestoneCategories) {
    try {
      const response = await makeAuthRequest(baseURL, 'POST', '/api/milestone-categories', {
        name: category.name,
        color: category.color,
        description: category.description
      }, token);

      if (response.status === 200 || response.status === 201) {
        categoryMap[category.name] = response.data.id;
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

// Create milestones (supports both global and local)
async function createMilestones(baseURL, token, workspaceMap, categoryMap = {}, milestonesData = milestones) {
  logSection('Creating Milestones');

  const createdMilestones = {};

  for (const milestone of milestonesData) {
    // Handle global vs local milestones
    let workspaceId = null;
    if (!milestone.is_global) {
      workspaceId = workspaceMap[milestone.workspaceKey];
      if (!workspaceId) {
        logError(`Workspace ${milestone.workspaceKey} not found for milestone ${milestone.name}`);
        continue;
      }
    }

    try {
      const targetDate = getRelativeDate(milestone.daysFromMonday);
      const milestoneData = {
        name: milestone.name,
        description: milestone.description,
        target_date: targetDate,
        status: milestone.status,
        is_global: milestone.is_global || false,
        workspace_id: workspaceId
      };

      // Add category_id if milestone has a categoryName and category exists
      if (milestone.categoryName && categoryMap[milestone.categoryName]) {
        milestoneData.category_id = categoryMap[milestone.categoryName];
      }

      const response = await makeAuthRequest(baseURL, 'POST', '/api/milestones', milestoneData, token);

      if (response.status === 200 || response.status === 201) {
        // Key format: global milestones use just name, local use workspace:name
        const key = milestone.is_global
          ? milestone.name
          : `${milestone.workspaceKey}:${milestone.name}`;
        createdMilestones[key] = response.data.id;
        const scope = milestone.is_global ? '(global)' : `(${milestone.workspaceKey})`;
        logSuccess(`Created milestone: ${milestone.name} ${scope} (${targetDate})`);
      } else {
        logError(`Failed to create milestone ${milestone.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating milestone ${milestone.name}: ${error.message}`);
    }
  }

  return createdMilestones;
}

// Create iterations (supports both global and local, with different types)
async function createIterations(baseURL, token, workspaceMap, iterationsData = iterations) {
  logSection('Creating Iterations');

  const createdIterations = {};

  // Map type names to IDs (pre-seeded in database)
  const typeIds = {
    'Sprint': 1,
    'PI / Quarter': 2,
    'Release': 3
  };

  for (const iteration of iterationsData) {
    // Handle global vs local iterations
    let workspaceId = null;
    if (!iteration.is_global) {
      workspaceId = workspaceMap[iteration.workspaceKey];
      if (!workspaceId) {
        logError(`Workspace ${iteration.workspaceKey} not found for iteration ${iteration.name}`);
        continue;
      }
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
        type_id: typeIds[iteration.type] || 1,
        is_global: iteration.is_global || false,
        workspace_id: workspaceId
      };

      const response = await makeAuthRequest(baseURL, 'POST', '/api/iterations', iterationData, token);

      if (response.status === 200 || response.status === 201) {
        // Key format: global iterations use just name, local use workspace:name
        const key = iteration.is_global
          ? iteration.name
          : `${iteration.workspaceKey}:${iteration.name}`;
        createdIterations[key] = response.data.id;
        const scope = iteration.is_global ? '(global)' : `(${iteration.workspaceKey})`;
        logSuccess(`Created iteration: ${iteration.name} [${iteration.type}] ${scope} (${startDate} - ${endDate})`);
      } else {
        logError(`Failed to create iteration ${iteration.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating iteration ${iteration.name}: ${error.message}`);
    }
  }

  return createdIterations;
}

// Get item types from the API
async function getItemTypes(baseURL, token) {
  try {
    const response = await makeAuthRequest(baseURL, 'GET', '/api/item-types', null, token);

    if (response.status === 200) {
      const itemTypes = {};
      for (const itemType of response.data) {
        itemTypes[itemType.name] = itemType.id;
      }
      return itemTypes;
    }

    logError('Failed to fetch item types');
    return {};
  } catch (error) {
    logError(`Error fetching item types: ${error.message}`);
    return {};
  }
}

// Determine appropriate item type based on item characteristics
function determineItemType(item, depth, itemTypes) {
  // Check if title suggests it's a bug
  const title = item.title.toLowerCase();
  const description = (item.description || '').toLowerCase();
  const isBugRelated = title.includes('bug') || title.includes('fix') ||
    description.includes('bug') || description.includes('defect');

  // Assign type based on hierarchy depth and characteristics
  if (isBugRelated && itemTypes['Bug']) {
    return itemTypes['Bug'];
  }

  if (depth === 0) {
    // Top-level items are Epics
    return itemTypes['Epic'] || null;
  } else if (depth === 1) {
    // First-level children are Stories
    return itemTypes['Story'] || null;
  } else if (depth === 2) {
    // Second-level children
    if (item.is_task) {
      return itemTypes['Task'] || null;
    }
    return itemTypes['Story'] || null;
  } else {
    // Deep nesting (depth 3+) - Sub-tasks
    return itemTypes['Sub-task'] || itemTypes['Task'] || null;
  }
}

// Create time tracking customers
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
        createdCustomers[customer.name] = response.data.id;
        logSuccess(`Created time customer: ${customer.name}`);
      } else {
        logError(`Failed to create customer ${customer.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating customer ${customer.name}: ${error.message}`);
    }
  }

  return createdCustomers;
}

// Create work logs for items
async function createWorkLogs(baseURL, token, itemMap, projectMap) {
  logSection('Creating Work Logs');

  let createdCount = 0;

  for (const log of workLogs) {
    // Find the item by title and workspace
    const itemKey = `${log.workspaceKey}:${log.itemTitle}`;
    const itemId = itemMap[itemKey];

    if (!itemId) {
      logError(`Item "${log.itemTitle}" not found in workspace ${log.workspaceKey}`);
      continue;
    }

    // Find the project ID
    const projectKey = `${log.workspaceKey}:${log.projectName}`;
    const projectId = projectMap[projectKey];

    if (!projectId) {
      logError(`Project "${log.projectName}" not found in workspace ${log.workspaceKey}`);
      continue;
    }

    try {
      const logData = {
        project_id: projectId,
        item_id: itemId,
        description: log.description,
        date: log.date,
        duration: log.duration,
        start_time: '',  // Let API calculate from duration
        end_time: ''
      };

      const response = await makeAuthRequest(baseURL, 'POST', '/api/time/worklogs', logData, token);

      if (response.status === 200 || response.status === 201) {
        createdCount++;
        logSuccess(`Created work log: ${log.duration} on "${log.itemTitle}" in ${log.projectName}`);
      } else {
        logError(`Failed to create work log for ${log.itemTitle}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating work log for ${log.itemTitle}: ${error.message}`);
    }
  }

  return createdCount;
}

// Create work items recursively
async function createWorkItem(baseURL, token, item, workspaceId, workspaceKey, itemMap, parentId = null, projectMap = {}, priorityMap = {}, itemTypes = {}, milestoneMap = {}, iterationMap = {}, depth = 0) {
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

    // Resolve milestone ID if specified (try workspace-specific first, then global)
    let milestoneId = null;
    if (item.milestoneName) {
      const localKey = `${workspaceKey}:${item.milestoneName}`;
      milestoneId = milestoneMap[localKey] || milestoneMap[item.milestoneName];
    }

    // Resolve iteration ID if specified (try workspace-specific first, then global)
    let iterationId = null;
    if (item.iterationName) {
      const localKey = `${workspaceKey}:${item.iterationName}`;
      iterationId = iterationMap[localKey] || iterationMap[item.iterationName];
    }

    // Determine item type based on depth and characteristics
    const itemTypeId = determineItemType(item, depth, itemTypes);

    const itemData = {
      workspace_id: workspaceId,
      parent_id: parentId,
      item_type_id: itemTypeId,
      title: item.title,
      description: item.description || '',
      status_id: item.status_id || 1, // Default to 1 (Open)
      is_task: item.is_task || false,
      project_id: projectId,
      priority_id: priorityId,
      milestone_id: milestoneId,
      iteration_id: iterationId,
      custom_field_values: item.custom_fields || {}
    };

    const response = await makeAuthRequest(baseURL, 'POST', '/api/items', itemData, token);

    if (response.status === 200 || response.status === 201) {
      const itemId = response.data.id;
      const icon = item.is_task ? '☐' : (item.children ? '📁' : '📄');
      const milestoneInfo = milestoneId ? ` [M:${item.milestoneName}]` : '';
      const iterationInfo = iterationId ? ` [I:${item.iterationName}]` : '';
      logSuccess(`${indent}${icon} Created: ${item.title}${milestoneInfo}${iterationInfo}`);

      // Track this item in the map for work logs
      const key = `${workspaceKey}:${item.title}`;
      itemMap[key] = itemId;

      // Create children recursively
      if (item.children && item.children.length > 0) {
        for (const child of item.children) {
          await createWorkItem(baseURL, token, child, workspaceId, workspaceKey, itemMap, itemId, projectMap, priorityMap, itemTypes, milestoneMap, iterationMap, depth + 1);
        }
      }

      return itemId;
    } else {
      logError(`${indent}Failed to create item "${item.title}": ${response.status}`);
      return null;
    }
  } catch (error) {
    logError(`Error creating item "${item.title}": ${error.message}`);
    return null;
  }
}

// Create all work items for all workspaces
async function createWorkItems(baseURL, token, workspaceMap, projectMap, priorityMap, itemTypes, milestoneMap = {}, iterationMap = {}, workItemsData = workItems) {
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
      await createWorkItem(baseURL, token, item, workspaceId, workspaceKey, itemMap, null, wsProjectMap, priorityMap, itemTypes, milestoneMap, iterationMap, 0);
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
      const response = await makeAuthRequest(baseURL, 'POST', `/api/workspaces/${workspaceId}/test-labels`, label, token);

      if (response.status === 200 || response.status === 201) {
        createdLabels[label.name] = response.data.id;
        logSuccess(`Created test label: ${label.name}`);
      } else {
        logError(`Failed to create label ${label.name}: ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating label ${label.name}: ${error.message}`);
    }
  }

  return createdLabels;
}

// Create test folders recursively
async function createTestFolder(baseURL, token, workspaceId, folder, parentId = null, folderMap = {}, depth = 0) {
  try {
    const indent = '  '.repeat(depth);

    const folderData = {
      name: folder.name,
      description: folder.description,
      parent_id: parentId
    };

    const response = await makeAuthRequest(baseURL, 'POST', `/api/workspaces/${workspaceId}/test-folders`, folderData, token);

    if (response.status === 200 || response.status === 201) {
      const folderId = response.data.id;
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
    } else {
      logError(`${indent}Failed to create folder "${folder.name}": ${response.status}`);
      return null;
    }
  } catch (error) {
    logError(`Error creating folder "${folder.name}": ${error.message}`);
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
      const testCaseData = {
        title: testCase.title,
        preconditions: testCase.preconditions,
        folder_id: folderId
      };

      const response = await makeAuthRequest(baseURL, 'POST', `/api/workspaces/${workspaceId}/test-cases`, testCaseData, token);

      if (response.status === 200 || response.status === 201) {
        const testCaseId = response.data.id;
        const key = `${testCase.folderPath}:${testCase.title}`;
        testCaseMap[key] = testCaseId;
        logSuccess(`Created test case: ${testCase.title}`);

        // Create test steps
        if (testCase.steps && testCase.steps.length > 0) {
          for (let i = 0; i < testCase.steps.length; i++) {
            const step = testCase.steps[i];
            const stepData = {
              step_number: i + 1,
              action: step.action,
              data: step.data,
              expected: step.expected
            };

            const stepResponse = await makeAuthRequest(
              baseURL,
              'POST',
              `/api/workspaces/${workspaceId}/test-cases/${testCaseId}/steps`,
              stepData,
              token
            );

            if (stepResponse.status !== 200 && stepResponse.status !== 201) {
              logError(`  Failed to create step ${i + 1} for test case "${testCase.title}"`);
            }
          }
          logInfo(`  Added ${testCase.steps.length} steps`);
        }

        // Add labels to test case
        if (testCase.labels && testCase.labels.length > 0) {
          for (const labelName of testCase.labels) {
            const labelId = labelMap[labelName];
            if (labelId) {
              const labelResponse = await makeAuthRequest(
                baseURL,
                'POST',
                `/api/workspaces/${workspaceId}/test-cases/${testCaseId}/labels`,
                { label_id: labelId },
                token
              );

              if (labelResponse.status !== 200 && labelResponse.status !== 201) {
                logError(`  Failed to add label "${labelName}" to test case "${testCase.title}"`);
              }
            }
          }
          logInfo(`  Added ${testCase.labels.length} labels`);
        }
      } else {
        logError(`Failed to create test case "${testCase.title}": ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating test case "${testCase.title}": ${error.message}`);
    }
  }

  return testCaseMap;
}

// Create test sets
async function createTestSets(baseURL, token, workspaceId, milestoneMap, testCaseMap, labelMap) {
  logSection('Creating Test Sets');

  const testSetMap = {};

  for (const testSet of testSets) {
    try {
      // Resolve milestone ID
      let milestoneId = null;
      if (testSet.milestone && milestoneMap[testSet.milestone]) {
        milestoneId = milestoneMap[testSet.milestone];
      }

      const testSetData = {
        name: testSet.name,
        description: testSet.description,
        milestone_id: milestoneId
      };

      const response = await makeAuthRequest(baseURL, 'POST', `/api/workspaces/${workspaceId}/test-sets`, testSetData, token);

      if (response.status === 200 || response.status === 201) {
        const testSetId = response.data.id;
        testSetMap[testSet.name] = testSetId;
        logSuccess(`Created test set: ${testSet.name}`);

        // Add test cases to set based on label filter
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
              // Add this test case to the set
              const addResponse = await makeAuthRequest(
                baseURL,
                'POST',
                `/api/workspaces/${workspaceId}/test-sets/${testSetId}/test-cases`,
                { test_case_id: testCaseId },
                token
              );

              if (addResponse.status === 200 || addResponse.status === 201) {
                addedCount++;
              }
            }
          }
          logInfo(`  Added ${addedCount} test cases with label "${testSet.labelFilter}"`);
        }
      } else {
        logError(`Failed to create test set "${testSet.name}": ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating test set "${testSet.name}": ${error.message}`);
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
      // Find test set ID
      const testSetId = testSetMap[template.testSet];
      if (!testSetId) {
        logError(`Test set not found: ${template.testSet}`);
        continue;
      }

      const templateData = {
        set_id: testSetId,
        name: template.name,
        description: template.description
      };

      const response = await makeAuthRequest(baseURL, 'POST', `/api/workspaces/${workspaceId}/test-run-templates`, templateData, token);

      if (response.status === 200 || response.status === 201) {
        templateMap[template.name] = response.data.id;
        logSuccess(`Created test run template: ${template.name}`);
      } else {
        logError(`Failed to create template "${template.name}": ${response.status}`);
      }
    } catch (error) {
      logError(`Error creating template "${template.name}": ${error.message}`);
    }
  }

  return templateMap;
}

// Execute test run templates and update results
async function executeTestRuns(baseURL, token, workspaceId, templateMap, itemMap) {
  logSection('Executing Test Runs');

  const testRunMap = {};

  // Execute "Daily Smoke Tests" template - all passing
  if (templateMap['Daily Smoke Tests']) {
    try {
      const executeResponse = await makeAuthRequest(
        baseURL,
        'POST',
        `/api/workspaces/${workspaceId}/test-run-templates/${templateMap['Daily Smoke Tests']}/execute`,
        {},
        token
      );

      if (executeResponse.status === 200 || executeResponse.status === 201) {
        const testRunId = executeResponse.data.id;
        testRunMap['Daily Smoke Tests - 2025-01-15'] = testRunId;
        logSuccess(`Executed template: Daily Smoke Tests`);

        // Get all test results for this run
        const resultsResponse = await makeAuthRequest(
          baseURL,
          'GET',
          `/api/workspaces/${workspaceId}/test-runs/${testRunId}/results`,
          null,
          token
        );

        if (resultsResponse.status === 200) {
          const results = resultsResponse.data;
          logInfo(`  Updating ${results.length} test results to "passed"`);

          // Mark all as passed
          for (const result of results) {
            await makeAuthRequest(
              baseURL,
              'PUT',
              `/api/workspaces/${workspaceId}/test-runs/${testRunId}/results/${result.id}`,
              {
                status: 'passed',
                actual_result: 'Test passed successfully',
                executed_at: '2025-01-15T10:00:00Z'
              },
              token
            );
          }

          // End the test run
          await makeAuthRequest(
            baseURL,
            'POST',
            `/api/workspaces/${workspaceId}/test-runs/${testRunId}/end`,
            {},
            token
          );

          logSuccess(`  All tests passed, run completed`);
        }
      }
    } catch (error) {
      logError(`Error executing Daily Smoke Tests: ${error.message}`);
    }
  }

  // Execute "Weekly Regression" template - mostly passing with some failures
  if (templateMap['Weekly Regression']) {
    try {
      const executeResponse = await makeAuthRequest(
        baseURL,
        'POST',
        `/api/workspaces/${workspaceId}/test-run-templates/${templateMap['Weekly Regression']}/execute`,
        {},
        token
      );

      if (executeResponse.status === 200 || executeResponse.status === 201) {
        const testRunId = executeResponse.data.id;
        testRunMap['Sprint Regression - 2025-01-18'] = testRunId;
        logSuccess(`Executed template: Weekly Regression`);

        // Get all test results for this run
        const resultsResponse = await makeAuthRequest(
          baseURL,
          'GET',
          `/api/workspaces/${workspaceId}/test-runs/${testRunId}/results`,
          null,
          token
        );

        if (resultsResponse.status === 200) {
          const results = resultsResponse.data;
          logInfo(`  Updating ${results.length} test results (mostly passed, 2 failed)`);

          let failedCount = 0;
          const maxFailures = 2;

          // Mark most as passed, a few as failed
          for (let i = 0; i < results.length; i++) {
            const result = results[i];
            const shouldFail = failedCount < maxFailures && Math.random() < 0.1;

            if (shouldFail) {
              failedCount++;
              await makeAuthRequest(
                baseURL,
                'PUT',
                `/api/workspaces/${workspaceId}/test-runs/${testRunId}/results/${result.id}`,
                {
                  status: 'failed',
                  actual_result: 'Test failed - unexpected behavior detected',
                  executed_at: '2025-01-18T14:30:00Z'
                },
                token
              );
            } else {
              await makeAuthRequest(
                baseURL,
                'PUT',
                `/api/workspaces/${workspaceId}/test-runs/${testRunId}/results/${result.id}`,
                {
                  status: 'passed',
                  actual_result: 'Test passed successfully',
                  executed_at: '2025-01-18T14:30:00Z'
                },
                token
              );
            }
          }

          // End the test run
          await makeAuthRequest(
            baseURL,
            'POST',
            `/api/workspaces/${workspaceId}/test-runs/${testRunId}/end`,
            {},
            token
          );

          const passedCount = results.length - failedCount;
          logSuccess(`  ${passedCount} passed, ${failedCount} failed, run completed`);
        }
      }
    } catch (error) {
      logError(`Error executing Weekly Regression: ${error.message}`);
    }
  }

  return testRunMap;
}

// Create links between test cases and requirements
async function createTestCaseLinks(baseURL, token, itemMap, testCaseMap) {
  logSection('Creating Test Case to Requirement Links');

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

      // Create link: test_case (source) → item (target)
      // This creates a "tests" relationship
      const linkData = {
        link_type_id: 1,  // "Tests" link type
        source_type: "test_case",
        source_id: testCaseId,
        target_type: "item",
        target_id: requirementId
      };

      const response = await makeAuthRequest(baseURL, 'POST', '/api/links', linkData, token);

      if (response.status === 200 || response.status === 201) {
        createdCount++;
        logSuccess(`Linked: "${link.testCaseTitle}" tests "${link.requirementTitle}"`);
      } else {
        logError(`Failed to link: ${response.status}`);
      }
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
      const response = await makeAuthRequest(baseURL, 'POST', '/api/asset-sets', set, token);

      if (response.status === 200 || response.status === 201) {
        createdSets[set.name] = response.data.id;
        logSuccess(`Created asset set: ${set.name}`);
      } else {
        logError(`Failed to create asset set ${set.name}: ${response.status}`);
      }
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
        const response = await makeAuthRequest(baseURL, 'POST', `/api/asset-sets/${setId}/types`, type, token);

        if (response.status === 200 || response.status === 201) {
          const key = `${setName}:${type.name}`;
          createdTypes[key] = response.data.id;
          logSuccess(`Created asset type: ${type.name} (${setName})`);
        } else {
          logError(`Failed to create asset type ${type.name}: ${response.status}`);
        }
      } catch (error) {
        logError(`Error creating asset type ${type.name}: ${error.message}`);
      }
    }
  }

  return createdTypes;
}

// Create asset categories recursively
async function createAssetCategory(baseURL, token, setId, setName, category, parentId = null, categoryMap = {}, parentPath = '', depth = 0) {
  try {
    const indent = '  '.repeat(depth);

    const categoryData = {
      name: category.name,
      description: category.description || '',
      parent_id: parentId
    };

    const response = await makeAuthRequest(baseURL, 'POST', `/api/asset-sets/${setId}/categories`, categoryData, token);

    if (response.status === 200 || response.status === 201) {
      const categoryId = response.data.id;
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
    } else {
      logError(`${indent}Failed to create category "${category.name}": ${response.status}`);
      return null;
    }
  } catch (error) {
    logError(`Error creating category "${category.name}": ${error.message}`);
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
        const response = await makeAuthRequest(baseURL, 'PUT', `/api/asset-types/${typeId}/fields`, { fields: fieldData }, token);

        if (response.status === 200 || response.status === 201) {
          logSuccess(`Assigned ${fields.join(', ')} to ${typeName}`);
        } else {
          logError(`Failed to assign fields to ${typeName}: ${response.status}`);
        }
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
              [ownerFieldId]: user.id  // Just store the user ID, backend will enrich it
            };
          }
        }

        const response = await makeAuthRequest(baseURL, 'POST', `/api/asset-sets/${setId}/assets`, assetData, token);

        if (response.status === 200 || response.status === 201) {
          createdCount++;
          logSuccess(`Created asset: ${asset.title} (${asset.type})`);
        } else {
          logError(`Failed to create asset ${asset.title}: ${response.status}`);
        }
      } catch (error) {
        logError(`Error creating asset ${asset.title}: ${error.message}`);
      }
    }
  }

  return createdCount;
}

// Get current user info (to get personal workspace ID)
async function getCurrentUser(baseURL, token) {
  try {
    const response = await makeAuthRequest(baseURL, 'GET', '/api/auth/me', null, token);
    if (response.status === 200) {
      return response.data;
    }
    return null;
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

      // Add due date if specified
      if (taskData.dueDaysFromMonday !== undefined) {
        itemData.due_date = getRelativeDate(taskData.dueDaysFromMonday);
      }

      // Create the task item
      const response = await makeAuthRequest(baseURL, 'POST', '/api/items', itemData, token);

      if (response.status === 200 || response.status === 201) {
        const itemId = response.data.id;
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

          const scheduleResponse = await makeAuthRequest(
            baseURL,
            'POST',
            `/api/items/${itemId}/schedule`,
            scheduleData,
            token
          );

          if (scheduleResponse.status === 200 || scheduleResponse.status === 201) {
            scheduledCount++;
            logInfo(`  Scheduled for ${scheduleData.scheduled_date} at ${scheduleData.scheduled_time}`);
          } else {
            logError(`  Failed to schedule: ${scheduleResponse.status}`);
          }
        }
      } else {
        logError(`Failed to create task "${taskData.title}": ${response.status}`);
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
      const userId = userMap[comment.username];
      if (!userId) continue;

      try {
        const response = await makeAuthRequest(baseURL, 'POST', `/api/items/${itemId}/comments`, {
          content: comment.content,
          author_id: userId,
          is_private: comment.is_private
        }, token);

        if (response.status >= 200 && response.status < 300) {
          createdCount++;
        } else {
          errorCount++;
        }
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
      serverProcess = await startServer(options);
      // Wait a bit for migrations
      await new Promise(resolve => setTimeout(resolve, 2000));

      // Complete setup only for fresh local instances
      const setupSuccess = await completeSetup(options.baseURL);
      if (!setupSuccess) {
        throw new Error('Setup failed');
      }
    } else {
      logInfo(`Using existing server at ${options.baseURL}`);
      logInfo('Skipping setup (assuming instance is already configured)');
    }

    // Get bearer token
    const token = await getBearerToken(options.baseURL, options);
    if (!token) {
      throw new Error('Failed to get bearer token');
    }

    // Create all demo content (merge with challenge data if enabled)
    const users = await createUsers(options.baseURL, token, getMergedData(demoUsers, 'challengeUsers', 'scaleUsers'));
    const workspaceMap = await createWorkspaces(options.baseURL, token, getMergedData(workspaces, 'challengeWorkspaces', 'scaleWorkspaces'));
    const customerMap = await createTimeCustomers(options.baseURL, token, getMergedData(timeCustomers, 'challengeTimeCustomers', 'scaleTimeCustomers'));
    const projectMap = await createProjects(options.baseURL, token, workspaceMap, customerMap, getMergedData(projects, 'challengeProjects', 'scaleProjects'));
    const fieldMap = await createCustomFields(options.baseURL, token);
    const screenMap = await createScreens(options.baseURL, token, fieldMap);
    const priorityMap = await createPriorities(options.baseURL, token);
    const categoryMap = await createMilestoneCategories(options.baseURL, token);
    const milestoneMap = await createMilestones(options.baseURL, token, workspaceMap, categoryMap, getMergedData(milestones, 'challengeMilestones', 'scaleMilestones'));
    const iterationMap = await createIterations(options.baseURL, token, workspaceMap, getMergedData(iterations, 'challengeIterations', 'scaleIterations'));
    const itemTypes = await getItemTypes(options.baseURL, token);
    const itemMap = await createWorkItems(options.baseURL, token, workspaceMap, projectMap, priorityMap, itemTypes, milestoneMap, iterationMap, getMergedData(workItems, 'challengeWorkItems', 'scaleWorkItems'));
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
      linkCount = await createTestCaseLinks(options.baseURL, token, itemMap, testCaseMap);
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
    logSuccess(`Created ${Object.keys(testSetMap).length} test sets`);
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
    logInfo(`Login with: ${options.adminUser} / ${options.adminPassword}`);

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
  completeSetup,
  getBearerToken,
  createUsers,
  createWorkspaces,
  createProjects,
  createCustomFields,
  createScreens,
  createPriorities,
  createMilestones,
  createIterations,
  createWorkItems,
  createComments,
  createPersonalTasks,
  createAssetSets,
  createAssetTypes,
  createAssetCategories,
  createAssets
};
