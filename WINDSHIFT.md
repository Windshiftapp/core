# Windshift CLI

This project is connected to Windshift workspace **WIS** (Windshift).

## Quick Commands

```bash
# My work
ws task mine              # Tasks assigned to me
ws task created           # Tasks I created

# Create & manage
ws task create -t "Title" [-d "Description"]
ws task move <KEY-123> <status>
ws task get <KEY-123>

# Test execution
ws test run mine          # My test runs
ws test run start <set>   # Start test run
ws test result <run> <case> passed|failed|blocked|skipped
```

## Available Item Types

- Target Initiative
- Zap Epic
- BookOpen Story
- CheckSquare Task
- Bug Bug
- Minus Sub-task

## Available Statuses

| ID | Status | Category | Default | Completed |
|----|--------|----------|---------|------------|
| 1 | Open | To Do |  |  |
| 2 | To Do | To Do |  |  |
| 3 | In Progress | In Progress |  |  |
| 4 | Under Review | In Progress |  |  |
| 6 | Closed | Done |  | Yes |
| 5 | Done | Done |  | Yes |

## Test Management

```bash
# Test Cases
ws test case ls                    # List all test cases
ws test case get <id>              # Get case with steps

# Test Runs
ws test run mine                   # My assigned runs
ws test run ls                     # List all runs
ws test run get <id>               # Get run with results
ws test run start <set-id>         # Start new run from set
ws test run end <id>               # End/complete a run

# Recording Results
ws test result <run-id> <case-id> passed
ws test result <run-id> <case-id> failed --notes "Issue description"
```

## Configuration

Project config is stored in `ws.toml`. Global config is at `~/.config/ws/config.toml`.

```bash
ws config show                     # Show effective config
ws config init                     # Initialize config
```
