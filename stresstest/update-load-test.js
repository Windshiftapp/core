#!/usr/bin/env node

/**
 * Update-Focused Load Testing Script for Windshift
 *
 * Tests the performance impact of history tracking on UPDATE operations.
 * Compares CREATE vs UPDATE throughput and measures history generation overhead.
 */

const API_BASE = process.env.API_BASE || 'http://localhost:8080/api';
const BEARER_TOKEN = process.env.BEARER_TOKEN || 'crw_test1234567890abcdef1234567890abcdef';

// Configuration
const CONFIG = {
  totalWorkspaces: parseInt(process.env.WORKSPACES || '10'),
  itemsPerWorkspace: parseInt(process.env.ITEMS_PER_WORKSPACE || '100'),
  updatesPerItem: parseInt(process.env.UPDATES_PER_ITEM || '5'),
  concurrency: parseInt(process.env.CONCURRENCY || '50'),
  retryAttempts: parseInt(process.env.RETRY_ATTEMPTS || '3'),
  requestTimeout: parseInt(process.env.REQUEST_TIMEOUT || '30000'),
};

// Statistics tracking - separated by operation type
const stats = {
  // Creation stats
  workspacesCreated: 0,
  itemsCreated: 0,
  createResponseTimes: [],
  createErrors: 0,

  // Update stats
  singleFieldUpdates: 0,
  multiFieldUpdates: 0,
  textFieldUpdates: 0,
  totalUpdates: 0,
  updateResponseTimes: [],
  updateErrors: 0,

  // General stats
  retries: 0,
  startTime: null,
  createEndTime: null,
  updateEndTime: null,
};

// Error tracking
const errorLog = [];

// Store created items for updates
const createdItems = [];

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
    // Retry logic
    if (retryCount < CONFIG.retryAttempts) {
      stats.retries++;
      await sleep(Math.pow(2, retryCount) * 100);
      return request(endpoint, options, retryCount + 1);
    }

    // Log error
    const errorInfo = {
      endpoint,
      error: error.message,
      timestamp: new Date().toISOString(),
      retries: retryCount
    };
    errorLog.push(errorInfo);

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

  if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  } else {
    return `${seconds}s`;
  }
}

/**
 * Display progress
 */
function displayProgress(phase, current, total, itemType) {
  const elapsed = Date.now() - stats.startTime;
  const progress = (current / total * 100).toFixed(2);
  const itemsPerSec = (current / (elapsed / 1000)).toFixed(2);

  let avgResponseTime = 0;
  if (phase === 'create' && stats.createResponseTimes.length > 0) {
    avgResponseTime = (stats.createResponseTimes.reduce((a, b) => a + b, 0) / stats.createResponseTimes.length).toFixed(0);
  } else if (phase === 'update' && stats.updateResponseTimes.length > 0) {
    avgResponseTime = (stats.updateResponseTimes.reduce((a, b) => a + b, 0) / stats.updateResponseTimes.length).toFixed(0);
  }

  const errorCount = phase === 'create' ? stats.createErrors : stats.updateErrors;

  process.stdout.write(
    `\r📊 ${phase.toUpperCase()}: ${progress}% | ` +
    `${itemType}: ${current}/${total} | ` +
    `Speed: ${itemsPerSec}/sec | ` +
    `Avg Response: ${avgResponseTime}ms | ` +
    `Errors: ${errorCount} | ` +
    `Elapsed: ${formatDuration(elapsed)}`
  );
}

/**
 * Create a workspace
 */
async function createWorkspace(index) {
  const randomSuffix = Math.random().toString(36).substring(2, 7).toUpperCase();
  const workspaceData = {
    name: `Update Test WS ${index}`,
    key: `UT${String(index).padStart(2, '0')}${randomSuffix}`,
    description: `Update test workspace ${index}`
  };

  const response = await request('/workspaces', {
    method: 'POST',
    body: JSON.stringify(workspaceData)
  });

  stats.createResponseTimes.push(response.responseTime);
  stats.workspacesCreated++;
  return response.data;
}

/**
 * Create a work item
 */
async function createItem(workspaceId, itemIndex) {
  const itemData = {
    title: `Update Test Item ${itemIndex}`,
    description: `Initial description for item ${itemIndex}`,
    status: 'open',
    priority: 'medium',
    workspace_id: workspaceId
  };

  const response = await request('/items', {
    method: 'POST',
    body: JSON.stringify(itemData)
  });

  stats.createResponseTimes.push(response.responseTime);
  stats.itemsCreated++;
  createdItems.push(response.data);
  return response.data;
}

/**
 * Update item - single field (status)
 */
async function updateItemStatus(item, newStatus) {
  const updateData = {
    status: newStatus
  };

  const response = await request(`/items/${item.id}`, {
    method: 'PUT',
    body: JSON.stringify(updateData)
  });

  stats.updateResponseTimes.push(response.responseTime);
  stats.singleFieldUpdates++;
  stats.totalUpdates++;
  return response.data;
}

/**
 * Update item - single field (priority)
 */
async function updateItemPriority(item, newPriority) {
  const updateData = {
    priority: newPriority
  };

  const response = await request(`/items/${item.id}`, {
    method: 'PUT',
    body: JSON.stringify(updateData)
  });

  stats.updateResponseTimes.push(response.responseTime);
  stats.singleFieldUpdates++;
  stats.totalUpdates++;
  return response.data;
}

/**
 * Update item - multiple fields (status + priority)
 */
async function updateItemMultiField(item, newStatus, newPriority) {
  const updateData = {
    status: newStatus,
    priority: newPriority
  };

  const response = await request(`/items/${item.id}`, {
    method: 'PUT',
    body: JSON.stringify(updateData)
  });

  stats.updateResponseTimes.push(response.responseTime);
  stats.multiFieldUpdates++;
  stats.totalUpdates++;
  return response.data;
}

/**
 * Update item - text field (description)
 */
async function updateItemDescription(item, iteration) {
  const updateData = {
    description: `Updated description iteration ${iteration}. This is a longer text to test string comparison performance in history tracking. Lorem ipsum dolor sit amet.`
  };

  const response = await request(`/items/${item.id}`, {
    method: 'PUT',
    body: JSON.stringify(updateData)
  });

  stats.updateResponseTimes.push(response.responseTime);
  stats.textFieldUpdates++;
  stats.totalUpdates++;
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
      displayProgress('create', stats.itemsCreated, CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace, 'Items');
    } catch (error) {
      console.error(`\n❌ Error in batch ${batch} for workspace ${workspace.name}: ${error.message}`);
      stats.createErrors++;
    }
  }
}

/**
 * Update items in batches
 */
async function updateItemsBatch(items, updateFn) {
  const batchSize = CONFIG.concurrency;
  const totalUpdates = items.length;

  for (let i = 0; i < items.length; i += batchSize) {
    const batch = items.slice(i, Math.min(i + batchSize, items.length));

    try {
      await Promise.all(batch.map(item => updateFn(item)));
    } catch (error) {
      console.error(`\n❌ Error updating items: ${error.message}`);
      stats.updateErrors++;
    }
  }
}

/**
 * Calculate statistics
 */
function calculateStats(responseTimes) {
  if (responseTimes.length === 0) {
    return { min: 0, max: 0, avg: 0, median: 0, p95: 0, p99: 0 };
  }

  const sorted = [...responseTimes].sort((a, b) => a - b);
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
  console.log('📊 UPDATE LOAD TEST RESULTS');
  console.log('='.repeat(80));

  const createDuration = stats.createEndTime - stats.startTime;
  const updateDuration = stats.updateEndTime - stats.createEndTime;
  const totalDuration = stats.updateEndTime - stats.startTime;

  const createThroughput = (stats.itemsCreated / (createDuration / 1000)).toFixed(2);
  const updateThroughput = (stats.totalUpdates / (updateDuration / 1000)).toFixed(2);

  const createStats = calculateStats(stats.createResponseTimes);
  const updateStats = calculateStats(stats.updateResponseTimes);

  console.log('\n📈 Performance Comparison:');
  console.log('─'.repeat(80));
  console.log('                    │  CREATE     │  UPDATE     │  DIFFERENCE');
  console.log('─'.repeat(80));
  console.log(`  Throughput        │  ${createThroughput.padStart(7)} /s │  ${updateThroughput.padStart(7)} /s │  ${((updateThroughput / createThroughput * 100) - 100).toFixed(1)}%`);
  console.log(`  Avg Response      │  ${createStats.avg.toFixed(0).padStart(7)} ms │  ${updateStats.avg.toFixed(0).padStart(7)} ms │  ${((updateStats.avg / createStats.avg * 100) - 100).toFixed(1)}%`);
  console.log(`  p95 Response      │  ${createStats.p95.toFixed(0).padStart(7)} ms │  ${updateStats.p95.toFixed(0).padStart(7)} ms │  ${((updateStats.p95 / createStats.p95 * 100) - 100).toFixed(1)}%`);
  console.log(`  p99 Response      │  ${createStats.p99.toFixed(0).padStart(7)} ms │  ${updateStats.p99.toFixed(0).padStart(7)} ms │  ${((updateStats.p99 / createStats.p99 * 100) - 100).toFixed(1)}%`);
  console.log('─'.repeat(80));

  console.log('\n📊 Detailed Metrics:');
  console.log(`   Create Phase:`);
  console.log(`     Duration:           ${formatDuration(createDuration)}`);
  console.log(`     Workspaces:         ${stats.workspacesCreated}/${CONFIG.totalWorkspaces}`);
  console.log(`     Items Created:      ${stats.itemsCreated}/${CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace}`);
  console.log(`     Throughput:         ${createThroughput} items/second`);
  console.log(`     Errors:             ${stats.createErrors}`);

  console.log(`\n   Update Phase:`);
  console.log(`     Duration:           ${formatDuration(updateDuration)}`);
  console.log(`     Single-field:       ${stats.singleFieldUpdates} updates`);
  console.log(`     Multi-field:        ${stats.multiFieldUpdates} updates`);
  console.log(`     Text-field:         ${stats.textFieldUpdates} updates`);
  console.log(`     Total Updates:      ${stats.totalUpdates}`);
  console.log(`     Throughput:         ${updateThroughput} updates/second`);
  console.log(`     Errors:             ${stats.updateErrors}`);

  console.log('\n⏱️  Create Response Times (ms):');
  console.log(`   Min:                   ${createStats.min.toFixed(0)}ms`);
  console.log(`   Max:                   ${createStats.max.toFixed(0)}ms`);
  console.log(`   Average:               ${createStats.avg.toFixed(0)}ms`);
  console.log(`   Median:                ${createStats.median.toFixed(0)}ms`);
  console.log(`   95th Percentile:       ${createStats.p95.toFixed(0)}ms`);
  console.log(`   99th Percentile:       ${createStats.p99.toFixed(0)}ms`);

  console.log('\n⏱️  Update Response Times (ms):');
  console.log(`   Min:                   ${updateStats.min.toFixed(0)}ms`);
  console.log(`   Max:                   ${updateStats.max.toFixed(0)}ms`);
  console.log(`   Average:               ${updateStats.avg.toFixed(0)}ms`);
  console.log(`   Median:                ${updateStats.median.toFixed(0)}ms`);
  console.log(`   95th Percentile:       ${updateStats.p95.toFixed(0)}ms`);
  console.log(`   99th Percentile:       ${updateStats.p99.toFixed(0)}ms`);

  console.log('\n📈 History Generation:');
  console.log(`   Estimated history records: ${stats.totalUpdates * 1.5} (avg 1.5 fields/update)`);

  console.log('\n❌ Errors:');
  console.log(`   Create Errors:         ${stats.createErrors}`);
  console.log(`   Update Errors:         ${stats.updateErrors}`);
  console.log(`   Total Retries:         ${stats.retries}`);
  console.log(`   Create Error Rate:     ${(stats.createErrors / stats.createResponseTimes.length * 100).toFixed(2)}%`);
  console.log(`   Update Error Rate:     ${(stats.updateErrors / stats.updateResponseTimes.length * 100).toFixed(2)}%`);

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
  console.log(`   Updates per Item:      ${CONFIG.updatesPerItem}`);
  console.log(`   Total Operations:      ${stats.itemsCreated + stats.totalUpdates}`);

  console.log('\n' + '='.repeat(80));

  // Exit with error code if significant failures
  const totalOps = stats.createResponseTimes.length + stats.updateResponseTimes.length;
  const totalErrors = stats.createErrors + stats.updateErrors;
  if (totalErrors > totalOps * 0.01) {
    console.log('❌ Test FAILED: Error rate exceeds 1%');
    process.exit(1);
  } else {
    console.log('✅ Test COMPLETED successfully!');
  }
}

/**
 * Main update load test execution
 */
async function runUpdateLoadTest() {
  console.log('🚀 Starting Update Load Test');
  console.log('='.repeat(80));
  console.log(`Target:              ${API_BASE}`);
  console.log(`Workspaces:          ${CONFIG.totalWorkspaces}`);
  console.log(`Items per Workspace: ${CONFIG.itemsPerWorkspace}`);
  console.log(`Total Items:         ${CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace}`);
  console.log(`Updates per Item:    ${CONFIG.updatesPerItem}`);
  console.log(`Total Updates:       ${CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace * CONFIG.updatesPerItem}`);
  console.log(`Concurrency:         ${CONFIG.concurrency} concurrent requests`);
  console.log('='.repeat(80));
  console.log('');

  stats.startTime = Date.now();

  try {
    // Phase 1: Create workspaces
    console.log('📦 Phase 1: Creating workspaces...');
    const workspaces = [];
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

    // Phase 2: Create items
    console.log('📝 Phase 2: Creating items...');
    for (let i = 0; i < workspaces.length; i++) {
      await processWorkspace(workspaces[i], i);
    }

    stats.createEndTime = Date.now();
    console.log('\n✅ Item creation completed!\n');

    // Phase 3: Single-field updates
    console.log('🔄 Phase 3: Single-field updates (status)...');
    let updateCount = 0;
    await updateItemsBatch(createdItems, async (item) => {
      await updateItemStatus(item, 'in-progress');
      updateCount++;
      if (updateCount % 50 === 0) {
        displayProgress('update', stats.totalUpdates, CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace * CONFIG.updatesPerItem, 'Updates');
      }
    });
    console.log('\n✅ Status updates completed!\n');

    // Phase 4: Single-field updates (priority)
    console.log('🔄 Phase 4: Single-field updates (priority)...');
    await updateItemsBatch(createdItems, async (item) => {
      await updateItemPriority(item, 'high');
      updateCount++;
      if (updateCount % 50 === 0) {
        displayProgress('update', stats.totalUpdates, CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace * CONFIG.updatesPerItem, 'Updates');
      }
    });
    console.log('\n✅ Priority updates completed!\n');

    // Phase 5: Multi-field updates
    console.log('🔄 Phase 5: Multi-field updates (status + priority)...');
    await updateItemsBatch(createdItems, async (item) => {
      await updateItemMultiField(item, 'done', 'low');
      updateCount++;
      if (updateCount % 50 === 0) {
        displayProgress('update', stats.totalUpdates, CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace * CONFIG.updatesPerItem, 'Updates');
      }
    });
    console.log('\n✅ Multi-field updates completed!\n');

    // Phase 6: Text field updates
    console.log('🔄 Phase 6: Text field updates (description)...');
    await updateItemsBatch(createdItems, async (item) => {
      await updateItemDescription(item, 1);
      updateCount++;
      if (updateCount % 50 === 0) {
        displayProgress('update', stats.totalUpdates, CONFIG.totalWorkspaces * CONFIG.itemsPerWorkspace * CONFIG.updatesPerItem, 'Updates');
      }
    });
    console.log('\n✅ Text field updates completed!\n');

    stats.updateEndTime = Date.now();

  } catch (error) {
    console.error(`\n\n❌ Fatal error during update load test: ${error.message}`);
    console.error(error.stack);
  } finally {
    displayReport();
  }
}

// Run the update load test
if (require.main === module) {
  runUpdateLoadTest().catch(error => {
    console.error('Update load test failed:', error);
    process.exit(1);
  });
}

module.exports = { runUpdateLoadTest };
