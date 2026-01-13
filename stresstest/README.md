# Load Testing for Windshift

This directory contains load testing tools to stress test the Windshift work management system under high load conditions.

## Overview

Two types of load tests are available:

1. **Write-Heavy Load Test** (`load-test.js`): Creates 100,000 work items across 100 workspaces to test pure write throughput
2. **Mixed Workload Test** (`mixed-load-test.js`): Simulates realistic usage with a mix of creates, reads, and updates

## Features

- **Concurrent Operations**: Uses Node.js async/await with Promise.all() for maximum concurrency
- **Configurable Load**: Adjust workspaces, items, and concurrency levels
- **Real-time Progress**: Live statistics showing throughput, response times, and errors
- **Comprehensive Metrics**: Min/max/avg/median/p95/p99 response times
- **Error Handling**: Automatic retry logic with exponential backoff
- **Isolated Testing**: Creates temporary database for local tests
- **Remote Testing**: Can test against production or staging servers

## Quick Start

### Write-Heavy Load Test

Test maximum write throughput by creating items as fast as possible:

```bash
cd stresstest
./run-load-test.sh
```

This will:
1. Create an isolated test database
2. Start a test server on a random port
3. Create 100,000 items across 100 workspaces
4. Clean up all test data automatically

**Custom parameters:**
```bash
# Smaller test: 10 workspaces, 100 items each = 1,000 items
./run-load-test.sh --workspaces 10 --items 100

# Medium test: 50 workspaces, 500 items each = 25,000 items
./run-load-test.sh --workspaces 50 --items 500 --concurrency 100

# Full test with higher concurrency
./run-load-test.sh --workspaces 100 --items 1000 --concurrency 100
```

### Mixed Workload Test (Recommended for Realism)

Test realistic usage patterns with mixed operations:

```bash
cd stresstest
./run-mixed-load-test.sh
```

This simulates real-world usage with:
- **40% Create operations**: New items being created
- **40% Read operations**: Listing, filtering, and viewing items
- **20% Update operations**: Status changes, priority updates, assignments

**Custom scenarios:**
```bash
# Quick 30-second test
./run-mixed-load-test.sh --duration 30

# Read-heavy workload (browsing/searching)
./run-mixed-load-test.sh --create 20 --read 60 --update 20

# Write-heavy workload (data entry)
./run-mixed-load-test.sh --create 60 --read 20 --update 20

# Update-heavy workload (team collaboration)
./run-mixed-load-test.sh --create 20 --read 30 --update 50

# Long-running stress test (2 minutes, 100 concurrent workers)
./run-mixed-load-test.sh --duration 120 --concurrency 100
```

### Remote Testing

Test against a remote server:

```bash
# Set your authentication token
export BEARER_TOKEN=crw_your_actual_token_here

# Run against remote server
./run-load-test.sh --remote http://your-server.com:8080
```

**⚠️ WARNING**: Remote testing will create real data on the target server. Use with caution!

## Configuration Options

### Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `--workspaces N` | Number of workspaces to create | 100 |
| `--items N` | Items per workspace | 1000 |
| `--concurrency N` | Concurrent requests in-flight | 50 |
| `--remote URL` | Test against remote server | (local) |
| `-h, --help` | Show help message | - |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `API_BASE` | API endpoint URL | http://localhost:8080/api |
| `BEARER_TOKEN` | Authentication token | crw_test123... (test token) |
| `WORKSPACES` | Number of workspaces | 100 |
| `ITEMS_PER_WORKSPACE` | Items per workspace | 1000 |
| `CONCURRENCY` | Concurrent requests | 50 |
| `RETRY_ATTEMPTS` | Number of retry attempts | 3 |
| `REQUEST_TIMEOUT` | Request timeout (ms) | 30000 |

## Understanding the Results

### Key Metrics

- **Throughput**: Items created per second - higher is better
- **Response Times**:
  - **Average**: Mean response time across all requests
  - **Median**: 50% of requests complete faster than this
  - **p95**: 95% of requests complete faster than this (important for SLAs)
  - **p99**: 99% of requests complete faster than this (detects outliers)
- **Error Rate**: Percentage of failed requests - should be < 1%

### Success Criteria

The test is considered successful if:
- ✅ All workspaces are created
- ✅ All items are created
- ✅ Error rate is below 1%
- ✅ No fatal errors occur

## How It Works

### Concurrency Model

The load test uses Node.js's async I/O model for concurrency:

```javascript
// Multiple requests in-flight simultaneously
await Promise.all([
  createItem(workspace, 1),
  createItem(workspace, 2),
  createItem(workspace, 3),
  // ... 50 concurrent requests
])
```

Even though Node.js is single-threaded, the event loop allows many HTTP requests to be in-flight at the same time, effectively simulating multiple concurrent users.

### Test Phases

1. **Workspace Creation**: Creates all workspaces in batches of 10
2. **Item Creation**: For each workspace, creates items in batches based on concurrency setting
3. **Progress Tracking**: Real-time updates every batch
4. **Report Generation**: Final statistics and performance analysis

### Error Handling

- **Automatic Retries**: Failed requests are retried up to 3 times with exponential backoff
- **Error Logging**: All errors are logged with details for debugging
- **Graceful Degradation**: Test continues even if some requests fail
- **Final Validation**: Test fails if error rate exceeds 1%

## Interpreting Results

### Good Performance

```
Throughput:            300+ items/second
Average Response:      <200ms
95th Percentile:       <500ms
Error Rate:            <0.1%
```

### Concerning Performance

```
Throughput:            <100 items/second
Average Response:      >500ms
95th Percentile:       >2000ms
Error Rate:            >1%
```

If you see concerning performance:
1. Check server resource usage (CPU, memory, disk I/O)
2. Review server logs for errors or warnings
3. Analyze database query performance
4. Consider scaling server resources
5. Optimize slow database queries or API endpoints

## Troubleshooting

### Server Won't Start

```bash
# Check if port is already in use
lsof -i :8080

# Try with explicit port
./run-load-test.sh --port 9090
```

### Database Errors

```bash
# Check database permissions
ls -la *.db

# Remove stale test databases
rm -f loadtest_*.db
```

### High Error Rate

1. Lower concurrency: `--concurrency 10`
2. Increase request timeout: `export REQUEST_TIMEOUT=60000`
3. Check server logs for specific errors
4. Verify network connectivity to remote server

### Out of Memory

If the test consumes too much memory:
1. Reduce batch size in the script
2. Lower concurrency setting
3. Run smaller test first to verify system capacity

## Advanced Usage

### Direct Script Execution

You can run the load test script directly with custom environment variables:

```bash
cd stresstest

# Custom configuration
export API_BASE=http://localhost:8080/api
export WORKSPACES=50
export ITEMS_PER_WORKSPACE=500
export CONCURRENCY=25

node load-test.js
```

### Continuous Testing

Run load tests periodically to track performance over time:

```bash
#!/bin/bash
# continuous-load-test.sh

for i in {1..10}; do
  echo "Run $i of 10"
  ./run-load-test.sh --workspaces 10 --items 100
  sleep 300  # Wait 5 minutes between runs
done
```

### Performance Benchmarking

Compare performance between different configurations:

```bash
# Baseline
./run-load-test.sh --workspaces 100 --items 1000 --concurrency 50 > baseline.txt

# After optimization
./run-load-test.sh --workspaces 100 --items 1000 --concurrency 50 > optimized.txt

# Compare
diff baseline.txt optimized.txt
```

## Safety Notes

- **Local Testing**: Always safe - uses isolated database that's automatically cleaned up
- **Remote Testing**: Creates real data - ensure you have permission and backups
- **Production Testing**: Never run against production without proper planning and approval
- **Resource Usage**: Monitor server resources during testing to avoid overload

## Requirements

- Node.js 18+ (for fetch API and AbortSignal.timeout)
- SQLite3 (for local testing)
- Compiled Windshift binary (for local testing)
- Network access to target server (for remote testing)

## Contributing

When modifying the load test:
1. Test with small loads first (`--workspaces 1 --items 10`)
2. Gradually increase load to verify behavior
3. Document any new configuration options
4. Update this README with new features

## License

Same as the main Windshift project.
