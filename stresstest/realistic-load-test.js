#!/usr/bin/env node

/**
 * Realistic Load Test for Windshift Work Management
 *
 * Simulates realistic user behavior with:
 * - User personas (Viewer, Editor, Power User)
 * - Think time between operations
 * - Session affinity (users stick to workspaces)
 * - Zipf distribution (hot/cold data)
 * - Realistic browse patterns
 * - History tracking reads
 *
 * Used for capacity testing to find maximum concurrent users for SQLite.
 */

const API_BASE = process.env.API_BASE || 'http://localhost:8080/api';
const BEARER_TOKEN = process.env.BEARER_TOKEN || 'crw_test1234567890abcdef1234567890abcdef';

// Configuration
const CONFIG = {
  // Concurrency can be increased dynamically during ramp-up
  startConcurrency: parseInt(process.env.START_CONCURRENCY || '10'),
  currentConcurrency: parseInt(process.env.START_CONCURRENCY || '10'),
  maxConcurrency: parseInt(process.env.MAX_CONCURRENCY || '500'),

  // Test duration
  testDuration: parseInt(process.env.TEST_DURATION || '300'), // 5 minutes default
  rampInterval: parseInt(process.env.RAMP_INTERVAL || '30'), // Increase every 30s
  rampIncrement: parseInt(process.env.RAMP_INCREMENT || '10'), // Add 10 users each step

  // User persona distribution
  viewerPercent: 30,
  editorPercent: 50,
  powerUserPercent: 20,

  // Behavior settings
  thinkTimeMin: parseInt(process.env.THINK_TIME_MIN || '200'), // ms
  thinkTimeMax: parseInt(process.env.THINK_TIME_MAX || '2000'), // ms
  sessionLength: [8, 12], // Operations before switching workspace

  // Hot data (Zipf distribution)
  hotDataPercent: 20, // 20% of items
  hotTrafficPercent: 80, // Receive 80% of reads

  // Failure thresholds
  maxErrorRate: parseFloat(process.env.MAX_ERROR_RATE || '0.01'), // 1%
  maxP95Latency: parseInt(process.env.MAX_P95_LATENCY || '500'), // ms
  maxThroughputDrop: parseFloat(process.env.MAX_THROUGHPUT_DROP || '0.30'), // 30%

  retryAttempts: 3,
  requestTimeout: 30000,
};

// User persona definitions
const USER_PERSONAS = {
  viewer: {
    name: 'Viewer',
    operations: {
      read: 85,
      update: 10,
      create: 5,
    }
  },
  editor: {
    name: 'Editor',
    operations: {
      read: 55,
      update: 30,
      create: 15,
    }
  },
  powerUser: {
    name: 'Power User',
    operations: {
      read: 40,
      update: 35,
      create: 25,
    }
  }
};

// Global state
const state = {
  workspaceIds: [],
  allItemIds: [],
  hotItemIds: [], // 20% of items that are "hot"
  coldItemIds: [], // 80% of items
  startTime: null,
  stopTime: null,
  shouldStop: false,
  stopReason: null,

  // Statistics by persona
  stats: {
    viewer: createPersonaStats(),
    editor: createPersonaStats(),
    powerUser: createPersonaStats(),
  },

  // Overall statistics
  overall: {
    totalOperations: 0,
    totalErrors: 0,
    operationTimes: [],
    concurrencyHistory: [], // Track concurrency over time
    throughputHistory: [], // Track throughput over time
    p95History: [], // Track p95 latency over time
  },

  // Detailed operation breakdown
  operationBreakdown: {
    read: {
      list: { count: 0, errors: 0, times: [] },
      filter: { count: 0, errors: 0, times: [] },
      detail: { count: 0, errors: 0, times: [] },
      history: { count: 0, errors: 0, times: [] },
      children: { count: 0, errors: 0, times: [] },
    },
    update: { count: 0, errors: 0, times: [] },
    create: { count: 0, errors: 0, times: [] },
  },

  // Active workers
  workers: [],
};

function createPersonaStats() {
  return {
    operations: { read: 0, update: 0, create: 0 },
    errors: { read: 0, update: 0, create: 0 },
    responseTimes: { read: [], update: [], create: [] },
  };
}

/**
 * HTTP request with retry logic
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
    const contentType = response.headers.get('content-type');

    if (contentType && contentType.includes('application/json') && response.status !== 204) {
      try {
        data = await response.json();
      } catch (jsonError) {
        // Ignore JSON parse errors
      }
    }

    if (!response.ok) {
      const errorMsg = data ? JSON.stringify(data) : await response.text();
      throw new Error(`HTTP ${response.status}: ${errorMsg}`);
    }

    return { status: response.status, ok: response.ok, data, responseTime };
  } catch (error) {
    if (retryCount < CONFIG.retryAttempts && !error.message.includes('AbortError')) {
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
 * Random utilities
 */
function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function randomElement(array) {
  return array[Math.floor(Math.random() * array.length)];
}

function randomThinkTime() {
  return randomInt(CONFIG.thinkTimeMin, CONFIG.thinkTimeMax);
}

/**
 * Zipf distribution for hot/cold data selection
 * Returns item from hot set 80% of time, cold set 20% of time
 */
function selectItemWithZipf() {
  if (state.hotItemIds.length === 0) {
    return randomElement(state.allItemIds);
  }

  const useHot = Math.random() < (CONFIG.hotTrafficPercent / 100);
  return useHot
    ? randomElement(state.hotItemIds)
    : randomElement(state.coldItemIds.length > 0 ? state.coldItemIds : state.allItemIds);
}

/**
 * Track operation stats
 */
function recordOperation(persona, opType, responseTime, isError = false) {
  const personaStats = state.stats[persona];

  personaStats.operations[opType]++;
  personaStats.responseTimes[opType].push(responseTime);

  if (isError) {
    personaStats.errors[opType]++;
    state.overall.totalErrors++;
  }

  state.overall.totalOperations++;
  state.overall.operationTimes.push(responseTime);
}

/**
 * Operations
 */

// Read operations with realistic patterns
async function performRead(persona, currentWorkspace, sessionContext) {
  const readTypes = ['list', 'filter', 'detail', 'history', 'children'];
  const readType = randomElement(readTypes);

  const startTime = Date.now();
  try {
    let response;

    if (readType === 'list') {
      // List items in workspace
      response = await request(`/items?workspace_id=${currentWorkspace}`);
      sessionContext.lastListedItems = response.data || [];
    } else if (readType === 'filter') {
      // Filter by status or priority
      const filters = [
        `workspace_id=${currentWorkspace}&status=open`,
        `workspace_id=${currentWorkspace}&status=in-progress`,
        `workspace_id=${currentWorkspace}&priority=high`,
        `workspace_id=${currentWorkspace}&priority=medium`,
      ];
      response = await request(`/items?${randomElement(filters)}`);
    } else if (readType === 'detail') {
      // Get item detail (prefer from session context if available)
      const itemId = sessionContext.lastListedItems && sessionContext.lastListedItems.length > 0
        ? randomElement(sessionContext.lastListedItems).id
        : selectItemWithZipf();

      if (itemId) {
        response = await request(`/items/${itemId}`);
        sessionContext.currentItem = response.data;
      }
    } else if (readType === 'history') {
      // Read item history (NEW FEATURE TEST)
      const itemId = sessionContext.currentItem?.id || selectItemWithZipf();

      if (itemId) {
        response = await request(`/items/${itemId}/history`);
      }
    } else if (readType === 'children') {
      // Get children/descendants (tests hierarchy)
      const itemId = sessionContext.currentItem?.id || selectItemWithZipf();

      if (itemId) {
        const endpoint = Math.random() < 0.5 ? 'children' : 'descendants';
        response = await request(`/items/${itemId}/${endpoint}`);
      }
    }

    const responseTime = Date.now() - startTime;
    recordOperation(persona, 'read', responseTime, false);

    // Track detailed breakdown
    state.operationBreakdown.read[readType].count++;
    state.operationBreakdown.read[readType].times.push(responseTime);

    return true;
  } catch (error) {
    const responseTime = Date.now() - startTime;
    recordOperation(persona, 'read', responseTime, true);

    // Track detailed breakdown for errors
    state.operationBreakdown.read[readType].errors++;

    // Log first 5 read errors for debugging
    if (state.stats[persona].errors.read <= 5) {
      console.error(`\n[${persona}] READ (${readType}) error: ${error.message}`);
    }
    return false;
  }
}

// Update operation
async function performUpdate(persona, currentWorkspace, sessionContext) {
  const startTime = Date.now();

  try {
    // Get item to update (prefer from context)
    let itemId = sessionContext.currentItem?.id;

    if (!itemId) {
      itemId = selectItemWithZipf();
    }

    if (!itemId) {
      // No items to update, skip
      return false;
    }

    // Fetch current item
    const getResponse = await request(`/items/${itemId}`);
    if (!getResponse.ok) {
      throw new Error('Failed to fetch item');
    }

    const item = getResponse.data;

    // Realistic update: only change 1-2 fields
    const updateType = randomElement(['status', 'priority', 'both', 'description']);
    const updateData = { ...item };

    if (updateType === 'status' || updateType === 'both') {
      updateData.status = randomElement(['open', 'in-progress', 'done']);
    }
    if (updateType === 'priority' || updateType === 'both') {
      updateData.priority = randomElement(['low', 'medium', 'high']);
    }
    if (updateType === 'description') {
      updateData.description = `Updated by ${persona} at ${new Date().toISOString()}`;
    }

    const updateResponse = await request(`/items/${itemId}`, {
      method: 'PUT',
      body: JSON.stringify(updateData)
    });

    const responseTime = Date.now() - startTime;
    recordOperation(persona, 'update', responseTime, false);

    // Track detailed breakdown
    state.operationBreakdown.update.count++;
    state.operationBreakdown.update.times.push(responseTime);

    return true;
  } catch (error) {
    const responseTime = Date.now() - startTime;
    recordOperation(persona, 'update', responseTime, true);

    // Track detailed breakdown for errors
    state.operationBreakdown.update.errors++;

    // Log first 5 update errors for debugging
    if (state.stats[persona].errors.update <= 5) {
      console.error(`\n[${persona}] UPDATE error: ${error.message}`);
    }
    return false;
  }
}

// Create operation
async function performCreate(persona, currentWorkspace) {
  const startTime = Date.now();

  try {
    const itemData = {
      title: `Item by ${persona} ${Date.now()}-${randomInt(1000, 9999)}`,
      description: `Created by ${persona} user at ${new Date().toISOString()}`,
      status: randomElement(['open', 'in-progress']),
      priority: randomElement(['low', 'medium', 'high']),
      workspace_id: currentWorkspace
    };

    const response = await request('/items', {
      method: 'POST',
      body: JSON.stringify(itemData)
    });

    // Track new item
    if (response.data && response.data.id) {
      state.allItemIds.push(response.data.id);

      // 20% chance to become a "hot" item
      if (Math.random() < 0.2) {
        state.hotItemIds.push(response.data.id);
      } else {
        state.coldItemIds.push(response.data.id);
      }
    }

    const responseTime = Date.now() - startTime;
    recordOperation(persona, 'create', responseTime, false);

    // Track detailed breakdown
    state.operationBreakdown.create.count++;
    state.operationBreakdown.create.times.push(responseTime);

    return true;
  } catch (error) {
    const responseTime = Date.now() - startTime;
    recordOperation(persona, 'create', responseTime, true);

    // Track detailed breakdown for errors
    state.operationBreakdown.create.errors++;

    // Log first 5 create errors for debugging
    if (state.stats[persona].errors.create <= 5) {
      console.error(`\n[${persona}] CREATE error: ${error.message}`);
    }
    return false;
  }
}

/**
 * User simulation worker
 * Each worker represents one user with a specific persona
 */
async function userWorker(workerId, persona) {
  const personaConfig = USER_PERSONAS[persona];

  // Build weighted operation array
  const operations = [];
  for (let i = 0; i < personaConfig.operations.read; i++) operations.push('read');
  for (let i = 0; i < personaConfig.operations.update; i++) operations.push('update');
  for (let i = 0; i < personaConfig.operations.create; i++) operations.push('create');

  // Session state
  let currentWorkspace = randomElement(state.workspaceIds);
  let sessionOpsRemaining = randomInt(...CONFIG.sessionLength);
  const sessionContext = {
    lastListedItems: [],
    currentItem: null,
  };

  while (Date.now() < state.stopTime && !state.shouldStop) {
    // Session affinity: switch workspace after session ends
    if (sessionOpsRemaining <= 0) {
      currentWorkspace = randomElement(state.workspaceIds);
      sessionOpsRemaining = randomInt(...CONFIG.sessionLength);
      sessionContext.lastListedItems = [];
      sessionContext.currentItem = null;
    }

    // Perform random operation based on persona weights
    const operation = randomElement(operations);

    if (operation === 'read') {
      await performRead(persona, currentWorkspace, sessionContext);
    } else if (operation === 'update') {
      await performUpdate(persona, currentWorkspace, sessionContext);
    } else if (operation === 'create') {
      await performCreate(persona, currentWorkspace);
    }

    sessionOpsRemaining--;

    // Think time (realistic pause between operations)
    await sleep(randomThinkTime());
  }
}

/**
 * Statistics calculation
 */
function calculateStats(times) {
  if (times.length === 0) {
    return { min: 0, max: 0, avg: 0, median: 0, p95: 0, p99: 0 };
  }

  const sorted = [...times].sort((a, b) => a - b);
  return {
    min: sorted[0],
    max: sorted[sorted.length - 1],
    avg: sorted.reduce((a, b) => a + b, 0) / sorted.length,
    median: sorted[Math.floor(sorted.length / 2)],
    p95: sorted[Math.floor(sorted.length * 0.95)] || sorted[sorted.length - 1],
    p99: sorted[Math.floor(sorted.length * 0.99)] || sorted[sorted.length - 1],
  };
}

/**
 * Monitor performance and check failure criteria
 */
function checkFailureCriteria() {
  const elapsed = Date.now() - state.startTime;

  // Need at least 100 operations for meaningful statistics
  if (state.overall.totalOperations < 100) {
    return { failed: false };
  }

  const stats = calculateStats(state.overall.operationTimes);
  const errorRate = state.overall.totalErrors / state.overall.totalOperations;
  const throughput = state.overall.totalOperations / (elapsed / 1000);

  // Check error rate
  if (errorRate > CONFIG.maxErrorRate) {
    return {
      failed: true,
      reason: `Error rate ${(errorRate * 100).toFixed(2)}% exceeds ${(CONFIG.maxErrorRate * 100).toFixed(2)}% threshold`,
      concurrency: CONFIG.currentConcurrency
    };
  }

  // Check p95 latency
  if (stats.p95 > CONFIG.maxP95Latency) {
    return {
      failed: true,
      reason: `p95 latency ${stats.p95.toFixed(0)}ms exceeds ${CONFIG.maxP95Latency}ms threshold`,
      concurrency: CONFIG.currentConcurrency
    };
  }

  // Check throughput degradation (need baseline)
  if (state.overall.throughputHistory.length > 0) {
    const baselineThroughput = state.overall.throughputHistory[0];
    const degradation = 1 - (throughput / baselineThroughput);

    if (degradation > CONFIG.maxThroughputDrop) {
      return {
        failed: true,
        reason: `Throughput degradation ${(degradation * 100).toFixed(1)}% exceeds ${(CONFIG.maxThroughputDrop * 100).toFixed(1)}% threshold`,
        concurrency: CONFIG.currentConcurrency
      };
    }
  }

  // Record current metrics
  state.overall.concurrencyHistory.push(CONFIG.currentConcurrency);
  state.overall.throughputHistory.push(throughput);
  state.overall.p95History.push(stats.p95);

  return { failed: false, stats, throughput, errorRate };
}

/**
 * Progress display
 */
function displayProgress() {
  const elapsed = Date.now() - state.startTime;
  const opsPerSec = (state.overall.totalOperations / (elapsed / 1000)).toFixed(2);
  const errorRate = ((state.overall.totalErrors / state.overall.totalOperations) * 100).toFixed(2);
  const stats = calculateStats(state.overall.operationTimes);

  process.stdout.write(
    `\r📊 Users: ${CONFIG.currentConcurrency} | ` +
    `Ops: ${state.overall.totalOperations} | ` +
    `Speed: ${opsPerSec}/s | ` +
    `p95: ${stats.p95.toFixed(0)}ms | ` +
    `Errors: ${errorRate}% | ` +
    `Time: ${Math.floor(elapsed / 1000)}s`
  );
}

/**
 * Report generation
 */
function displayReport() {
  console.log('\n\n' + '='.repeat(80));
  console.log('📊 REALISTIC LOAD TEST RESULTS');
  console.log('='.repeat(80));

  const duration = (state.stopTime || Date.now()) - state.startTime;
  const overallStats = calculateStats(state.overall.operationTimes);
  const throughput = (state.overall.totalOperations / (duration / 1000)).toFixed(2);
  const errorRate = ((state.overall.totalErrors / state.overall.totalOperations) * 100).toFixed(2);

  console.log('\n📈 Overall Performance:');
  console.log(`   Test Duration:         ${Math.floor(duration / 1000)}s`);
  console.log(`   Total Operations:      ${state.overall.totalOperations}`);
  console.log(`   Throughput:            ${throughput} ops/second`);
  console.log(`   Total Errors:          ${state.overall.totalErrors}`);
  console.log(`   Error Rate:            ${errorRate}%`);
  console.log(`   Max Concurrency:       ${CONFIG.currentConcurrency} users`);

  console.log('\n⏱️  Response Times (overall):');
  console.log(`   Min:                   ${overallStats.min.toFixed(0)}ms`);
  console.log(`   Max:                   ${overallStats.max.toFixed(0)}ms`);
  console.log(`   Average:               ${overallStats.avg.toFixed(0)}ms`);
  console.log(`   Median:                ${overallStats.median.toFixed(0)}ms`);
  console.log(`   95th Percentile:       ${overallStats.p95.toFixed(0)}ms`);
  console.log(`   99th Percentile:       ${overallStats.p99.toFixed(0)}ms`);

  // Persona breakdown
  console.log('\n👥 Performance by User Persona:');

  for (const [personaKey, personaStats] of Object.entries(state.stats)) {
    const persona = USER_PERSONAS[personaKey];
    const totalOps = personaStats.operations.read + personaStats.operations.update + personaStats.operations.create;
    const totalErrors = personaStats.errors.read + personaStats.errors.update + personaStats.errors.create;

    if (totalOps === 0) continue;

    console.log(`\n   ${persona.name}:`);
    console.log(`     Operations:          ${totalOps}`);
    console.log(`     Errors:              ${totalErrors} (${((totalErrors / totalOps) * 100).toFixed(2)}%)`);

    if (personaStats.responseTimes.read.length > 0) {
      const readStats = calculateStats(personaStats.responseTimes.read);
      console.log(`     READ - Count: ${personaStats.operations.read}, Avg: ${readStats.avg.toFixed(0)}ms, p95: ${readStats.p95.toFixed(0)}ms`);
    }

    if (personaStats.responseTimes.update.length > 0) {
      const updateStats = calculateStats(personaStats.responseTimes.update);
      console.log(`     UPDATE - Count: ${personaStats.operations.update}, Avg: ${updateStats.avg.toFixed(0)}ms, p95: ${updateStats.p95.toFixed(0)}ms`);
    }

    if (personaStats.responseTimes.create.length > 0) {
      const createStats = calculateStats(personaStats.responseTimes.create);
      console.log(`     CREATE - Count: ${personaStats.operations.create}, Avg: ${createStats.avg.toFixed(0)}ms, p95: ${createStats.p95.toFixed(0)}ms`);
    }
  }

  // Detailed operation breakdown
  console.log('\n📋 Detailed Operation Breakdown:');

  // READ operation breakdown
  console.log('\n   READ Operations by Type:');
  for (const [readType, readData] of Object.entries(state.operationBreakdown.read)) {
    if (readData.count > 0) {
      const stats = calculateStats(readData.times);
      const errorRate = readData.errors > 0 ? ((readData.errors / readData.count) * 100).toFixed(2) + '%' : '0%';
      console.log(`     ${readType.toUpperCase().padEnd(10)} - Count: ${readData.count.toString().padStart(5)}, Avg: ${stats.avg.toFixed(0).padStart(4)}ms, p95: ${stats.p95.toFixed(0).padStart(4)}ms, Errors: ${errorRate}`);
    }
  }

  // UPDATE operation stats
  if (state.operationBreakdown.update.count > 0) {
    console.log('\n   UPDATE Operations:');
    const updateStats = calculateStats(state.operationBreakdown.update.times);
    const updateErrorRate = state.operationBreakdown.update.errors > 0 ? ((state.operationBreakdown.update.errors / state.operationBreakdown.update.count) * 100).toFixed(2) + '%' : '0%';
    console.log(`     Count: ${state.operationBreakdown.update.count}, Avg: ${updateStats.avg.toFixed(0)}ms, Min: ${updateStats.min.toFixed(0)}ms, Max: ${updateStats.max.toFixed(0)}ms, p95: ${updateStats.p95.toFixed(0)}ms, Errors: ${updateErrorRate}`);
  }

  // CREATE operation stats
  if (state.operationBreakdown.create.count > 0) {
    console.log('\n   CREATE Operations:');
    const createStats = calculateStats(state.operationBreakdown.create.times);
    const createErrorRate = state.operationBreakdown.create.errors > 0 ? ((state.operationBreakdown.create.errors / state.operationBreakdown.create.count) * 100).toFixed(2) + '%' : '0%';
    console.log(`     Count: ${state.operationBreakdown.create.count}, Avg: ${createStats.avg.toFixed(0)}ms, Min: ${createStats.min.toFixed(0)}ms, Max: ${createStats.max.toFixed(0)}ms, p95: ${createStats.p95.toFixed(0)}ms, Errors: ${createErrorRate}`);
  }

  // Capacity analysis
  if (state.shouldStop && state.stopReason) {
    console.log('\n🚨 Capacity Limit Reached:');
    console.log(`   Reason:                ${state.stopReason}`);
    console.log(`   Max Stable Users:      ${CONFIG.currentConcurrency - CONFIG.rampIncrement}`);
    console.log(`   Failed at:             ${CONFIG.currentConcurrency} users`);
  } else {
    console.log('\n✅ Test Completed:');
    console.log(`   Status:                Successfully handled ${CONFIG.currentConcurrency} concurrent users`);
    console.log(`   Recommendation:        Could potentially handle more load`);
  }

  console.log('\n⚙️  Configuration:');
  console.log(`   Target:                ${API_BASE}`);
  console.log(`   Test Duration:         ${CONFIG.testDuration}s`);
  console.log(`   Ramp Interval:         ${CONFIG.rampInterval}s`);
  console.log(`   Ramp Increment:        +${CONFIG.rampIncrement} users`);
  console.log(`   Think Time:            ${CONFIG.thinkTimeMin}-${CONFIG.thinkTimeMax}ms`);
  console.log(`   Session Length:        ${CONFIG.sessionLength[0]}-${CONFIG.sessionLength[1]} ops`);
  console.log(`   Hot Data:              ${CONFIG.hotDataPercent}% of items receive ${CONFIG.hotTrafficPercent}% of reads`);

  console.log('\n📋 Test Dataset:');
  console.log(`   Workspaces:            ${state.workspaceIds.length}`);
  console.log(`   Total Items:           ${state.allItemIds.length}`);
  console.log(`   Hot Items:             ${state.hotItemIds.length}`);
  console.log(`   Cold Items:            ${state.coldItemIds.length}`);

  console.log('\n' + '='.repeat(80));

  if (state.shouldStop && state.stopReason) {
    console.log('⚠️  Test STOPPED: Capacity limit reached');
  } else {
    console.log('✅ Test COMPLETED successfully!');
  }
}

/**
 * Main test execution
 */
async function runRealisticLoadTest() {
  console.log('🚀 Starting Realistic Load Test');
  console.log('='.repeat(80));
  console.log(`Target:              ${API_BASE}`);
  console.log(`Start Concurrency:   ${CONFIG.startConcurrency} users`);
  console.log(`Max Concurrency:     ${CONFIG.maxConcurrency} users`);
  console.log(`Test Duration:       ${CONFIG.testDuration} seconds`);
  console.log(`Ramp Interval:       ${CONFIG.rampInterval} seconds (+${CONFIG.rampIncrement} users)`);
  console.log(`User Personas:       Viewer ${CONFIG.viewerPercent}% / Editor ${CONFIG.editorPercent}% / Power ${CONFIG.powerUserPercent}%`);
  console.log('='.repeat(80));
  console.log('');

  state.startTime = Date.now();
  state.stopTime = state.startTime + (CONFIG.testDuration * 1000);

  try {
    // Load pre-populated data if available
    if (process.env.PREPOPULATE_DATA) {
      console.log('📊 Loading pre-populated data...');
      const prepopData = JSON.parse(process.env.PREPOPULATE_DATA);
      state.workspaceIds = prepopData.workspaceIds;
      state.allItemIds = prepopData.itemIds;

      // Distribute items into hot/cold sets (20% hot, 80% cold)
      const hotCount = Math.floor(state.allItemIds.length * (CONFIG.hotDataPercent / 100));
      state.hotItemIds = state.allItemIds.slice(0, hotCount);
      state.coldItemIds = state.allItemIds.slice(hotCount);

      console.log(`✅ Loaded ${state.workspaceIds.length} workspaces and ${state.allItemIds.length} items`);
      console.log(`   Hot items: ${state.hotItemIds.length}, Cold items: ${state.coldItemIds.length}\n`);
    } else {
      console.log('⚠️  No pre-populated data found. Tests will create data as needed.\n');
    }

    // Start initial workers
    console.log(`🚀 Starting ${CONFIG.startConcurrency} initial workers...`);

    for (let i = 0; i < CONFIG.startConcurrency; i++) {
      const persona = i < CONFIG.startConcurrency * (CONFIG.viewerPercent / 100) ? 'viewer'
        : i < CONFIG.startConcurrency * ((CONFIG.viewerPercent + CONFIG.editorPercent) / 100) ? 'editor'
        : 'powerUser';

      state.workers.push(userWorker(i, persona));
    }

    console.log('✅ Initial workers started\n');

    // Monitor and ramp up
    const monitorInterval = setInterval(() => {
      displayProgress();

      const check = checkFailureCriteria();

      if (check.failed) {
        state.shouldStop = true;
        state.stopReason = check.reason;
        clearInterval(monitorInterval);
        clearInterval(rampInterval);
      }
    }, 5000);

    // Ramp up concurrency
    const rampInterval = setInterval(() => {
      if (state.shouldStop || CONFIG.currentConcurrency >= CONFIG.maxConcurrency) {
        clearInterval(rampInterval);
        return;
      }

      // Add more workers
      const newWorkers = CONFIG.rampIncrement;

      for (let i = 0; i < newWorkers; i++) {
        const workerId = state.workers.length;
        const persona = workerId < state.workers.length * (CONFIG.viewerPercent / 100) ? 'viewer'
          : workerId < state.workers.length * ((CONFIG.viewerPercent + CONFIG.editorPercent) / 100) ? 'editor'
          : 'powerUser';

        state.workers.push(userWorker(workerId, persona));
      }

      CONFIG.currentConcurrency += newWorkers;
      console.log(`\n📈 Ramped up to ${CONFIG.currentConcurrency} concurrent users`);
    }, CONFIG.rampInterval * 1000);

    // Wait for workers to complete or stop
    await Promise.all(state.workers);

    clearInterval(monitorInterval);
    clearInterval(rampInterval);

  } catch (error) {
    console.error(`\n\n❌ Fatal error: ${error.message}`);
    console.error(error.stack);
  } finally {
    state.stopTime = Date.now();
    displayReport();
  }
}

// Export for external use
if (require.main === module) {
  runRealisticLoadTest().catch(error => {
    console.error('Test failed:', error);
    process.exit(1);
  });
}

module.exports = { runRealisticLoadTest, state };
