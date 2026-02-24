#!/usr/bin/env node

/**
 * PostgreSQL Test Setup via API
 *
 * Performs initial setup and authentication for PostgreSQL load testing:
 * 1. Completes initial setup with admin user
 * 2. Logs in as admin
 * 3. Creates bearer token for API testing
 */

const API_BASE = process.argv[2] || 'http://localhost:8080';

async function request(endpoint, options = {}) {
  const url = `${API_BASE}${endpoint}`;
  const response = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers
    },
    ...options
  });

  let data = null;
  const contentType = response.headers.get('content-type');

  if (contentType && contentType.includes('application/json') && response.status !== 204) {
    try {
      data = await response.json();
    } catch (e) {
      // Ignore JSON parse errors
    }
  }

  if (!response.ok) {
    const text = data ? JSON.stringify(data) : await response.text();
    throw new Error(`HTTP ${response.status}: ${text}`);
  }

  return { status: response.status, data, response };
}

async function setupPostgresTest() {
  console.log('Setting up PostgreSQL test environment via API...');

  try {
    // Step 1: Check setup status
    console.log('1. Checking setup status...');
    const statusResponse = await request('/api/setup/status');

    if (statusResponse.data.setup_complete) {
      console.log('   Setup already complete!');
    } else {
      console.log('   Setup not complete, performing initial setup...');

      // Step 2: Complete initial setup
      console.log('2. Completing initial setup...');
      await request('/api/setup/complete', {
        method: 'POST',
        body: JSON.stringify({
          email: 'admin@test.com',
          username: 'admin',
          password: 'testpass123',
          first_name: 'Test',
          last_name: 'Admin'
        })
      });
      console.log('   Initial setup complete!');
    }

    // Step 3: Login as admin
    console.log('3. Logging in as admin...');
    const loginResponse = await request('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({
        username: 'admin',
        password: 'testpass123'
      })
    });

    // Get session cookie from response
    const setCookieHeader = loginResponse.response.headers.get('set-cookie');
    if (!setCookieHeader) {
      throw new Error('No session cookie received from login');
    }

    // Extract session ID from cookie
    const sessionMatch = setCookieHeader.match(/session_id=([^;]+)/);
    if (!sessionMatch) {
      throw new Error('Could not extract session ID from cookie');
    }
    const sessionCookie = `session_id=${sessionMatch[1]}`;

    console.log('   Login successful!');

    // Step 4: Create bearer token for testing
    console.log('4. Creating bearer token...');
    const tokenResponse = await request('/api/api-tokens', {
      method: 'POST',
      headers: {
        'Cookie': sessionCookie
      },
      body: JSON.stringify({
        name: 'PostgreSQL Load Test Token',
        permissions: ['read', 'write', 'admin']
      })
    });

    const bearerToken = tokenResponse.data.token;
    console.log(`   Bearer token created: ${bearerToken.substring(0, 20)}...`);

    // Step 5: Save token to file for use by load test
    const fs = require('fs');
    const path = require('path');
    const tokenFile = path.join(__dirname, 'test_token.txt');
    fs.writeFileSync(tokenFile, bearerToken);
    console.log(`   Token saved to: ${tokenFile}`);

    console.log('\n✅ PostgreSQL test setup complete!');
    console.log(`   API Base: ${API_BASE}`);
    console.log(`   Bearer Token: ${bearerToken.substring(0, 20)}...`);

  } catch (error) {
    console.error('\n❌ Setup failed:', error.message);
    console.error(error.stack);
    process.exit(1);
  }
}

setupPostgresTest();
