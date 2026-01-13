# Demo Content Generator

Comprehensive demo content generator that creates realistic multi-workspace scenarios for testing, development, and demonstration purposes.

> **Note**: This tool is configured via environment variables `APP_NAME` (default: WINDSHIFT) and `BINARY_NAME` (default: windshift) for easy rebranding.

## Overview

This script generates a complete demo environment with:

- **3 Workspaces** representing different use cases:
  - Software Development (SOFT)
  - Customer Support (SUPP)
  - Marketing (MKTG)

- **Demo Users** with different roles:
  - Admin User (admin@demo.com)
  - Developer (john@demo.com)
  - Support Agent (jane@demo.com)
  - Marketing Manager (mike@demo.com)

- **Custom Fields** of various types:
  - Text fields (Environment, Browser, URL)
  - Number fields (Story Points, Estimated Hours)
  - Select fields (Severity, Customer Tier, Request Type, Campaign Type)
  - Date fields (Due Date, Release Date)

- **Screens** for different workflows:
  - Bug Report Screen
  - Feature Request Screen
  - Support Ticket Screen
  - Marketing Campaign Screen

- **Hierarchical Work Items** with realistic content:
  - Software Dev: Epics → Stories → Tasks
  - Support: Categories → Tickets → Sub-tasks
  - Marketing: Campaigns → Tasks
  - 50+ total work items across all workspaces

- **Projects & Priorities**:
  - 5 projects across workspaces
  - 4 priority levels with icons and colors

## Prerequisites

1. **Node.js** (v18 or higher)
2. **Server binary** built: `go build -o windshift` (or custom name via `$BINARY_NAME`)

## Quick Start

### Generate Demo with Default Settings

```bash
cd frontend/e2e
node generate-demo.js
```

This will:
1. Create a new `demo.db` database
2. Start server on port 8080
3. Complete initial setup
4. Generate all demo content
5. Stop the server when done

### Access the Demo

After generation:
- **URL**: http://localhost:8080
- **Username**: `admin`
- **Password**: `Admin123!`

Other demo users:
- `john` / `Demo123!` (Developer)
- `jane` / `Demo123!` (Support Agent)
- `mike` / `Demo123!` (Marketing Manager)

## Usage Options

### Custom Port

```bash
node generate-demo.js --port 3000
```

### Custom Database

```bash
node generate-demo.js --db my-demo.db
```

### Stop Server After Generation

By default, the server keeps running after demo generation. To stop it automatically (for CI/e2e tests):

```bash
node generate-demo.js --stop-server
```

### Use Existing Server

If you already have a server running:

```bash
node generate-demo.js --no-server --base-url http://localhost:8080
```

### Custom Binary Path

```bash
node generate-demo.js --binary /path/to/windshift
```

### Help

```bash
node generate-demo.js --help
```

## Common Scenarios

### Fresh Demo for Testing

```bash
# Generate new demo (server stays running by default)
node generate-demo.js --db test-demo.db
```

### CI/CD Demo Generation

```bash
# Generate demo on existing server (non-interactive)
node generate-demo.js --no-server --base-url http://localhost:8080
```

### Quick Local Demo

```bash
# Default settings - fastest way to get started
node generate-demo.js
```

## What Gets Created

### Workspaces

1. **Software Development (SOFT)**
   - 2 Projects: Mobile App Rewrite, API v2 Development
   - Work items: 2 epics with stories and tasks
   - Custom fields: Story Points, Estimated Hours, Environment, Browser

2. **Customer Support (SUPP)**
   - 2 Projects: Customer Onboarding, Technical Support
   - Work items: Support categories with tickets and sub-tasks
   - Custom fields: Customer Tier, Request Type, Severity

3. **Marketing (MKTG)**
   - 1 Project: Q1 2025 Campaign
   - Work items: Campaigns with tasks
   - Custom fields: Campaign Type, Due Date, Release Date

### Hierarchical Work Items

The script creates realistic parent-child relationships:

```
Software Development (SOFT):
├── Mobile App Rewrite (Epic)
│   ├── Setup React Native project (Story)
│   │   ├── Configure ESLint (Task)
│   │   └── Setup CI/CD (Task)
│   ├── Implement authentication (Story)
│   │   ├── Design login screen (Task)
│   │   ├── Integrate OAuth (Task)
│   │   └── Token storage (Task)
│   └── Build dashboard (Story)
├── API v2 with GraphQL (Epic)
│   ├── Design schema (Story)
│   ├── Implement resolvers (Story)
│   │   ├── User resolver (Task)
│   │   └── Workspace resolver (Task)
│   └── Add GraphQL playground (Story)
└── Login redirect bug (Standalone Bug)

Customer Support (SUPP):
├── Enterprise Onboarding (Category)
│   ├── Acme Corp Setup (Ticket)
│   │   ├── Configure SAML (Sub-task)
│   │   └── Import users (Sub-task)
│   └── TechStart Migration (Ticket)
├── Technical Support Queue (Category)
│   ├── Email notifications issue (Ticket)
│   ├── CSV export broken (Ticket)
│   └── Custom workflows help (Ticket)
└── Dark mode feature request (Standalone)

Marketing (MKTG):
├── Q1 Product Launch (Campaign)
│   ├── Email series (Task)
│   │   ├── Design templates (Sub-task)
│   │   ├── Write copy (Sub-task)
│   │   └── Setup A/B tests (Sub-task)
│   ├── Social media (Task)
│   │   ├── LinkedIn posts (Sub-task)
│   │   └── Twitter thread (Sub-task)
│   └── Blog post (Task)
├── Webinar Series (Campaign)
│   ├── Plan topics (Task)
│   └── Book speakers (Task)
└── Update homepage (Standalone)
```

## Script Features

### CORS Support

The script automatically allows CORS for localhost:
- Adds `--allowed-hosts localhost,127.0.0.1` when starting server
- Enables cross-origin requests from local development environments
- No additional configuration needed

### Clean Database State

The script automatically:
- Deletes existing database file if present
- Removes WAL files (SQLite)
- Starts with fresh state

### Robust Error Handling

- Validates binary exists before starting
- Waits for server readiness with timeout
- Graceful cleanup on errors
- Detailed error messages

### Beautiful Console Output

- Color-coded messages (success/error/info)
- Progress indicators
- Hierarchical work item visualization
- Summary statistics

### Modular Design

Can be imported as a module for testing:

```javascript
import {
  createUsers,
  createWorkspaces,
  createWorkItems
} from './generate-demo.js';

// Use individual functions in your tests
```

## Continuous Integration

### Example: Playwright Global Setup

You can use this script in your test setup:

```javascript
// global.setup.ts
import { execSync } from 'child_process';

setup('generate demo content', async () => {
  execSync('node generate-demo.js --no-server', {
    cwd: 'frontend/e2e',
    stdio: 'inherit'
  });
});
```

### Example: Go Test Setup

```go
func TestDemoContent(t *testing.T) {
    // Start server
    // ...

    // Generate demo
    cmd := exec.Command("node", "frontend/e2e/generate-demo.js",
        "--no-server",
        "--base-url", serverURL)
    output, err := cmd.CombinedOutput()
    require.NoError(t, err, string(output))

    // Run tests against demo content
    // ...
}
```

## Troubleshooting

### Binary Not Found

```
✗ Binary not found at: ../windshift
```

**Solution**: Build the binary first:
```bash
cd /Users/stefanernst/realigned/windshift
go build -o windshift  # or your custom $BINARY_NAME
```

### Server Startup Timeout

```
✗ Server failed to start within 30 seconds
```

**Solutions**:
- Check if port is already in use
- Verify database permissions
- Try a different port: `--port 3000`

### Port Already in Use

**Solution**: Use a different port or stop the existing server:
```bash
# Use different port
node generate-demo.js --port 9000

# Or kill existing server
lsof -ti:8080 | xargs kill
```

### CORS Origin Not Allowed

```
Error: CORS: Origin not allowed
```

**Solution**: If running the server manually (not with the script), add the `--allowed-hosts` flag:
```bash
# Run server with CORS enabled for localhost
./windshift --allowed-hosts localhost,127.0.0.1

# Or for development, disable CSRF (less secure)
./windshift --no-csrf
```

### Permission Denied

```
Error: EACCES: permission denied
```

**Solution**: Make script executable:
```bash
chmod +x generate-demo.js
```

### Database Locked

```
Error: database is locked
```

**Solution**:
- Ensure no other server instances are running
- Delete database file and try again:
```bash
rm demo.db demo.db-shm demo.db-wal
```

## File Structure

```
frontend/e2e/
├── generate-demo.js      # Main script
├── demo-data.js          # Data definitions
└── README-DEMO.md        # This file
```

## Customization

### Modify Demo Content

Edit `demo-data.js` to customize:

```javascript
// Add more workspaces
export const workspaces = [
  // ... existing workspaces
  {
    name: 'Your New Workspace',
    key: 'NEW',
    description: 'Your description'
  }
];

// Add more work items
export const workItems = {
  'NEW': [
    {
      title: 'Your Work Item',
      description: 'Description',
      status: 'open',
      children: [ /* nested items */ ]
    }
  ]
};
```

### Extend the Script

The script exports functions for custom use:

```javascript
import { createWorkspaces, createUsers } from './generate-demo.js';

// Create your own generation logic
async function customGeneration() {
  const token = await getBearerToken('http://localhost:8080');
  const workspaces = await createWorkspaces('http://localhost:8080', token);
  // ... your custom logic
}
```

## Performance

Typical generation time:
- **Setup & Auth**: ~2-3 seconds
- **Users, Workspaces, Fields**: ~1-2 seconds
- **Work Items (50+)**: ~5-10 seconds
- **Total**: ~10-15 seconds

The script creates items sequentially to maintain parent-child relationships correctly.

## Best Practices

1. **Use Clean Database**: Always start with a fresh database for consistent results
2. **Keep Server Running**: Use `--keep-server` when actively developing/testing
3. **Version Control**: Don't commit `demo.db` - it's regenerated each time
4. **CI/CD**: Use `--no-server` when server is already running in CI
5. **Documentation**: Update `demo-data.js` comments when adding new content

## Support

For issues or questions:
- Check server logs for errors
- Verify prerequisites are met
- Review troubleshooting section
- Run with verbose Node output: `NODE_DEBUG=* node generate-demo.js`

