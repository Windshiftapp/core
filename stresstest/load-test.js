#!/usr/bin/env node

/**
 * Load Testing Script for Windshift Work Management
 *
 * Generates 100,000 items across 100 workspaces to test system stability under load.
 * Uses concurrent async operations to stress test the server.
 */

const API_BASE = process.env.API_BASE || 'http://localhost:8080/api';
const BEARER_TOKEN = process.env.BEARER_TOKEN || 'crw_test1234567890abcdef1234567890abcdef';

// Configuration
const CONFIG = {
  totalWorkspaces: parseInt(process.env.WORKSPACES || '100'),
  itemsPerWorkspace: parseInt(process.env.ITEMS_PER_WORKSPACE || '1000'),
  concurrency: parseInt(process.env.CONCURRENCY || '50'),
  retryAttempts: parseInt(process.env.RETRY_ATTEMPTS || '3'),
  requestTimeout: parseInt(process.env.REQUEST_TIMEOUT || '30000'),
};

// Statistics tracking
const stats = {
  workspacesCreated: 0,
  itemsCreated: 0,
  errors: 0,
  retries: 0,
  responseTimes: [],
  startTime: null,
  endTime: null,
};

// Error tracking
const errorLog = [];

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
    stats.responseTimes.push(responseTime);

    let data = null;
    let rawText = null;
    const contentType = response.headers.get('content-type');

    if (contentType && contentType.includes('application/json') && response.status !== 204) {
      try {
        data = await response.json();
      } catch (jsonError) {
        // If JSON parsing fails, try to read as text
        rawText = await response.text();
      }
    } else if (!response.ok) {
      // For non-JSON error responses, capture the raw text
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
    // Retry logic
    if (retryCount < CONFIG.retryAttempts) {
      stats.retries++;
      await sleep(Math.pow(2, retryCount) * 100); // Exponential backoff
      return request(endpoint, options, retryCount + 1);
    }

    // Log error and rethrow
    const errorInfo = {
      endpoint,
      error: error.message,
      timestamp: new Date().toISOString(),
      retries: retryCount
    };
    errorLog.push(errorInfo);
    stats.errors++;

    // Print detailed error on first few failures to help debug
    if (stats.errors <= 5) {
      console.error(`\n⚠️  Error #${stats.errors}: ${error.message}`);
      console.error(`   Endpoint: ${endpoint}`);
      console.error(`   Retries: ${retryCount}/${CONFIG.retryAttempts}`);
    }

    throw error;
  }
}

/**
 * Sleep utility
 */
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Format time duration
 */
function formatDuration(ms) {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
  } else if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  } else {
    return `${seconds}s`;
  }
}

/**
 * Display progress
 */
function displayProgress() {
  const elapsed = Date.now() - stats.startTime;
  const totalItems = CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace;
  const progress = (stats.itemsCreated / totalItems * 100).toFixed(2);
  const itemsPerSec = (stats.itemsCreated / (elapsed / 1000)).toFixed(2);

  const avgResponseTime = stats.responseTimes.length > 0
    ? (stats.responseTimes.reduce((a, b) => a + b, 0) / stats.responseTimes.length).toFixed(0)
    : 0;

  process.stdout.write(
    `\r📊 Progress: ${progress}% | ` +
    `Items: ${stats.itemsCreated}/${totalItems} | ` +
    `Workspaces: ${stats.workspacesCreated}/${CONFIG.totalWorkspaces} | ` +
    `Speed: ${itemsPerSec} items/sec | ` +
    `Avg Response: ${avgResponseTime}ms | ` +
    `Errors: ${stats.errors} | ` +
    `Elapsed: ${formatDuration(elapsed)}`
  );
}

/**
 * Create a workspace
 */
async function createWorkspace(index) {
  // Generate short key (<= 10 chars): LT + index padded to 3 digits + random 5 chars
  const randomSuffix = Math.random().toString(36).substring(2, 7).toUpperCase();
  const workspaceData = {
    name: `Load Test Workspace ${index}`,
    key: `LT${String(index).padStart(2, '0')}${randomSuffix}`,
    description: `Auto-generated workspace for load testing`
  };

  const response = await request('/workspaces', {
    method: 'POST',
    body: JSON.stringify(workspaceData)
  });

  stats.workspacesCreated++;
  return response.data;
}

/**
 * Create a work item
 */
async function createItem(workspaceId, itemIndex) {
  const itemData = {
    title: `Load Test Item ${itemIndex}`,
    description: `Auto-generated item ${itemIndex} for load testing`,
    status: 'open',
    priority: 'medium',
    workspace_id: workspaceId
  };

  const response = await request('/items', {
    method: 'POST',
    body: JSON.stringify(itemData)
  });

  stats.itemsCreated++;
  return response.data;
}

/**
 * Create items in batches with controlled concurrency
 */
async function createItemsBatch(workspaceId, startIndex, count) {
  const promises = [];
  for (let i = 0; i < count; i++) {
    promises.push(createItem(workspaceId, startIndex + i));
  }
  return Promise.all(promises);
}

/**
 * Process items for a workspace with controlled concurrency
 */
async function processWorkspace(workspace, workspaceIndex) {
  const batchSize = CONFIG.concurrency;
  const totalItems = CONFIG.itemsPerWorkspace;
  const batches = Math.ceil(totalItems / batchSize);

  for (let batch = 0; batch < batches; batch++) {
    const startIndex = batch * batchSize;
    const itemsInBatch = Math.min(batchSize, totalItems - startIndex);

    try {
      await createItemsBatch(workspace.id, startIndex, itemsInBatch);
      displayProgress();
    } catch (error) {
      console.error(`\n❌ Error in batch ${batch} for workspace ${workspace.name}: ${error.message}`);
    }
  }
}

/**
 * Calculate statistics
 */
function calculateStats() {
  if (stats.responseTimes.length === 0) {
    return {
      min: 0,
      max: 0,
      avg: 0,
      median: 0,
      p95: 0,
      p99: 0
    };
  }

  const sorted = [...stats.responseTimes].sort((a, b) => a - b);
  const min = sorted[0];
  const max = sorted[sorted.length - 1];
  const avg = sorted.reduce((a, b) => a + b, 0) / sorted.length;
  const median = sorted[Math.floor(sorted.length / 2)];
  const p95 = sorted[Math.floor(sorted.length * 0.95)];
  const p99 = sorted[Math.floor(sorted.length * 0.99)];

  return { min, max, avg, median, p95, p99 };
}

/**
 * Display final report
 */
function displayReport() {
  console.log('\n\n' + '='.repeat(80));
  console.log('📊 LOAD TEST RESULTS');
  console.log('='.repeat(80));

  const duration = stats.endTime - stats.startTime;
  const totalItems = CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace;
  const throughput = (stats.itemsCreated / (duration / 1000)).toFixed(2);
  const responseStats = calculateStats();

  console.log('\n📈 Performance Metrics:');
  console.log(`   Total Duration:        ${formatDuration(duration)}`);
  console.log(`   Workspaces Created:    ${stats.workspacesCreated}/${CONFIG.totalWorkspaces}`);
  console.log(`   Items Created:         ${stats.itemsCreated}/${totalItems}`);
  console.log(`   Throughput:            ${throughput} items/second`);
  console.log(`   Total Requests:        ${stats.responseTimes.length}`);

  console.log('\n⏱️  Response Times (ms):');
  console.log(`   Min:                   ${responseStats.min.toFixed(0)}ms`);
  console.log(`   Max:                   ${responseStats.max.toFixed(0)}ms`);
  console.log(`   Average:               ${responseStats.avg.toFixed(0)}ms`);
  console.log(`   Median:                ${responseStats.median.toFixed(0)}ms`);
  console.log(`   95th Percentile:       ${responseStats.p95.toFixed(0)}ms`);
  console.log(`   99th Percentile:       ${responseStats.p99.toFixed(0)}ms`);

  console.log('\n❌ Errors:');
  console.log(`   Total Errors:          ${stats.errors}`);
  console.log(`   Total Retries:         ${stats.retries}`);
  console.log(`   Error Rate:            ${(stats.errors / stats.responseTimes.length * 100).toFixed(2)}%`);

  if (errorLog.length > 0) {
    console.log('\n🔍 Recent Errors (last 10):');
    errorLog.slice(-10).forEach((err, idx) => {
      console.log(`   ${idx + 1}. ${err.endpoint}: ${err.error}`);
    });
  }

  console.log('\n⚙️  Configuration:');
  console.log(`   Target:                ${API_BASE}`);
  console.log(`   Concurrency:           ${CONFIG.concurrency} requests`);
  console.log(`   Workspaces:            ${CONFIG.totalWorkspaces}`);
  console.log(`   Items per Workspace:   ${CONFIG.itemsPerWorkspace}`);
  console.log(`   Request Timeout:       ${CONFIG.requestTimeout}ms`);
  console.log(`   Retry Attempts:        ${CONFIG.retryAttempts}`);

  console.log('\n' + '='.repeat(80));

  // Exit with error code if significant failures
  if (stats.errors > totalItems * 0.01) { // More than 1% error rate
    console.log('❌ Test FAILED: Error rate exceeds 1%');
    process.exit(1);
  } else {
    console.log('✅ Test COMPLETED successfully!');
  }
}

/**
 * Main load test execution
 */
async function runLoadTest() {
  console.log('🚀 Starting Load Test');
  console.log('='.repeat(80));
  console.log(`Target:              ${API_BASE}`);
  console.log(`Workspaces:          ${CONFIG.totalWorkspaces}`);
  console.log(`Items per Workspace: ${CONFIG.itemsPerWorkspace}`);
  console.log(`Total Items:         ${CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace}`);
  console.log(`Concurrency:         ${CONFIG.concurrency} concurrent requests`);
  console.log('='.repeat(80));
  console.log('');

  stats.startTime = Date.now();

  try {
    // Phase 1: Create workspaces
    console.log('📦 Phase 1: Creating workspaces...');
    const workspaces = [];

    // Create workspaces in batches to avoid overwhelming the server
    const workspaceBatchSize = 10;
    for (let i = 0; i < CONFIG.totalWorkspaces; i += workspaceBatchSize) {
      const batchPromises = [];
      const batchCount = Math.min(workspaceBatchSize, CONFIG.totalWorkspaces - i);

      for (let j = 0; j < batchCount; j++) {
        batchPromises.push(createWorkspace(i + j + 1));
      }

      const batchResults = await Promise.all(batchPromises);
      workspaces.push(...batchResults);

      process.stdout.write(`\r   Created ${stats.workspacesCreated}/${CONFIG.totalWorkspaces} workspaces...`);
    }

    console.log(`\n✅ Created ${workspaces.length} workspaces\n`);

    // Phase 2: Create items across all workspaces (STRIPED for parallelism)
    console.log('📝 Phase 2: Creating items (striped across workspaces)...');

    // Build a queue of all items to create, striped across workspaces
    // This allows PostgreSQL's row-level locking to enable true parallelism
    const itemQueue = [];
    for (let itemIndex = 0; itemIndex < CONFIG.itemsPerWorkspace; itemIndex++) {
      for (let wsIndex = 0; wsIndex < workspaces.length; wsIndex++) {
        itemQueue.push({
          workspace: workspaces[wsIndex],
          itemIndex: itemIndex
        });
      }
    }

    // Process queue with controlled concurrency
    const concurrency = CONFIG.concurrency;
    let queueIndex = 0;

    async function processNext() {
      while (queueIndex < itemQueue.length) {
        const current = queueIndex++;
        if (current >= itemQueue.length) break;

        const { workspace, itemIndex } = itemQueue[current];
        try {
          await createItem(workspace.id, itemIndex);
          displayProgress();
        } catch (error) {
          // Error already logged in createItem
        }
      }
    }

    // Start concurrent workers
    const workers = [];
    for (let i = 0; i < concurrency; i++) {
      workers.push(processNext());
    }
    await Promise.all(workers);

    console.log('\n\n✅ Item creation completed!\n');

  } catch (error) {
    console.error(`\n\n❌ Fatal error during load test: ${error.message}`);
    console.error(error.stack);
  } finally {
    stats.endTime = Date.now();
    displayReport();
  }
}

// Run the load test
if (require.main === module) {
  runLoadTest().catch(error => {
    console.error('Load test failed:', error);
    process.exit(1);
  });
}

module.exports = { runLoadTest };
