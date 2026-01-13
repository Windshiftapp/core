#!/usr/bin/env node

/**
 * Mixed Workload Load Test for Windshift Work Management
 *
 * Simulates realistic usage with mixed read/write/update operations:
 * - Creates items (writes)
 * - Lists/filters items (reads)
 * - Updates item status/priority (updates)
 */

const API_BASE = process.env.API_BASE || 'http://localhost:8080/api';
const BEARER_TOKEN = process.env.BEARER_TOKEN || 'crw_test1234567890abcdef1234567890abcdef';

// Configuration
const CONFIG = {
  duration: parseInt(process.env.DURATION || '60'), // Test duration in seconds
  concurrency: parseInt(process.env.CONCURRENCY || '50'),
  workspaces: parseInt(process.env.WORKSPACES || '10'),
  initialItems: parseInt(process.env.INITIAL_ITEMS || '100'), // Items to create before starting mixed load

  // Operation mix percentages (should add up to 100)
  createWeight: parseInt(process.env.CREATE_WEIGHT || '40'),
  readWeight: parseInt(process.env.READ_WEIGHT || '40'),
  updateWeight: parseInt(process.env.UPDATE_WEIGHT || '20'),

  retryAttempts: parseInt(process.env.RETRY_ATTEMPTS || '3'),
  requestTimeout: parseInt(process.env.REQUEST_TIMEOUT || '30000'),
};

// Statistics tracking by operation type
const stats = {
  operations: {
    create: { count: 0, errors: 0, responseTimes: [] },
    read: { count: 0, errors: 0, responseTimes: [] },
    update: { count: 0, errors: 0, responseTimes: [] },
  },
  totalOperations: 0,
  totalErrors: 0,
  startTime: null,
  endTime: null,
  workspaceIds: [],
  createdItemIds: [],
};

// Operation types weighted by configuration
const operationTypes = [];
for (let i = 0; i < CONFIG.createWeight; i++) operationTypes.push('create');
for (let i = 0; i < CONFIG.readWeight; i++) operationTypes.push('read');
for (let i = 0; i < CONFIG.updateWeight; i++) operationTypes.push('update');

/**
 * Make HTTP request with retry logic
 */
async function request(endpoint, options = {}, retryCount = 0) {
  const url = `${API_BASE}${endpoint}`;
  const startTime = Date.now();

  const defaultOptions = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${BEARER_TOKEN}`
    },
    signal: AbortSignal.timeout(CONFIG.requestTimeout)
  };

  try {
    const response = await fetch(url, { ...defaultOptions, ...options });
    const responseTime = Date.now() - startTime;

    let data = null;
    let rawText = null;
    const contentType = response.headers.get('content-type');

    if (contentType && contentType.includes('application/json') && response.status !== 204) {
      try {
        data = await response.json();
      } catch (jsonError) {
        rawText = await response.text();
      }
    } else if (!response.ok) {
      rawText = await response.text();
    }

    if (!response.ok) {
      const errorMsg = data ? JSON.stringify(data) : (rawText || 'No error message');
      throw new Error(`HTTP ${response.status}: ${errorMsg}`);
    }

    return {
      status: response.status,
      ok: response.ok,
      data,
      responseTime
    };
  } catch (error) {
    if (retryCount < CONFIG.retryAttempts) {
      await sleep(Math.pow(2, retryCount) * 100);
      return request(endpoint, options, retryCount + 1);
    }
    throw error;
  }
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Random helpers
 */
function randomElement(array) {
  return array[Math.floor(Math.random() * array.length)];
}

function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

/**
 * Operation: Create Item
 */
async function createItem() {
  const workspaceId = randomElement(stats.workspaceIds);
  const itemData = {
    title: `Test Item ${Date.now()}-${randomInt(1000, 9999)}`,
    description: `Auto-generated item for mixed load testing`,
    status: 'open',
    priority: randomElement(['low', 'medium', 'high']),
    workspace_id: workspaceId
  };

  const startTime = Date.now();
  try {
    const response = await request('/items', {
      method: 'POST',
      body: JSON.stringify(itemData)
    });

    const responseTime = Date.now() - startTime;
    stats.operations.create.count++;
    stats.operations.create.responseTimes.push(responseTime);

    // Track created item ID for updates
    if (response.data && response.data.id) {
      stats.createdItemIds.push(response.data.id);
    }

    return true;
  } catch (error) {
    stats.operations.create.errors++;
    if (stats.operations.create.errors <= 5) {
      console.error(`CREATE error: ${error.message}`);
    }
    return false;
  }
}

/**
 * Operation: Read Items
 */
async function readItems() {
  const workspaceId = randomElement(stats.workspaceIds);

  // Random read operation
  const readType = randomElement(['list', 'filter', 'get']);
  const startTime = Date.now();

  try {
    let response;

    if (readType === 'list') {
      // List items in workspace
      response = await request(`/items?workspace_id=${workspaceId}`);
    } else if (readType === 'filter') {
      // Filter by status or priority
      const filter = randomElement([
        `workspace_id=${workspaceId}&status=open`,
        `workspace_id=${workspaceId}&priority=high`,
        `workspace_id=${workspaceId}&status=in-progress`
      ]);
      response = await request(`/items?${filter}`);
    } else {
      // Get specific item
      if (stats.createdItemIds.length > 0) {
        const itemId = randomElement(stats.createdItemIds);
        response = await request(`/items/${itemId}`);
      } else {
        // Fall back to list if no items yet
        response = await request(`/items?workspace_id=${workspaceId}`);
      }
    }

    const responseTime = Date.now() - startTime;
    stats.operations.read.count++;
    stats.operations.read.responseTimes.push(responseTime);
    return true;
  } catch (error) {
    stats.operations.read.errors++;
    if (stats.operations.read.errors <= 5) {
      console.error(`READ error: ${error.message}`);
    }
    return false;
  }
}

/**
 * Operation: Update Item
 */
async function updateItem() {
  if (stats.createdItemIds.length === 0) {
    // No items to update yet, do a read instead
    return readItems();
  }

  const itemId = randomElement(stats.createdItemIds);
  const startTime = Date.now();

  try {
    // First get the item to get current data
    const getResponse = await request(`/items/${itemId}`);
    if (!getResponse.ok) {
      throw new Error('Failed to get item for update');
    }

    const item = getResponse.data;

    // Update with random changes
    const updateData = {
      ...item,
      status: randomElement(['open', 'in-progress', 'done']),
      priority: randomElement(['low', 'medium', 'high']),
    };

    const updateResponse = await request(`/items/${itemId}`, {
      method: 'PUT',
      body: JSON.stringify(updateData)
    });

    const responseTime = Date.now() - startTime;
    stats.operations.update.count++;
    stats.operations.update.responseTimes.push(responseTime);
    return true;
  } catch (error) {
    stats.operations.update.errors++;
    if (stats.operations.update.errors <= 5) {
      console.error(`UPDATE error: ${error.message}`);
    }
    return false;
  }
}

/**
 * Execute random operation based on weights
 */
async function executeRandomOperation() {
  const operation = randomElement(operationTypes);

  switch (operation) {
    case 'create':
      return createItem();
    case 'read':
      return readItems();
    case 'update':
      return updateItem();
    default:
      return false;
  }
}

/**
 * Worker that executes operations until test duration expires
 */
async function worker(workerId, stopTime) {
  while (Date.now() < stopTime) {
    await executeRandomOperation();
    stats.totalOperations++;

    // Update progress every 100 operations
    if (stats.totalOperations % 100 === 0) {
      displayProgress();
    }
  }
}

/**
 * Display progress
 */
function displayProgress() {
  const elapsed = Date.now() - stats.startTime;
  const opsPerSec = (stats.totalOperations / (elapsed / 1000)).toFixed(2);
  const remaining = Math.max(0, CONFIG.duration - Math.floor(elapsed / 1000));

  process.stdout.write(
    `\r📊 Ops: ${stats.totalOperations} | ` +
    `Speed: ${opsPerSec} ops/sec | ` +
    `Create: ${stats.operations.create.count} | ` +
    `Read: ${stats.operations.read.count} | ` +
    `Update: ${stats.operations.update.count} | ` +
    `Errors: ${stats.totalErrors} | ` +
    `Time: ${remaining}s remaining`
  );
}

/**
 * Calculate statistics for an operation type
 */
function calculateOpStats(opStats) {
  if (opStats.responseTimes.length === 0) {
    return { min: 0, max: 0, avg: 0, p95: 0 };
  }

  const sorted = [...opStats.responseTimes].sort((a, b) => a - b);
  const min = sorted[0];
  const max = sorted[sorted.length - 1];
  const avg = sorted.reduce((a, b) => a + b, 0) / sorted.length;
  const p95 = sorted[Math.floor(sorted.length * 0.95)];

  return { min, max, avg, p95 };
}

/**
 * Format duration
 */
function formatDuration(ms) {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  return minutes > 0 ? `${minutes}m ${seconds % 60}s` : `${seconds}s`;
}

/**
 * Display final report
 */
function displayReport() {
  console.log('\n\n' + '='.repeat(80));
  console.log('📊 MIXED LOAD TEST RESULTS');
  console.log('='.repeat(80));

  const duration = stats.endTime - stats.startTime;
  const throughput = (stats.totalOperations / (duration / 1000)).toFixed(2);

  console.log('\n📈 Overall Performance:');
  console.log(`   Total Duration:        ${formatDuration(duration)}`);
  console.log(`   Total Operations:      ${stats.totalOperations}`);
  console.log(`   Throughput:            ${throughput} operations/second`);
  console.log(`   Total Errors:          ${stats.totalErrors}`);
  console.log(`   Error Rate:            ${(stats.totalErrors / stats.totalOperations * 100).toFixed(2)}%`);

  // Create operations
  console.log('\n✏️  CREATE Operations:');
  const createStats = calculateOpStats(stats.operations.create);
  console.log(`   Count:                 ${stats.operations.create.count}`);
  console.log(`   Errors:                ${stats.operations.create.errors}`);
  console.log(`   Avg Response Time:     ${createStats.avg.toFixed(0)}ms`);
  console.log(`   Min Response Time:     ${createStats.min.toFixed(0)}ms`);
  console.log(`   Max Response Time:     ${createStats.max.toFixed(0)}ms`);
  console.log(`   95th Percentile:       ${createStats.p95.toFixed(0)}ms`);

  // Read operations
  console.log('\n📖 READ Operations:');
  const readStats = calculateOpStats(stats.operations.read);
  console.log(`   Count:                 ${stats.operations.read.count}`);
  console.log(`   Errors:                ${stats.operations.read.errors}`);
  console.log(`   Avg Response Time:     ${readStats.avg.toFixed(0)}ms`);
  console.log(`   Min Response Time:     ${readStats.min.toFixed(0)}ms`);
  console.log(`   Max Response Time:     ${readStats.max.toFixed(0)}ms`);
  console.log(`   95th Percentile:       ${readStats.p95.toFixed(0)}ms`);

  // Update operations
  console.log('\n🔄 UPDATE Operations:');
  const updateStats = calculateOpStats(stats.operations.update);
  console.log(`   Count:                 ${stats.operations.update.count}`);
  console.log(`   Errors:                ${stats.operations.update.errors}`);
  console.log(`   Avg Response Time:     ${updateStats.avg.toFixed(0)}ms`);
  console.log(`   Min Response Time:     ${updateStats.min.toFixed(0)}ms`);
  console.log(`   Max Response Time:     ${updateStats.max.toFixed(0)}ms`);
  console.log(`   95th Percentile:       ${updateStats.p95.toFixed(0)}ms`);

  console.log('\n⚙️  Configuration:');
  console.log(`   Target:                ${API_BASE}`);
  console.log(`   Duration:              ${CONFIG.duration} seconds`);
  console.log(`   Concurrency:           ${CONFIG.concurrency} workers`);
  console.log(`   Workspaces:            ${CONFIG.workspaces}`);
  console.log(`   Initial Items:         ${CONFIG.initialItems}`);
  console.log(`   Operation Mix:         Create ${CONFIG.createWeight}% / Read ${CONFIG.readWeight}% / Update ${CONFIG.updateWeight}%`);

  console.log('\n' + '='.repeat(80));
  console.log('✅ Test COMPLETED successfully!');
}

/**
 * Main test execution
 */
async function runMixedLoadTest() {
  console.log('🚀 Starting Mixed Workload Load Test');
  console.log('='.repeat(80));
  console.log(`Target:              ${API_BASE}`);
  console.log(`Duration:            ${CONFIG.duration} seconds`);
  console.log(`Concurrency:         ${CONFIG.concurrency} workers`);
  console.log(`Workspaces:          ${CONFIG.workspaces}`);
  console.log(`Initial Items:       ${CONFIG.initialItems}`);
  console.log(`Operation Mix:       Create ${CONFIG.createWeight}% / Read ${CONFIG.readWeight}% / Update ${CONFIG.updateWeight}%`);
  console.log('='.repeat(80));
  console.log('');

  try {
    // Phase 1: Create workspaces
    console.log('📦 Phase 1: Creating workspaces...');
    for (let i = 1; i <= CONFIG.workspaces; i++) {
      const randomSuffix = Math.random().toString(36).substring(2, 7).toUpperCase();
      const workspaceData = {
        name: `Load Test Workspace ${i}`,
        key: `ML${String(i).padStart(2, '0')}${randomSuffix}`, // e.g., "ML01ABCDE" = 9 chars
        description: `Workspace ${i} for mixed load testing`
      };

      const response = await request('/workspaces', {
        method: 'POST',
        body: JSON.stringify(workspaceData)
      });

      if (response.data && response.data.id) {
        stats.workspaceIds.push(response.data.id);
      }

      process.stdout.write(`\r   Created ${i}/${CONFIG.workspaces} workspaces...`);
    }
    console.log(`\n✅ Created ${stats.workspaceIds.length} workspaces\n`);

    // Phase 2: Create initial items
    console.log('📝 Phase 2: Creating initial items...');
    for (let i = 0; i < CONFIG.initialItems; i++) {
      await createItem();
      if ((i + 1) % 10 === 0) {
        process.stdout.write(`\r   Created ${i + 1}/${CONFIG.initialItems} items...`);
      }
    }
    console.log(`\n✅ Created ${CONFIG.initialItems} initial items\n`);

    // Phase 3: Run mixed workload
    console.log('🔥 Phase 3: Starting mixed workload...');
    stats.startTime = Date.now();
    const stopTime = stats.startTime + (CONFIG.duration * 1000);

    // Start workers
    const workers = [];
    for (let i = 0; i < CONFIG.concurrency; i++) {
      workers.push(worker(i, stopTime));
    }

    // Wait for all workers to complete
    await Promise.all(workers);

    stats.endTime = Date.now();
    stats.totalErrors = stats.operations.create.errors + stats.operations.read.errors + stats.operations.update.errors;

    console.log('\n\n✅ Mixed workload completed!\n');

  } catch (error) {
    console.error(`\n\n❌ Fatal error during load test: ${error.message}`);
    console.error(error.stack);
  } finally {
    displayReport();
  }
}

// Run the test
if (require.main === module) {
  runMixedLoadTest().catch(error => {
    console.error('Load test failed:', error);
    process.exit(1);
  });
}

module.exports = { runMixedLoadTest };
