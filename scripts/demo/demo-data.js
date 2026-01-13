/**
 * Demo Data Definitions
 *
 * Comprehensive demo content showcasing different use cases
 */

// Demo users (besides admin which is created during setup)
export const demoUsers = [
  // Original demo users
  {
    email: 'john@demo.com',
    username: 'john',
    password: 'john',
    first_name: 'John',
    last_name: 'Doe',
    role: 'Senior Developer',
    timezone: 'America/New_York',
    language: 'en'
  },
  {
    email: 'jane@demo.com',
    username: 'jane',
    password: 'jane',
    first_name: 'Jane',
    last_name: 'Smith',
    role: 'Support Manager',
    timezone: 'America/Los_Angeles',
    language: 'en'
  },
  {
    email: 'mike@demo.com',
    username: 'mike',
    password: 'mike',
    first_name: 'Mike',
    last_name: 'Wilson',
    role: 'Sales Director',
    timezone: 'America/Chicago',
    language: 'en'
  },
  // Engineering team
  {
    email: 'sarah@demo.com',
    username: 'sarah',
    password: 'sarah',
    first_name: 'Sarah',
    last_name: 'Chen',
    role: 'Developer',
    timezone: 'America/New_York',
    language: 'en'
  },
  {
    email: 'alex@demo.com',
    username: 'alex',
    password: 'alex',
    first_name: 'Alex',
    last_name: 'Kim',
    role: 'Developer',
    timezone: 'America/New_York',
    language: 'en'
  },
  {
    email: 'emily@demo.com',
    username: 'emily',
    password: 'emily',
    first_name: 'Emily',
    last_name: 'Wang',
    role: 'Junior Developer',
    timezone: 'America/New_York',
    language: 'en'
  },
  {
    email: 'tom@demo.com',
    username: 'tom',
    password: 'tom',
    first_name: 'Tom',
    last_name: 'Brown',
    role: 'DevOps Engineer',
    timezone: 'America/New_York',
    language: 'en'
  },
  {
    email: 'lisa@demo.com',
    username: 'lisa',
    password: 'lisa',
    first_name: 'Lisa',
    last_name: 'Park',
    role: 'Data Engineer',
    timezone: 'America/Los_Angeles',
    language: 'en'
  },
  {
    email: 'david@demo.com',
    username: 'david',
    password: 'david',
    first_name: 'David',
    last_name: 'Lee',
    role: 'Tech Lead',
    timezone: 'America/New_York',
    language: 'en'
  },
  {
    email: 'maria@demo.com',
    username: 'maria',
    password: 'maria',
    first_name: 'Maria',
    last_name: 'Garcia',
    role: 'QA Engineer',
    timezone: 'America/Los_Angeles',
    language: 'en'
  },
  // Sales team
  {
    email: 'jennifer@demo.com',
    username: 'jennifer',
    password: 'jennifer',
    first_name: 'Jennifer',
    last_name: 'Adams',
    role: 'Account Executive',
    timezone: 'America/Chicago',
    language: 'en'
  },
  {
    email: 'robert@demo.com',
    username: 'robert',
    password: 'robert',
    first_name: 'Robert',
    last_name: 'Taylor',
    role: 'Account Executive',
    timezone: 'America/Chicago',
    language: 'en'
  },
  {
    email: 'amanda@demo.com',
    username: 'amanda',
    password: 'amanda',
    first_name: 'Amanda',
    last_name: 'White',
    role: 'Sales Rep',
    timezone: 'America/Chicago',
    language: 'en'
  },
  {
    email: 'chris@demo.com',
    username: 'chris',
    password: 'chris',
    first_name: 'Chris',
    last_name: 'Martin',
    role: 'Sales Rep',
    timezone: 'America/Chicago',
    language: 'en'
  },
  // Support team
  {
    email: 'kevin@demo.com',
    username: 'kevin',
    password: 'kevin',
    first_name: 'Kevin',
    last_name: 'Johnson',
    role: 'Support Engineer',
    timezone: 'America/Los_Angeles',
    language: 'en'
  },
  {
    email: 'rachel@demo.com',
    username: 'rachel',
    password: 'rachel',
    first_name: 'Rachel',
    last_name: 'Green',
    role: 'Support Engineer',
    timezone: 'America/Los_Angeles',
    language: 'en'
  },
  {
    email: 'brian@demo.com',
    username: 'brian',
    password: 'brian',
    first_name: 'Brian',
    last_name: 'Miller',
    role: 'Support Specialist',
    timezone: 'America/Los_Angeles',
    language: 'en'
  },
  {
    email: 'nicole@demo.com',
    username: 'nicole',
    password: 'nicole',
    first_name: 'Nicole',
    last_name: 'Davis',
    role: 'Support Specialist',
    timezone: 'America/Los_Angeles',
    language: 'en'
  }
];

// Workspaces - Different use case scenarios
export const workspaces = [
  {
    name: 'Software Development',
    key: 'SOFT',
    description: 'Main software development workspace for product engineering'
  },
  {
    name: 'Customer Support',
    key: 'SUPP',
    description: 'Customer support and service request tracking'
  },
  {
    name: 'Marketing',
    key: 'MKTG',
    description: 'Marketing campaigns and content management'
  }
];

// Projects - One or more per workspace
export const projects = [
  {
    workspaceKey: 'SOFT',
    customerName: 'Acme Corporation',
    name: 'Mobile App Rewrite',
    description: 'Complete rewrite of mobile application with React Native',
    active: true
  },
  {
    workspaceKey: 'SOFT',
    customerName: 'TechStart Inc',
    name: 'API v2 Development',
    description: 'Next generation REST API with GraphQL support',
    active: true
  },
  {
    workspaceKey: 'SUPP',
    customerName: 'Acme Corporation',
    name: 'Customer Onboarding',
    description: 'New customer onboarding and setup assistance',
    active: true
  },
  {
    workspaceKey: 'SUPP',
    customerName: 'Global Industries',
    name: 'Technical Support',
    description: 'Technical issues and bug reports from customers',
    active: true
  },
  {
    workspaceKey: 'MKTG',
    customerName: 'TechStart Inc',
    name: 'Q1 2025 Campaign',
    description: 'First quarter marketing campaign and content',
    active: true
  }
];

// Custom Fields - Various types
export const customFields = [
  // Text fields
  {
    name: 'Environment',
    field_type: 'text',
    description: 'Deployment environment (dev, staging, production)',
    required: false
  },
  {
    name: 'Browser',
    field_type: 'text',
    description: 'Browser and version where issue occurred',
    required: false
  },
  {
    name: 'URL',
    field_type: 'text',
    description: 'URL where issue can be reproduced',
    required: false
  },
  // Number fields
  {
    name: 'Story Points',
    field_type: 'number',
    description: 'Agile story points estimation',
    required: false
  },
  {
    name: 'Estimated Hours',
    field_type: 'number',
    description: 'Estimated hours to complete',
    required: false
  },
  // Select fields
  {
    name: 'Severity',
    field_type: 'select',
    description: 'Bug severity level',
    options: JSON.stringify(['Critical', 'High', 'Medium', 'Low']),
    required: false
  },
  {
    name: 'Customer Tier',
    field_type: 'select',
    description: 'Customer subscription tier',
    options: JSON.stringify(['Enterprise', 'Professional', 'Basic', 'Trial']),
    required: false
  },
  {
    name: 'Request Type',
    field_type: 'select',
    description: 'Type of support request',
    options: JSON.stringify(['Bug Report', 'Feature Request', 'How-to Question', 'Account Issue']),
    required: false
  },
  {
    name: 'Campaign Type',
    field_type: 'select',
    description: 'Marketing campaign type',
    options: JSON.stringify(['Email', 'Social Media', 'Content Marketing', 'Paid Ads', 'Event']),
    required: false
  },
  // Date fields
  {
    name: 'Release Date',
    field_type: 'date',
    description: 'Planned release date',
    required: false
  },
  // User fields (for assets)
  {
    name: 'Owner',
    field_type: 'user',
    description: 'Asset owner/assignee',
    required: false
  }
];

// Screens - Configurable field layouts
export const screens = [
  {
    name: 'Bug Report Screen',
    description: 'Screen for bug reports with technical details',
    fields: ['Environment', 'Browser', 'URL', 'Severity']
  },
  {
    name: 'Feature Request Screen',
    description: 'Screen for feature requests with estimation',
    fields: ['Story Points', 'Estimated Hours']
  },
  {
    name: 'Support Ticket Screen',
    description: 'Screen for customer support tickets',
    fields: ['Customer Tier', 'Request Type', 'Severity']
  },
  {
    name: 'Marketing Campaign Screen',
    description: 'Screen for marketing campaigns',
    fields: ['Campaign Type', 'Release Date']
  }
];

// Priorities - Custom priority definitions
export const priorities = [
  {
    name: 'Critical',
    description: 'Urgent items requiring immediate attention',
    icon: '🔴',
    color: '#dc2626',
    sort_order: 1,
    is_default: false
  },
  {
    name: 'High',
    description: 'High priority items',
    icon: '🟠',
    color: '#ea580c',
    sort_order: 2,
    is_default: false
  },
  {
    name: 'Medium',
    description: 'Normal priority items',
    icon: '🟡',
    color: '#ca8a04',
    sort_order: 3,
    is_default: true
  },
  {
    name: 'Low',
    description: 'Low priority items',
    icon: '🟢',
    color: '#16a34a',
    sort_order: 4,
    is_default: false
  }
];

// Hierarchical Work Items - Realistic data for each workspace
export const workItems = {
  'SOFT': [
    // Epic 1: Mobile App
    {
      title: 'Mobile App Rewrite',
      description: 'Complete rewrite of the mobile application using React Native for iOS and Android',
      status_id: 3,
      priority: 'High',
      is_task: false,
      project: 'Mobile App Rewrite',
      milestoneName: 'MVP Release',
      iterationName: 'Q1 2025',  // Global iteration - cross-team initiative
      custom_fields: {
        'Story Points': 89,
        'Release Date': '2025-03-15'
      },
      children: [
        {
          title: 'Setup React Native project structure',
          description: 'Initialize new React Native project with TypeScript and necessary dependencies',
          status_id: 5,
          priority: 'Medium',
          milestoneName: 'MVP Release',
          iterationName: 'Sprint 2025-01',  // Local sprint
          custom_fields: {
            'Story Points': 8,
            'Estimated Hours': 16
          },
          children: [
            {
              title: 'Configure ESLint and Prettier',
              description: 'Setup code quality tools',
              status_id: 5,
              priority: 'Medium',
              milestoneName: 'MVP Release',
              iterationName: 'Sprint 2025-01',
              is_task: true
            },
            {
              title: 'Setup CI/CD pipeline',
              description: 'Configure automated builds',
              status_id: 5,
              priority: 'Medium',
              milestoneName: 'MVP Release',
              iterationName: 'Sprint 2025-01',
              is_task: true
            }
          ]
        },
        {
          title: 'Implement authentication flow',
          description: 'Build login, registration, and password reset screens with secure token management',
          status_id: 1,
          priority: 'High',
          milestoneName: 'MVP Release',
          iterationName: 'Sprint 2025-02',  // Next sprint
          custom_fields: {
            'Story Points': 13,
            'Estimated Hours': 32
          },
          children: [
            {
              title: 'Design login screen UI',
              description: 'Create responsive login form',
              status_id: 5,
              priority: 'Medium',
              milestoneName: 'Beta Launch',
              iterationName: 'Sprint 2025-01',
              is_task: true
            },
            {
              title: 'Integrate OAuth providers',
              description: 'Add Google and Apple sign-in',
              status_id: 1,
              priority: 'Critical',
              milestoneName: 'Beta Launch',
              iterationName: 'Sprint 2025-02',
              is_task: true
            },
            {
              title: 'Implement secure token storage',
              description: 'Use keychain for token storage',
              status_id: 1,
              priority: 'Critical',
              milestoneName: 'Beta Launch',
              iterationName: 'Sprint 2025-02',
              is_task: true
            }
          ]
        },
        {
          title: 'Build dashboard and navigation',
          description: 'Create main dashboard with bottom tab navigation',
          status_id: 1,
          priority: 'Medium',
          milestoneName: 'MVP Release',
          iterationName: 'Sprint 2025-02',
          due_date: '2025-02-01',
          custom_fields: {
            'Story Points': 13
          }
        }
      ]
    },
    // Epic 2: API Development
    {
      title: 'API v2 with GraphQL Support',
      description: 'Develop next generation API with GraphQL alongside REST endpoints',
      status_id: 3,
      priority: 'Medium',
      project: 'API v2 Development',
      milestoneName: 'API v2 Release',
      iterationName: 'Q1 2025',  // Global iteration
      custom_fields: {
        'Story Points': 55,
        'Release Date': '2025-02-28'
      },
      children: [
        {
          title: 'Design GraphQL schema',
          description: 'Define types, queries, and mutations for core entities',
          status_id: 5,
          priority: 'Medium',
          milestoneName: 'API v2 Release',
          iterationName: 'Sprint 2025-01',
          custom_fields: {
            'Story Points': 8
          }
        },
        {
          title: 'Implement GraphQL resolvers',
          description: 'Build resolver functions with DataLoader for N+1 optimization',
          status_id: 1,
          priority: 'Medium',
          milestoneName: 'API v2 Release',
          iterationName: 'Sprint 2025-02',
          custom_fields: {
            'Story Points': 21,
            'Estimated Hours': 48
          },
          children: [
            {
              title: 'User resolver with authentication',
              description: 'Implement user queries and mutations',
              status_id: 5,
              priority: 'Medium',
              milestoneName: 'API v2 Release',
              iterationName: 'Sprint 2025-01',
              is_task: true
            },
            {
              title: 'Workspace resolver',
              description: 'Implement workspace CRUD operations',
              status_id: 1,
              priority: 'Low',
              milestoneName: 'API v2 Release',
              iterationName: 'Sprint 2025-02',
              is_task: true
            }
          ]
        },
        {
          title: 'Add GraphQL playground for development',
          description: 'Setup GraphiQL interface for API exploration',
          status_id: 1,
          priority: 'Low',
          milestoneName: 'API v2 Release',
          iterationName: 'v1.5 Release',  // Part of release iteration
          custom_fields: {
            'Story Points': 3,
            'Estimated Hours': 4
          }
        }
      ]
    },
    // Standalone bug
    {
      title: 'Fix login redirect loop on Safari',
      description: 'Users on Safari experience infinite redirect loop after successful login',
      status_id: 1,
      priority: 'Critical',
      milestoneName: 'Beta Launch',
      iterationName: 'Company Sprint 1',  // Global sprint - urgent fix
      custom_fields: {
        'Browser': 'Safari 17.2',
        'Environment': 'production',
        'Severity': 'Critical',
        'URL': 'https://app.example.com/login'
      }
    },
    // Additional Sprint 2025-01 items
    {
      title: 'Setup monitoring dashboards',
      description: 'Create Grafana dashboards for API performance and error tracking',
      status_id: 3,
      priority: 'High',
      milestoneName: 'Platform Reliability',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Story Points': 5 }
    },
    {
      title: 'Configure alerting rules',
      description: 'Define PagerDuty alerts for critical services and SLA thresholds',
      status_id: 1,
      priority: 'High',
      milestoneName: 'Platform Reliability',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Story Points': 3 }
    },
    {
      title: 'Setup log aggregation',
      description: 'Configure centralized logging with structured JSON output',
      status_id: 5,
      priority: 'Medium',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Story Points': 5 }
    },
    {
      title: 'Database backup automation',
      description: 'Implement automated daily backups with offsite replication',
      status_id: 5,
      priority: 'High',
      milestoneName: 'Platform Reliability',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Story Points': 8 }
    },
    {
      title: 'User profile page redesign',
      description: 'Modernize user profile page with new design system components',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'MVP Release',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Story Points': 8 }
    },
    {
      title: 'Search functionality improvements',
      description: 'Add fuzzy search and search history to global search',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'MVP Release',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Story Points': 13 }
    },
    {
      title: 'Fix timezone handling in reports',
      description: 'Ensure all reports display times in user-configured timezone',
      status_id: 3,
      priority: 'High',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Severity': 'High', 'Story Points': 3 }
    },
    {
      title: 'Resolve memory leak in worker',
      description: 'Background worker memory consumption grows unbounded over time',
      status_id: 1,
      priority: 'Critical',
      milestoneName: 'Platform Reliability',
      iterationName: 'Sprint 2025-01',
      custom_fields: { 'Severity': 'Critical', 'Story Points': 5 }
    },
    // Additional Sprint 2025-02 items
    {
      title: 'API rate limiting implementation',
      description: 'Implement configurable rate limits per API key with Redis backend',
      status_id: 1,
      priority: 'High',
      milestoneName: 'API v2 Release',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Story Points': 8 }
    },
    {
      title: 'Webhook retry mechanism',
      description: 'Add exponential backoff retry for failed webhook deliveries',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'API v2 Release',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Story Points': 5 }
    },
    {
      title: 'Bulk import functionality',
      description: 'Allow CSV/Excel import for work items with field mapping',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'MVP Release',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Story Points': 13 }
    },
    {
      title: 'Export to PDF feature',
      description: 'Generate PDF reports from dashboards and work item lists',
      status_id: 1,
      priority: 'Low',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Story Points': 8 }
    },
    {
      title: 'Fix pagination on large datasets',
      description: 'Pagination breaks when dataset exceeds 10000 items',
      status_id: 1,
      priority: 'High',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Severity': 'High', 'Story Points': 3 }
    },
    {
      title: 'Implement request caching layer',
      description: 'Add Redis caching for frequently accessed API endpoints',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'Platform Reliability',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Story Points': 8 }
    },
    {
      title: 'Add API versioning support',
      description: 'Implement API versioning with deprecation warnings',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'API v2 Release',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Story Points': 5 }
    },
    {
      title: 'Performance profiling for mobile',
      description: 'Profile and optimize React Native app startup time',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'MVP Release',
      iterationName: 'Sprint 2025-02',
      custom_fields: { 'Story Points': 5 }
    }
  ],

  'SUPP': [
    // Support Category 1
    {
      title: 'Enterprise Customer Onboarding',
      description: 'Onboarding and setup for new enterprise customers',
      status_id: 3,
      priority: 'Medium',
      project: 'Customer Onboarding',
      iterationName: 'Support Onboarding Sprint',  // Local sprint
      children: [
        {
          title: 'Acme Corp - Initial Setup',
          description: 'Setup workspace and configure SSO for Acme Corporation',
          status_id: 1,
          priority: 'Critical',
          due_date: '2025-01-25',
          iterationName: 'Support Onboarding Sprint',
          custom_fields: {
            'Customer Tier': 'Enterprise',
            'Request Type': 'Account Issue'
          },
          children: [
            {
              title: 'Configure SAML SSO',
              description: 'Setup SAML integration with their IdP',
              status_id: 1,
              priority: 'High',
              iterationName: 'Support Onboarding Sprint',
              is_task: true
            },
            {
              title: 'Import user list',
              description: 'Bulk import 200 users from CSV',
              status_id: 1,
              priority: 'Medium',
              iterationName: 'Support Onboarding Sprint',
              is_task: true
            }
          ]
        },
        {
          title: 'TechStart Inc - Migration Assistance',
          description: 'Help customer migrate from competitor platform',
          status_id: 1,
          priority: 'Medium',
          iterationName: 'Support Onboarding Sprint',
          custom_fields: {
            'Customer Tier': 'Enterprise',
            'Request Type': 'How-to Question'
          }
        }
      ]
    },
    // Support Category 2
    {
      title: 'Technical Support Queue',
      description: 'Active technical support requests',
      status_id: 3,
      priority: 'Medium',
      project: 'Technical Support',
      iterationName: 'Company Sprint 1',  // Global sprint - support issues
      children: [
        {
          title: 'Email notifications not sending',
          description: 'Customer reports not receiving email notifications for mentions',
          status_id: 1,
          priority: 'Medium',
          iterationName: 'Company Sprint 1',
          custom_fields: {
            'Customer Tier': 'Professional',
            'Request Type': 'Bug Report',
            'Severity': 'High'
          }
        },
        {
          title: 'Export to CSV feature broken',
          description: 'CSV export returns empty file',
          status_id: 1,
          priority: 'Low',
          iterationName: 'Cross-team Alignment',  // Global - next sprint
          custom_fields: {
            'Customer Tier': 'Basic',
            'Request Type': 'Bug Report',
            'Severity': 'Medium',
            'Browser': 'Chrome 120'
          }
        },
        {
          title: 'How to setup custom workflows?',
          description: 'Customer needs guidance on configuring custom workflow for their process',
          status_id: 1,
          priority: 'Low',
          iterationName: 'Support Onboarding Sprint',
          custom_fields: {
            'Customer Tier': 'Professional',
            'Request Type': 'How-to Question'
          }
        }
      ]
    },
    // Standalone ticket
    {
      title: 'Feature Request: Dark mode',
      description: 'Multiple customers requesting dark mode support for the application',
      status_id: 1,
      priority: 'Low',
      milestoneName: 'Customer Success Goals',  // Global milestone
      iterationName: 'H1 Planning',  // Global PI iteration
      custom_fields: {
        'Request Type': 'Feature Request',
        'Customer Tier': 'Enterprise'
      }
    },
    // Additional Support Onboarding Sprint items
    {
      title: 'Create onboarding checklist template',
      description: 'Standardized checklist for all new enterprise customer onboarding',
      status_id: 1,
      priority: 'High',
      milestoneName: 'Customer Success Goals',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Customer Tier': 'Enterprise', 'Request Type': 'Account Issue' }
    },
    {
      title: 'Write knowledge base articles',
      description: 'Create 10 new KB articles covering common questions',
      status_id: 3,
      priority: 'Medium',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Request Type': 'How-to Question' }
    },
    {
      title: 'Setup automated welcome emails',
      description: 'Configure drip campaign for new user onboarding',
      status_id: 1,
      priority: 'Medium',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Request Type': 'Account Issue' }
    },
    {
      title: 'Train new support team members',
      description: 'Conduct training sessions for 3 new hires on product features',
      status_id: 1,
      priority: 'High',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Customer Tier': 'Professional', 'Request Type': 'How-to Question' }
    },
    {
      title: 'Update support SLA documentation',
      description: 'Revise SLA documents to reflect new response time commitments',
      status_id: 5,
      priority: 'Medium',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Customer Tier': 'Enterprise', 'Request Type': 'Account Issue' }
    },
    {
      title: 'Setup customer health score dashboard',
      description: 'Create dashboard to track customer engagement metrics',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'Customer Success Goals',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Request Type': 'Feature Request' }
    },
    {
      title: 'Implement ticket tagging system',
      description: 'Add auto-tagging for support tickets based on content',
      status_id: 1,
      priority: 'Low',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Request Type': 'Feature Request' }
    },
    {
      title: 'Review and close stale tickets',
      description: 'Audit tickets older than 30 days and follow up or close',
      status_id: 3,
      priority: 'Low',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Customer Tier': 'Basic', 'Request Type': 'Account Issue' }
    },
    {
      title: 'GlobalTech Ltd - Workspace Migration',
      description: 'Migrate customer from legacy system to new platform',
      status_id: 1,
      priority: 'High',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Customer Tier': 'Enterprise', 'Request Type': 'Account Issue' }
    },
    {
      title: 'Create video tutorial series',
      description: 'Record 5 getting-started videos for new users',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'Customer Success Goals',
      iterationName: 'Support Onboarding Sprint',
      custom_fields: { 'Request Type': 'How-to Question' }
    }
  ],

  'MKTG': [
    // Campaign 1
    {
      title: 'Q1 2025 Product Launch Campaign',
      description: 'Multi-channel campaign for new product features',
      status_id: 3,
      priority: 'High',
      project: 'Q1 2025 Campaign',
      milestoneName: 'Q1 Campaign Launch',
      iterationName: 'Q1 Campaign Sprint',  // Local sprint
      custom_fields: {
        'Campaign Type': 'Email',
        'Release Date': '2025-01-30'
      },
      children: [
        {
          title: 'Email announcement series',
          description: 'Design and schedule 3-part email campaign',
          status_id: 1,
          priority: 'High',
          milestoneName: 'Q1 Campaign Launch',
          iterationName: 'Q1 Campaign Sprint',
          due_date: '2025-01-28',
          custom_fields: {
            'Campaign Type': 'Email'
          },
          children: [
            {
              title: 'Design email templates',
              description: 'Create responsive email templates',
              status_id: 5,
              priority: 'Medium',
              milestoneName: 'Q1 Campaign Launch',
              iterationName: 'Q1 Campaign Sprint',
              is_task: true
            },
            {
              title: 'Write email copy',
              description: 'Draft compelling copy for all emails',
              status_id: 1,
              priority: 'Medium',
              milestoneName: 'Q1 Campaign Launch',
              iterationName: 'Q1 Campaign Sprint',
              is_task: true
            },
            {
              title: 'Setup A/B tests',
              description: 'Configure subject line variants',
              status_id: 1,
              priority: 'Low',
              milestoneName: 'Q1 Campaign Launch',
              iterationName: 'Q1 Campaign Sprint',
              is_task: true
            }
          ]
        },
        {
          title: 'Social media content',
          description: 'Create social media posts for all platforms',
          status_id: 1,
          priority: 'Medium',
          milestoneName: 'Q1 Campaign Launch',
          iterationName: 'Q1 Campaign Sprint',
          due_date: '2025-01-29',
          custom_fields: {
            'Campaign Type': 'Social Media'
          },
          children: [
            {
              title: 'LinkedIn posts',
              description: 'Create 5 LinkedIn posts',
              status_id: 1,
              priority: 'Medium',
              milestoneName: 'Q1 Campaign Launch',
              iterationName: 'Q1 Campaign Sprint',
              is_task: true
            },
            {
              title: 'Twitter thread',
              description: 'Write announcement thread',
              status_id: 1,
              priority: 'Low',
              milestoneName: 'Q1 Campaign Launch',
              iterationName: 'Q1 Campaign Sprint',
              is_task: true
            }
          ]
        },
        {
          title: 'Blog post announcement',
          description: 'Write detailed blog post about new features',
          status_id: 1,
          priority: 'Medium',
          milestoneName: 'Q1 Campaign Launch',
          iterationName: 'Q1 Campaign Sprint',
          due_date: '2025-01-27',
          custom_fields: {
            'Campaign Type': 'Content Marketing'
          }
        }
      ]
    },
    // Campaign 2
    {
      title: 'Webinar Series: Best Practices',
      description: 'Monthly webinar series on product best practices',
      status_id: 1,
      priority: 'Medium',
      project: 'Q1 2025 Campaign',
      milestoneName: 'Company Q1 Goals',  // Global milestone
      iterationName: 'Q1 2025',  // Global PI iteration
      custom_fields: {
        'Campaign Type': 'Event'
      },
      children: [
        {
          title: 'Plan webinar topics',
          description: 'Research and finalize 3 webinar topics',
          status_id: 1,
          priority: 'Low',
          iterationName: 'Q1 Campaign Sprint',
          is_task: false
        },
        {
          title: 'Book guest speakers',
          description: 'Invite and confirm 2 industry experts',
          status_id: 1,
          priority: 'Low',
          iterationName: 'Cross-team Alignment',  // Global sprint
          is_task: false
        }
      ]
    },
    // Additional Q1 Campaign Sprint items
    {
      title: 'Design social media assets',
      description: 'Create branded graphics for Instagram, LinkedIn, and Twitter',
      status_id: 1,
      priority: 'High',
      milestoneName: 'Q1 Campaign Launch',
      iterationName: 'Q1 Campaign Sprint',
      custom_fields: { 'Campaign Type': 'Social Media' }
    },
    {
      title: 'Create landing page',
      description: 'Build dedicated landing page for product launch campaign',
      status_id: 3,
      priority: 'High',
      milestoneName: 'Q1 Campaign Launch',
      iterationName: 'Q1 Campaign Sprint',
      custom_fields: { 'Campaign Type': 'Content Marketing' }
    },
    {
      title: 'Write blog series',
      description: 'Create 4-part blog series on product features',
      status_id: 1,
      priority: 'Medium',
      iterationName: 'Q1 Campaign Sprint',
      custom_fields: { 'Campaign Type': 'Content Marketing' }
    },
    {
      title: 'Setup UTM tracking',
      description: 'Configure UTM parameters for all campaign links',
      status_id: 5,
      priority: 'Medium',
      iterationName: 'Q1 Campaign Sprint',
      custom_fields: { 'Campaign Type': 'Email' }
    },
    {
      title: 'Design infographic',
      description: 'Create shareable infographic summarizing key features',
      status_id: 1,
      priority: 'Medium',
      milestoneName: 'Q1 Campaign Launch',
      iterationName: 'Q1 Campaign Sprint',
      custom_fields: { 'Campaign Type': 'Content Marketing' }
    },
    {
      title: 'Coordinate press release',
      description: 'Draft and distribute press release to media outlets',
      status_id: 1,
      priority: 'High',
      iterationName: 'Q1 Campaign Sprint',
      custom_fields: { 'Campaign Type': 'Event' }
    },
    {
      title: 'Setup retargeting ads',
      description: 'Configure Google and LinkedIn retargeting campaigns',
      status_id: 1,
      priority: 'Medium',
      iterationName: 'Q1 Campaign Sprint',
      custom_fields: { 'Campaign Type': 'Social Media' }
    },
    // Standalone task
    {
      title: 'Update website homepage',
      description: 'Refresh homepage copy and images for Q1',
      status_id: 1,
      priority: 'Low',
      iterationName: 'v2.0 Release',  // Global release iteration
      due_date: '2025-02-01'
    }
  ]
};

// Milestone Categories - Global categories for organizing milestones
export const milestoneCategories = [
  { name: 'Product', color: '#3b82f6', description: 'Product development milestones' },
  { name: 'Engineering', color: '#10b981', description: 'Engineering and infrastructure' },
  { name: 'Customer Success', color: '#f59e0b', description: 'Customer-facing milestones' },
  { name: 'Business', color: '#8b5cf6', description: 'Business and strategy goals' }
];

// Milestones - Project milestones and releases (dates relative to current week)
// Supports both global (is_global: true) and local (workspace-specific) milestones
export const milestones = [
  // Global milestones (shared across all workspaces)
  {
    name: 'Company Q1 Goals',
    description: 'Q1 2025 company-wide objectives and key results',
    daysFromMonday: 30,
    status: 'planning',
    is_global: true,
    categoryName: 'Business'
  },
  {
    name: '2025 Product Vision',
    description: 'Annual product roadmap and strategic milestones',
    daysFromMonday: 90,
    status: 'planning',
    is_global: true,
    categoryName: 'Product'
  },
  {
    name: 'Platform Reliability',
    description: 'Cross-team initiative for 99.9% uptime',
    daysFromMonday: 45,
    status: 'in-progress',
    is_global: true,
    categoryName: 'Engineering'
  },
  {
    name: 'Customer Success Goals',
    description: 'Company-wide customer satisfaction targets',
    daysFromMonday: 60,
    status: 'in-progress',
    is_global: true,
    categoryName: 'Customer Success'
  },
  // Local milestones (workspace-specific)
  {
    workspaceKey: 'SOFT',
    name: 'Beta Launch',
    description: 'Beta version for early adopters',
    daysFromMonday: 7,
    status: 'in-progress',
    is_global: false,
    categoryName: 'Product'
  },
  {
    workspaceKey: 'SOFT',
    name: 'MVP Release',
    description: 'Minimum viable product release',
    daysFromMonday: 14,
    status: 'in-progress',
    is_global: false,
    categoryName: 'Product'
  },
  {
    workspaceKey: 'SOFT',
    name: 'API v2 Release',
    description: 'GraphQL API general availability',
    daysFromMonday: 21,
    status: 'in-progress',
    is_global: false,
    categoryName: 'Engineering'
  },
  {
    workspaceKey: 'MKTG',
    name: 'Q1 Campaign Launch',
    description: 'First quarter marketing campaign kickoff',
    daysFromMonday: 4,
    status: 'in-progress',
    is_global: false,
    categoryName: 'Business'
  }
];

// Iterations - Sprints, quarters, and release cycles (dates relative to current week)
// Supports both global and local iterations with different types
export const iterations = [
  // Global iterations (shared across all workspaces)
  {
    name: 'Company Sprint 1',
    description: 'Cross-team sprint for Q1 kickoff initiatives',
    daysFromMonday: 0,
    durationDays: 14,
    status: 'active',
    type: 'Sprint',
    is_global: true
  },
  {
    name: 'Q1 2025',
    description: 'First quarter program increment',
    daysFromMonday: 0,
    durationDays: 90,
    status: 'active',
    type: 'PI / Quarter',
    is_global: true
  },
  {
    name: 'v2.0 Release',
    description: 'Major platform release cycle',
    daysFromMonday: 60,
    durationDays: 30,
    status: 'planned',
    type: 'Release',
    is_global: true
  },
  {
    name: 'Cross-team Alignment',
    description: 'Sprint focused on cross-team integration',
    daysFromMonday: 14,
    durationDays: 14,
    status: 'planned',
    type: 'Sprint',
    is_global: true
  },
  {
    name: 'H1 Planning',
    description: 'First half of year strategic planning period',
    daysFromMonday: 0,
    durationDays: 180,
    status: 'active',
    type: 'PI / Quarter',
    is_global: true
  },
  // Local iterations (workspace-specific)
  {
    workspaceKey: 'SOFT',
    name: 'Sprint 2025-01',
    description: 'January development sprint - Mobile app focus',
    daysFromMonday: 0,
    durationDays: 14,
    status: 'active',
    type: 'Sprint',
    is_global: false
  },
  {
    workspaceKey: 'SOFT',
    name: 'Sprint 2025-02',
    description: 'February development sprint - API features',
    daysFromMonday: 14,
    durationDays: 14,
    status: 'planned',
    type: 'Sprint',
    is_global: false
  },
  {
    workspaceKey: 'SOFT',
    name: 'v1.5 Release',
    description: 'Minor feature release with bug fixes',
    daysFromMonday: 21,
    durationDays: 7,
    status: 'planned',
    type: 'Release',
    is_global: false
  },
  {
    workspaceKey: 'MKTG',
    name: 'Q1 Campaign Sprint',
    description: 'Marketing campaign development and launch sprint',
    daysFromMonday: 0,
    durationDays: 14,
    status: 'active',
    type: 'Sprint',
    is_global: false
  },
  {
    workspaceKey: 'SUPP',
    name: 'Support Onboarding Sprint',
    description: 'Customer onboarding focused sprint',
    daysFromMonday: 0,
    durationDays: 14,
    status: 'active',
    type: 'Sprint',
    is_global: false
  }
];

// Time Tracking Customers - Customer organizations for time tracking
export const timeCustomers = [
  {
    name: 'Acme Corporation',
    email: 'support@acme.com',
    description: 'Enterprise customer - priority support',
    active: true
  },
  {
    name: 'TechStart Inc',
    email: 'help@techstart.io',
    description: 'Mid-sized startup customer',
    active: true
  },
  {
    name: 'Global Industries',
    email: 'support@globalindustries.com',
    description: 'Large enterprise with multiple teams',
    active: true
  }
];

// Work Logs - Time tracking entries (will be added to items after creation)
export const workLogs = [
  // Logs for "Setup React Native project structure"
  {
    itemTitle: 'Setup React Native project structure',
    workspaceKey: 'SOFT',
    projectName: 'Mobile App Rewrite',
    duration: '8h',
    description: 'Initial project setup and configuration',
    date: '2025-01-15'
  },
  {
    itemTitle: 'Setup React Native project structure',
    workspaceKey: 'SOFT',
    projectName: 'Mobile App Rewrite',
    duration: '6h',
    description: 'Dependency installation and testing',
    date: '2025-01-16'
  },
  // Logs for "Design login screen UI"
  {
    itemTitle: 'Design login screen UI',
    workspaceKey: 'SOFT',
    projectName: 'Mobile App Rewrite',
    duration: '4h',
    description: 'Created mockups and responsive layout',
    date: '2025-01-17'
  },
  // Logs for "Design GraphQL schema"
  {
    itemTitle: 'Design GraphQL schema',
    workspaceKey: 'SOFT',
    projectName: 'API v2 Development',
    duration: '6h',
    description: 'Defined types and relationships',
    date: '2025-01-18'
  },
  // Logs for "Email notifications not sending"
  {
    itemTitle: 'Email notifications not sending',
    workspaceKey: 'SUPP',
    projectName: 'Technical Support',
    duration: '3h',
    description: 'Investigated SMTP configuration issues',
    date: '2025-01-19'
  },
  // Logs for "Design email templates"
  {
    itemTitle: 'Design email templates',
    workspaceKey: 'MKTG',
    projectName: 'Q1 2025 Campaign',
    duration: '5h',
    description: 'Created responsive HTML templates',
    date: '2025-01-20'
  }
];

// Test Labels - Categorization tags for test cases
export const testLabels = [
  {
    name: 'smoke',
    color: '#ef4444',
    description: 'Critical smoke tests that must pass'
  },
  {
    name: 'regression',
    color: '#f97316',
    description: 'Regression tests for existing functionality'
  },
  {
    name: 'integration',
    color: '#8b5cf6',
    description: 'Integration tests between components'
  },
  {
    name: 'api',
    color: '#3b82f6',
    description: 'API endpoint tests'
  },
  {
    name: 'ui',
    color: '#10b981',
    description: 'User interface tests'
  },
  {
    name: 'critical',
    color: '#dc2626',
    description: 'Critical path tests'
  }
];

// Test Folders - Hierarchical organization for test cases
export const testFolders = [
  {
    name: 'Authentication & Security',
    description: 'Tests for authentication and security features',
    parent_id: null,
    children: [
      {
        name: 'Login Tests',
        description: 'User login functionality'
      },
      {
        name: 'Permission Tests',
        description: 'Role-based access control'
      }
    ]
  },
  {
    name: 'API Tests',
    description: 'Backend API endpoint testing',
    parent_id: null,
    children: [
      {
        name: 'Workspace APIs',
        description: 'Workspace CRUD operations'
      },
      {
        name: 'Item APIs',
        description: 'Work item management APIs'
      }
    ]
  },
  {
    name: 'UI Tests',
    description: 'Frontend user interface testing',
    parent_id: null,
    children: [
      {
        name: 'Item Management',
        description: 'Creating and editing items'
      },
      {
        name: 'Workflow Designer',
        description: 'Workflow configuration UI'
      }
    ]
  }
];

// Test Cases - Individual test cases with steps
export const testCases = [
  // Authentication & Security > Login Tests
  {
    folderPath: 'Authentication & Security/Login Tests',
    title: 'Successful login with valid credentials',
    preconditions: 'User account exists with username "admin" and password "admin"',
    labels: ['smoke', 'critical', 'ui'],
    steps: [
      {
        action: 'Navigate to login page',
        data: 'URL: /login',
        expected: 'Login form is displayed with username and password fields'
      },
      {
        action: 'Enter username',
        data: 'Username: admin',
        expected: 'Username is entered in the field'
      },
      {
        action: 'Enter password',
        data: 'Password: admin',
        expected: 'Password is masked in the field'
      },
      {
        action: 'Click Login button',
        data: '',
        expected: 'User is redirected to dashboard and session is created'
      }
    ]
  },
  {
    folderPath: 'Authentication & Security/Login Tests',
    title: 'Login fails with invalid password',
    preconditions: 'User account exists',
    labels: ['smoke', 'ui'],
    steps: [
      {
        action: 'Navigate to login page',
        data: 'URL: /login',
        expected: 'Login form is displayed'
      },
      {
        action: 'Enter valid username',
        data: 'Username: admin',
        expected: 'Username is entered'
      },
      {
        action: 'Enter invalid password',
        data: 'Password: wrongpassword',
        expected: 'Invalid password is entered'
      },
      {
        action: 'Click Login button',
        data: '',
        expected: 'Error message displayed: "Invalid credentials"'
      }
    ]
  },
  {
    folderPath: 'Authentication & Security/Login Tests',
    title: 'Logout functionality',
    preconditions: 'User is logged in',
    labels: ['smoke', 'ui'],
    steps: [
      {
        action: 'Click user menu',
        data: '',
        expected: 'User menu dropdown opens'
      },
      {
        action: 'Click Logout option',
        data: '',
        expected: 'User is logged out and redirected to login page'
      },
      {
        action: 'Attempt to access protected page',
        data: 'URL: /dashboard',
        expected: 'Redirected to login page'
      }
    ]
  },
  {
    folderPath: 'Authentication & Security/Login Tests',
    title: 'Session persistence across page refresh',
    preconditions: 'User is logged in',
    labels: ['regression', 'ui'],
    steps: [
      {
        action: 'Login successfully',
        data: 'Username: admin, Password: admin',
        expected: 'User is on dashboard'
      },
      {
        action: 'Refresh the page',
        data: 'Press F5 or Ctrl+R',
        expected: 'User remains logged in and dashboard is displayed'
      }
    ]
  },

  // Authentication & Security > Permission Tests
  {
    folderPath: 'Authentication & Security/Permission Tests',
    title: 'Workspace admin can create items',
    preconditions: 'User has workspace admin role',
    labels: ['critical', 'ui'],
    steps: [
      {
        action: 'Navigate to workspace',
        data: 'Workspace: SOFT',
        expected: 'Workspace items list is displayed'
      },
      {
        action: 'Click Create Item button',
        data: '',
        expected: 'Create item modal opens'
      },
      {
        action: 'Fill in item details and save',
        data: 'Title: Test Item',
        expected: 'Item is created successfully'
      }
    ]
  },
  {
    folderPath: 'Authentication & Security/Permission Tests',
    title: 'Non-admin cannot access admin settings',
    preconditions: 'User is logged in without admin privileges',
    labels: ['critical', 'ui'],
    steps: [
      {
        action: 'Attempt to navigate to admin settings',
        data: 'URL: /admin',
        expected: 'Access denied message or redirect to dashboard'
      }
    ]
  },

  // API Tests > Workspace APIs
  {
    folderPath: 'API Tests/Workspace APIs',
    title: 'Create workspace via API',
    preconditions: 'Valid API token exists',
    labels: ['smoke', 'api', 'critical'],
    steps: [
      {
        action: 'Send POST request to /api/workspaces',
        data: '{"name": "Test Workspace", "key": "TEST", "description": "Test"}',
        expected: 'Response status 201, workspace created with ID'
      },
      {
        action: 'Verify workspace in database',
        data: 'Query: SELECT * FROM workspaces WHERE key = "TEST"',
        expected: 'Workspace exists with correct data'
      }
    ]
  },
  {
    folderPath: 'API Tests/Workspace APIs',
    title: 'Get workspace by ID',
    preconditions: 'Workspace with ID exists',
    labels: ['smoke', 'api'],
    steps: [
      {
        action: 'Send GET request to /api/workspaces/{id}',
        data: 'ID: 1',
        expected: 'Response status 200, workspace data returned'
      },
      {
        action: 'Validate response schema',
        data: 'Check for id, name, key, description fields',
        expected: 'All required fields present'
      }
    ]
  },
  {
    folderPath: 'API Tests/Workspace APIs',
    title: 'Update workspace via API',
    preconditions: 'Workspace exists',
    labels: ['regression', 'api'],
    steps: [
      {
        action: 'Send PUT request to /api/workspaces/{id}',
        data: '{"name": "Updated Name"}',
        expected: 'Response status 200, workspace updated'
      },
      {
        action: 'Get workspace to verify update',
        data: 'GET /api/workspaces/{id}',
        expected: 'Name field reflects new value'
      }
    ]
  },
  {
    folderPath: 'API Tests/Workspace APIs',
    title: 'Delete workspace via API',
    preconditions: 'Workspace exists with no items',
    labels: ['regression', 'api'],
    steps: [
      {
        action: 'Send DELETE request to /api/workspaces/{id}',
        data: 'ID: workspace_to_delete',
        expected: 'Response status 200 or 204'
      },
      {
        action: 'Attempt to GET deleted workspace',
        data: 'GET /api/workspaces/{id}',
        expected: 'Response status 404'
      }
    ]
  },
  {
    folderPath: 'API Tests/Workspace APIs',
    title: 'List all workspaces',
    preconditions: 'Multiple workspaces exist',
    labels: ['smoke', 'api'],
    steps: [
      {
        action: 'Send GET request to /api/workspaces',
        data: '',
        expected: 'Response status 200, array of workspaces returned'
      },
      {
        action: 'Verify workspace count',
        data: '',
        expected: 'At least 3 workspaces in response'
      }
    ]
  },

  // API Tests > Item APIs
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Create top-level item',
    preconditions: 'Workspace exists, valid item type exists',
    labels: ['smoke', 'api', 'critical'],
    steps: [
      {
        action: 'Send POST request to /api/items',
        data: '{"workspace_id": 1, "title": "Test Epic", "item_type_id": 1}',
        expected: 'Response status 201, item created with ID'
      },
      {
        action: 'Verify item hierarchy',
        data: 'Check parent_id is null, level is 0',
        expected: 'Item is top-level with correct hierarchy'
      }
    ]
  },
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Create child item with parent',
    preconditions: 'Parent item exists',
    labels: ['smoke', 'api', 'critical'],
    steps: [
      {
        action: 'Send POST request to /api/items',
        data: '{"workspace_id": 1, "parent_id": 1, "title": "Child Story", "item_type_id": 2}',
        expected: 'Response status 201, child item created'
      },
      {
        action: 'Verify hierarchy',
        data: 'Check parent_id, level, path',
        expected: 'parent_id matches parent, level is 1, path includes parent'
      }
    ]
  },
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Update item title',
    preconditions: 'Item exists',
    labels: ['regression', 'api'],
    steps: [
      {
        action: 'Send PUT request to /api/items/{id}',
        data: '{"title": "Updated Title"}',
        expected: 'Response status 200'
      },
      {
        action: 'Get item to verify',
        data: 'GET /api/items/{id}',
        expected: 'Title is updated, updated_at timestamp changed'
      }
    ]
  },
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Delete item cascades to children',
    preconditions: 'Parent item with children exists',
    labels: ['critical', 'api'],
    steps: [
      {
        action: 'Get count of child items',
        data: 'GET /api/items?parent_id={parent_id}',
        expected: 'Multiple children exist'
      },
      {
        action: 'Delete parent item',
        data: 'DELETE /api/items/{parent_id}',
        expected: 'Response status 200 or 204'
      },
      {
        action: 'Verify children are deleted',
        data: 'GET /api/items?parent_id={parent_id}',
        expected: 'Empty array returned'
      }
    ]
  },
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Filter items by workspace',
    preconditions: 'Items exist in multiple workspaces',
    labels: ['regression', 'api'],
    steps: [
      {
        action: 'Send GET request with workspace filter',
        data: 'GET /api/items?workspace_id=1',
        expected: 'Only items from workspace 1 returned'
      },
      {
        action: 'Verify all items belong to workspace',
        data: 'Check workspace_id field in response',
        expected: 'All items have workspace_id = 1'
      }
    ]
  },
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Get item descendants',
    preconditions: 'Item with nested children exists',
    labels: ['regression', 'api'],
    steps: [
      {
        action: 'Send GET request to /api/items/{id}/descendants',
        data: '',
        expected: 'All descendants returned in flat list'
      },
      {
        action: 'Verify descendant count',
        data: '',
        expected: 'Count matches expected nested children'
      }
    ]
  },
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Get item tree hierarchy',
    preconditions: 'Item with children exists',
    labels: ['integration', 'api'],
    steps: [
      {
        action: 'Send GET request to /api/items/{id}/tree',
        data: '',
        expected: 'Hierarchical tree structure returned'
      },
      {
        action: 'Verify nested structure',
        data: 'Check children array is properly nested',
        expected: 'Children contain their own children recursively'
      }
    ]
  },

  // UI Tests > Item Management
  {
    folderPath: 'UI Tests/Item Management',
    title: 'Create item via UI',
    preconditions: 'User is logged in, workspace exists',
    labels: ['smoke', 'ui', 'critical'],
    steps: [
      {
        action: 'Navigate to workspace items',
        data: 'Workspace: SOFT',
        expected: 'Items list is displayed'
      },
      {
        action: 'Click Create Item button',
        data: '',
        expected: 'Create item modal opens'
      },
      {
        action: 'Fill in title and description',
        data: 'Title: New Epic, Description: Test epic',
        expected: 'Fields are populated'
      },
      {
        action: 'Select item type',
        data: 'Type: Epic',
        expected: 'Epic type selected'
      },
      {
        action: 'Click Save button',
        data: '',
        expected: 'Item created and appears in list'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Item Management',
    title: 'Inline edit item title',
    preconditions: 'Item exists in list',
    labels: ['smoke', 'ui'],
    steps: [
      {
        action: 'Click on item title to edit',
        data: '',
        expected: 'Title becomes editable input field'
      },
      {
        action: 'Change title text',
        data: 'New title: Updated Epic Name',
        expected: 'Text is updated in field'
      },
      {
        action: 'Press Enter or click outside',
        data: '',
        expected: 'Title is saved, input reverts to text'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Item Management',
    title: 'Delete item with confirmation',
    preconditions: 'Item exists',
    labels: ['regression', 'ui'],
    steps: [
      {
        action: 'Click delete button on item',
        data: '',
        expected: 'Confirmation dialog appears'
      },
      {
        action: 'Click Confirm in dialog',
        data: '',
        expected: 'Item is deleted and removed from list'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Item Management',
    title: 'Move item in hierarchy',
    preconditions: 'Multiple items exist',
    labels: ['integration', 'ui'],
    steps: [
      {
        action: 'Drag item to new parent',
        data: 'Drag story under different epic',
        expected: 'Hierarchy is updated visually'
      },
      {
        action: 'Refresh page',
        data: '',
        expected: 'Hierarchy change persists'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Item Management',
    title: 'Assign priority to item',
    preconditions: 'Item and priorities exist',
    labels: ['regression', 'ui'],
    steps: [
      {
        action: 'Open item detail view',
        data: '',
        expected: 'Item details displayed'
      },
      {
        action: 'Click priority dropdown',
        data: '',
        expected: 'Priority options displayed'
      },
      {
        action: 'Select High priority',
        data: '',
        expected: 'Priority updated, High icon shown'
      }
    ]
  },

  // UI Tests > Workflow Designer
  {
    folderPath: 'UI Tests/Workflow Designer',
    title: 'Create new workflow',
    preconditions: 'User has admin access',
    labels: ['smoke', 'ui'],
    steps: [
      {
        action: 'Navigate to Workflow Designer',
        data: 'URL: /admin (Workflows tab)',
        expected: 'Workflow list is displayed'
      },
      {
        action: 'Click Create Workflow button',
        data: '',
        expected: 'Create workflow modal opens'
      },
      {
        action: 'Enter workflow name',
        data: 'Name: Development Workflow',
        expected: 'Name is entered'
      },
      {
        action: 'Click Save',
        data: '',
        expected: 'Workflow created and opens in designer'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Workflow Designer',
    title: 'Add status to workflow',
    preconditions: 'Workflow exists',
    labels: ['regression', 'ui'],
    steps: [
      {
        action: 'Open workflow in designer',
        data: '',
        expected: 'Workflow designer interface displayed'
      },
      {
        action: 'Add new status',
        data: 'Status: Code Review',
        expected: 'Status appears in workflow'
      },
      {
        action: 'Save workflow',
        data: '',
        expected: 'Changes saved successfully'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Workflow Designer',
    title: 'Create transition between statuses',
    preconditions: 'Workflow with multiple statuses exists',
    labels: ['integration', 'ui'],
    steps: [
      {
        action: 'Drag from one status to another',
        data: 'From: In Progress, To: Code Review',
        expected: 'Transition arrow appears'
      },
      {
        action: 'Save workflow',
        data: '',
        expected: 'Transition is saved'
      },
      {
        action: 'Test transition on item',
        data: 'Change item status using transition',
        expected: 'Status change allowed via transition'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Workflow Designer',
    title: 'Delete workflow',
    preconditions: 'Workflow exists not assigned to workspace',
    labels: ['regression', 'ui'],
    steps: [
      {
        action: 'Click delete on workflow',
        data: '',
        expected: 'Confirmation dialog appears'
      },
      {
        action: 'Confirm deletion',
        data: '',
        expected: 'Workflow deleted from list'
      }
    ]
  },

  // Additional edge cases and integration tests
  {
    folderPath: 'API Tests/Item APIs',
    title: 'Custom field values persist on item',
    preconditions: 'Custom fields defined, item exists',
    labels: ['integration', 'api'],
    steps: [
      {
        action: 'Create item with custom field values',
        data: '{"title": "Test", "custom_field_values": {"Story Points": 8}}',
        expected: 'Item created with custom fields'
      },
      {
        action: 'Get item details',
        data: 'GET /api/items/{id}',
        expected: 'Custom field values returned correctly'
      },
      {
        action: 'Update custom field value',
        data: 'PUT with new Story Points value: 13',
        expected: 'Custom field updated'
      }
    ]
  },
  {
    folderPath: 'UI Tests/Item Management',
    title: 'Search items by VQL',
    preconditions: 'Multiple items exist with different properties',
    labels: ['integration', 'ui'],
    steps: [
      {
        action: 'Open search/filter interface',
        data: '',
        expected: 'VQL search box displayed'
      },
      {
        action: 'Enter VQL query',
        data: 'Query: priority = "High" AND status = "In Progress"',
        expected: 'Query is entered'
      },
      {
        action: 'Execute search',
        data: '',
        expected: 'Only items matching criteria displayed'
      }
    ]
  },
  {
    folderPath: 'Authentication & Security/Permission Tests',
    title: 'Workspace roles restrict access',
    preconditions: 'User has viewer role on workspace',
    labels: ['critical', 'ui'],
    steps: [
      {
        action: 'Login as viewer user',
        data: '',
        expected: 'User logged in'
      },
      {
        action: 'Navigate to workspace',
        data: '',
        expected: 'Items are visible'
      },
      {
        action: 'Attempt to create item',
        data: '',
        expected: 'Create button disabled or not visible'
      },
      {
        action: 'Attempt to edit item',
        data: '',
        expected: 'Edit action not allowed'
      }
    ]
  }
];

// Test Sets - Collections of test cases for execution
export const testSets = [
  {
    name: 'Smoke Test Suite',
    description: 'Critical smoke tests that must pass before any release',
    milestone: 'SOFT:MVP Release',
    labelFilter: 'smoke', // Will include all test cases with 'smoke' label
    testCases: [] // Will be populated with smoke-labeled tests
  },
  {
    name: 'Sprint Regression Suite',
    description: 'Comprehensive regression tests for sprint validation',
    milestone: 'SOFT:MVP Release',
    labelFilter: 'regression',
    testCases: []
  },
  {
    name: 'API Validation Suite',
    description: 'Complete API endpoint testing',
    milestone: 'SOFT:API v2 Release',
    labelFilter: 'api',
    testCases: []
  },
  {
    name: 'Pre-release Test Suite',
    description: 'Full test suite before production release',
    milestone: 'SOFT:MVP Release',
    labelFilter: 'critical', // All critical tests
    testCases: []
  }
];

// Test Run Templates - Reusable test execution templates
export const testRunTemplates = [
  {
    testSet: 'Smoke Test Suite',
    name: 'Daily Smoke Tests',
    description: 'Daily smoke test execution template'
  },
  {
    testSet: 'Sprint Regression Suite',
    name: 'Weekly Regression',
    description: 'Weekly regression test execution'
  },
  {
    testSet: 'Pre-release Test Suite',
    name: 'Release Candidate Validation',
    description: 'Pre-release validation template'
  },
  {
    testSet: 'API Validation Suite',
    name: 'API Regression Tests',
    description: 'API-focused regression testing'
  }
];

// Test Case to Requirement Links - Traceability mapping
export const testCaseLinks = [
  // Authentication & Security Tests → Authentication work items
  {
    testCasePath: 'Authentication & Security/Login Tests',
    testCaseTitle: 'Successful login with valid credentials',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement authentication flow'
  },
  {
    testCasePath: 'Authentication & Security/Login Tests',
    testCaseTitle: 'Login fails with invalid password',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement authentication flow'
  },
  {
    testCasePath: 'Authentication & Security/Login Tests',
    testCaseTitle: 'Logout functionality',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement authentication flow'
  },
  {
    testCasePath: 'Authentication & Security/Login Tests',
    testCaseTitle: 'Session persistence across page refresh',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement authentication flow'
  },
  {
    testCasePath: 'Authentication & Security/Permission Tests',
    testCaseTitle: 'Workspace admin can create items',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Setup React Native project structure'
  },
  {
    testCasePath: 'Authentication & Security/Permission Tests',
    testCaseTitle: 'Non-admin cannot access admin settings',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Setup React Native project structure'
  },
  {
    testCasePath: 'Authentication & Security/Permission Tests',
    testCaseTitle: 'Workspace roles restrict access',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Setup React Native project structure'
  },

  // API Tests → API work items
  {
    testCasePath: 'API Tests/Workspace APIs',
    testCaseTitle: 'Create workspace via API',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'API v2 with GraphQL Support'
  },
  {
    testCasePath: 'API Tests/Workspace APIs',
    testCaseTitle: 'Get workspace by ID',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'API v2 with GraphQL Support'
  },
  {
    testCasePath: 'API Tests/Workspace APIs',
    testCaseTitle: 'Update workspace via API',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'API v2 with GraphQL Support'
  },
  {
    testCasePath: 'API Tests/Workspace APIs',
    testCaseTitle: 'Delete workspace via API',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'API v2 with GraphQL Support'
  },
  {
    testCasePath: 'API Tests/Workspace APIs',
    testCaseTitle: 'List all workspaces',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'API v2 with GraphQL Support'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Create top-level item',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Create child item with parent',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Update item title',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Delete item cascades to children',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Filter items by workspace',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Get item descendants',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Get item tree hierarchy',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },
  {
    testCasePath: 'API Tests/Item APIs',
    testCaseTitle: 'Custom field values persist on item',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Implement GraphQL resolvers'
  },

  // UI Tests → UI work items
  {
    testCasePath: 'UI Tests/Item Management',
    testCaseTitle: 'Create item via UI',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Build dashboard and navigation'
  },
  {
    testCasePath: 'UI Tests/Item Management',
    testCaseTitle: 'Inline edit item title',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Build dashboard and navigation'
  },
  {
    testCasePath: 'UI Tests/Item Management',
    testCaseTitle: 'Delete item with confirmation',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Build dashboard and navigation'
  },
  {
    testCasePath: 'UI Tests/Item Management',
    testCaseTitle: 'Move item in hierarchy',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Build dashboard and navigation'
  },
  {
    testCasePath: 'UI Tests/Item Management',
    testCaseTitle: 'Assign priority to item',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Build dashboard and navigation'
  },
  {
    testCasePath: 'UI Tests/Item Management',
    testCaseTitle: 'Search items by VQL',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Build dashboard and navigation'
  },

  // Workflow Tests → Workflow items (link to setup story)
  {
    testCasePath: 'UI Tests/Workflow Designer',
    testCaseTitle: 'Create new workflow',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Setup React Native project structure'
  },
  {
    testCasePath: 'UI Tests/Workflow Designer',
    testCaseTitle: 'Add status to workflow',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Setup React Native project structure'
  },
  {
    testCasePath: 'UI Tests/Workflow Designer',
    testCaseTitle: 'Create transition between statuses',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Setup React Native project structure'
  },
  {
    testCasePath: 'UI Tests/Workflow Designer',
    testCaseTitle: 'Delete workflow',
    requirementWorkspace: 'SOFT',
    requirementTitle: 'Setup React Native project structure'
  }
];

// Asset Management Sets
export const assetSets = [
  {
    name: 'IT Infrastructure',
    description: 'Hardware, software, and network equipment'
  },
  {
    name: 'Office Equipment',
    description: 'Desks, chairs, monitors, and office supplies'
  }
];

// Asset Types (per set)
export const assetTypes = {
  'IT Infrastructure': [
    { name: 'Server', icon: 'Server', color: '#3b82f6', description: 'Physical or virtual servers' },
    { name: 'Laptop', icon: 'Laptop', color: '#8b5cf6', description: 'Employee laptops' },
    { name: 'Network Device', icon: 'Network', color: '#10b981', description: 'Routers, switches, firewalls' },
    { name: 'Software License', icon: 'FileCode', color: '#f59e0b', description: 'Software licenses and subscriptions' }
  ],
  'Office Equipment': [
    { name: 'Monitor', icon: 'Monitor', color: '#6366f1', description: 'Computer monitors' },
    { name: 'Desk', icon: 'Box', color: '#78716c', description: 'Office desks' },
    { name: 'Chair', icon: 'Armchair', color: '#64748b', description: 'Office chairs' }
  ]
};

// Asset Type Custom Fields - Assign custom fields to specific asset types
export const assetTypeFields = {
  'IT Infrastructure': {
    'Laptop': ['Owner']  // Laptops have an Owner field
  }
};

// Asset Categories (hierarchical, per set)
export const assetCategories = {
  'IT Infrastructure': [
    {
      name: 'Data Center',
      description: 'Data center equipment',
      children: [
        { name: 'Production', description: 'Production servers' },
        { name: 'Development', description: 'Development and test servers' },
        { name: 'Networking', description: 'Network infrastructure' }
      ]
    },
    {
      name: 'End User Devices',
      description: 'Employee devices',
      children: [
        { name: 'Engineering', description: 'Engineering team devices' },
        { name: 'Sales', description: 'Sales team devices' },
        { name: 'Support', description: 'Support team devices' }
      ]
    },
    {
      name: 'Software',
      description: 'Software and licenses',
      children: [
        { name: 'Development Tools', description: 'IDEs, build tools, etc.' },
        { name: 'Productivity', description: 'Office and collaboration software' }
      ]
    }
  ],
  'Office Equipment': [
    {
      name: 'Floor 1',
      description: 'First floor equipment',
      children: [
        { name: 'Conference Room A', description: '' },
        { name: 'Open Space', description: '' }
      ]
    },
    {
      name: 'Floor 2',
      description: 'Second floor equipment'
    }
  ]
};

// Assets (per set, referencing types and categories by name)
export const assets = {
  'IT Infrastructure': [
    // Production Servers
    { title: 'PROD-WEB-01', type: 'Server', category: 'Data Center/Production', description: 'Primary web server - nginx load balancer', asset_tag: 'SRV-001' },
    { title: 'PROD-WEB-02', type: 'Server', category: 'Data Center/Production', description: 'Secondary web server - nginx load balancer', asset_tag: 'SRV-002' },
    { title: 'PROD-WEB-03', type: 'Server', category: 'Data Center/Production', description: 'Tertiary web server - nginx load balancer', asset_tag: 'SRV-003' },
    { title: 'PROD-APP-01', type: 'Server', category: 'Data Center/Production', description: 'Application server - Node.js', asset_tag: 'SRV-004' },
    { title: 'PROD-APP-02', type: 'Server', category: 'Data Center/Production', description: 'Application server - Node.js', asset_tag: 'SRV-005' },
    { title: 'PROD-APP-03', type: 'Server', category: 'Data Center/Production', description: 'Application server - Node.js', asset_tag: 'SRV-006' },
    { title: 'PROD-DB-01', type: 'Server', category: 'Data Center/Production', description: 'Primary PostgreSQL database', asset_tag: 'SRV-007' },
    { title: 'PROD-DB-02', type: 'Server', category: 'Data Center/Production', description: 'PostgreSQL replica', asset_tag: 'SRV-008' },
    { title: 'PROD-REDIS-01', type: 'Server', category: 'Data Center/Production', description: 'Redis cache cluster node 1', asset_tag: 'SRV-009' },
    { title: 'PROD-REDIS-02', type: 'Server', category: 'Data Center/Production', description: 'Redis cache cluster node 2', asset_tag: 'SRV-010' },
    { title: 'PROD-ELASTIC-01', type: 'Server', category: 'Data Center/Production', description: 'Elasticsearch node 1', asset_tag: 'SRV-011' },
    { title: 'PROD-ELASTIC-02', type: 'Server', category: 'Data Center/Production', description: 'Elasticsearch node 2', asset_tag: 'SRV-012' },
    // Development Servers
    { title: 'DEV-APP-01', type: 'Server', category: 'Data Center/Development', description: 'Development application server', asset_tag: 'SRV-013' },
    { title: 'DEV-DB-01', type: 'Server', category: 'Data Center/Development', description: 'Development database server', asset_tag: 'SRV-014' },
    { title: 'STAGE-APP-01', type: 'Server', category: 'Data Center/Development', description: 'Staging application server', asset_tag: 'SRV-015' },
    { title: 'STAGE-DB-01', type: 'Server', category: 'Data Center/Development', description: 'Staging database server', asset_tag: 'SRV-016' },
    { title: 'CI-RUNNER-01', type: 'Server', category: 'Data Center/Development', description: 'GitLab CI runner', asset_tag: 'SRV-017' },
    { title: 'CI-RUNNER-02', type: 'Server', category: 'Data Center/Development', description: 'GitLab CI runner', asset_tag: 'SRV-018' },
    // Network Equipment
    { title: 'CORE-SW-01', type: 'Network Device', category: 'Data Center/Networking', description: 'Core switch - Cisco Nexus 9000', asset_tag: 'NET-001' },
    { title: 'CORE-SW-02', type: 'Network Device', category: 'Data Center/Networking', description: 'Core switch - Cisco Nexus 9000', asset_tag: 'NET-002' },
    { title: 'DIST-SW-01', type: 'Network Device', category: 'Data Center/Networking', description: 'Distribution switch - Floor 1', asset_tag: 'NET-003' },
    { title: 'DIST-SW-02', type: 'Network Device', category: 'Data Center/Networking', description: 'Distribution switch - Floor 2', asset_tag: 'NET-004' },
    { title: 'FW-01', type: 'Network Device', category: 'Data Center/Networking', description: 'Primary firewall - Palo Alto PA-3200', asset_tag: 'NET-005' },
    { title: 'FW-02', type: 'Network Device', category: 'Data Center/Networking', description: 'Secondary firewall - Palo Alto PA-3200', asset_tag: 'NET-006' },
    { title: 'VPN-01', type: 'Network Device', category: 'Data Center/Networking', description: 'VPN concentrator', asset_tag: 'NET-007' },
    { title: 'LB-01', type: 'Network Device', category: 'Data Center/Networking', description: 'Hardware load balancer - F5 BIG-IP', asset_tag: 'NET-008' },
    { title: 'AP-FLOOR1-01', type: 'Network Device', category: 'Data Center/Networking', description: 'Wireless access point - Floor 1 North', asset_tag: 'NET-009' },
    { title: 'AP-FLOOR1-02', type: 'Network Device', category: 'Data Center/Networking', description: 'Wireless access point - Floor 1 South', asset_tag: 'NET-010' },
    { title: 'AP-FLOOR2-01', type: 'Network Device', category: 'Data Center/Networking', description: 'Wireless access point - Floor 2', asset_tag: 'NET-011' },
    // Engineering Laptops
    { title: 'MacBook Pro 16" M3 Max - John Doe', type: 'Laptop', category: 'End User Devices/Engineering', description: 'Senior Developer laptop - 64GB RAM', asset_tag: 'LT-001', ownerUsername: 'john' },
    { title: 'MacBook Pro 16" M3 Pro - Sarah Chen', type: 'Laptop', category: 'End User Devices/Engineering', description: 'Developer laptop - 32GB RAM', asset_tag: 'LT-002', ownerUsername: 'sarah' },
    { title: 'MacBook Pro 14" M3 Pro - Alex Kim', type: 'Laptop', category: 'End User Devices/Engineering', description: 'Developer laptop - 32GB RAM', asset_tag: 'LT-003', ownerUsername: 'alex' },
    { title: 'MacBook Pro 14" M3 - Emily Wang', type: 'Laptop', category: 'End User Devices/Engineering', description: 'Junior Developer laptop - 16GB RAM', asset_tag: 'LT-004', ownerUsername: 'emily' },
    { title: 'ThinkPad X1 Carbon - Tom Brown', type: 'Laptop', category: 'End User Devices/Engineering', description: 'DevOps Engineer laptop - Linux', asset_tag: 'LT-005', ownerUsername: 'tom' },
    { title: 'ThinkPad P16 - Lisa Park', type: 'Laptop', category: 'End User Devices/Engineering', description: 'Data Engineer laptop - 64GB RAM', asset_tag: 'LT-006', ownerUsername: 'lisa' },
    { title: 'MacBook Pro 16" M3 Max - David Lee', type: 'Laptop', category: 'End User Devices/Engineering', description: 'Tech Lead laptop - 64GB RAM', asset_tag: 'LT-007', ownerUsername: 'david' },
    { title: 'MacBook Pro 14" M3 Pro - Maria Garcia', type: 'Laptop', category: 'End User Devices/Engineering', description: 'QA Engineer laptop - 32GB RAM', asset_tag: 'LT-008', ownerUsername: 'maria' },
    // Sales Laptops
    { title: 'Dell XPS 15 - Mike Wilson', type: 'Laptop', category: 'End User Devices/Sales', description: 'Sales Director laptop', asset_tag: 'LT-009', ownerUsername: 'mike' },
    { title: 'Dell XPS 13 - Jennifer Adams', type: 'Laptop', category: 'End User Devices/Sales', description: 'Account Executive laptop', asset_tag: 'LT-010', ownerUsername: 'jennifer' },
    { title: 'Dell XPS 13 - Robert Taylor', type: 'Laptop', category: 'End User Devices/Sales', description: 'Account Executive laptop', asset_tag: 'LT-011', ownerUsername: 'robert' },
    { title: 'MacBook Air M2 - Amanda White', type: 'Laptop', category: 'End User Devices/Sales', description: 'Sales Rep laptop', asset_tag: 'LT-012', ownerUsername: 'amanda' },
    { title: 'MacBook Air M2 - Chris Martin', type: 'Laptop', category: 'End User Devices/Sales', description: 'Sales Rep laptop', asset_tag: 'LT-013', ownerUsername: 'chris' },
    // Support Laptops
    { title: 'MacBook Pro 14" M3 - Jane Smith', type: 'Laptop', category: 'End User Devices/Support', description: 'Support Manager laptop', asset_tag: 'LT-014', ownerUsername: 'jane' },
    { title: 'MacBook Air M2 - Kevin Johnson', type: 'Laptop', category: 'End User Devices/Support', description: 'Support Engineer laptop', asset_tag: 'LT-015', ownerUsername: 'kevin' },
    { title: 'MacBook Air M2 - Rachel Green', type: 'Laptop', category: 'End User Devices/Support', description: 'Support Engineer laptop', asset_tag: 'LT-016', ownerUsername: 'rachel' },
    { title: 'Dell Latitude 5540 - Brian Miller', type: 'Laptop', category: 'End User Devices/Support', description: 'Support Specialist laptop', asset_tag: 'LT-017', ownerUsername: 'brian' },
    { title: 'Dell Latitude 5540 - Nicole Davis', type: 'Laptop', category: 'End User Devices/Support', description: 'Support Specialist laptop', asset_tag: 'LT-018', ownerUsername: 'nicole' },
    // Development Tools
    { title: 'JetBrains All Products Pack', type: 'Software License', category: 'Software/Development Tools', description: '25 seats - Annual subscription', asset_tag: 'SW-001' },
    { title: 'GitHub Enterprise', type: 'Software License', category: 'Software/Development Tools', description: '50 seats - Source control', asset_tag: 'SW-002' },
    { title: 'GitLab Ultimate', type: 'Software License', category: 'Software/Development Tools', description: 'Self-hosted - CI/CD platform', asset_tag: 'SW-003' },
    { title: 'Docker Business', type: 'Software License', category: 'Software/Development Tools', description: '30 seats - Container platform', asset_tag: 'SW-004' },
    { title: 'Postman Enterprise', type: 'Software License', category: 'Software/Development Tools', description: '20 seats - API development', asset_tag: 'SW-005' },
    { title: 'Figma Organization', type: 'Software License', category: 'Software/Development Tools', description: '15 seats - Design tool', asset_tag: 'SW-006' },
    { title: 'DataDog Pro', type: 'Software License', category: 'Software/Development Tools', description: 'Infrastructure monitoring', asset_tag: 'SW-007' },
    { title: 'PagerDuty Business', type: 'Software License', category: 'Software/Development Tools', description: 'Incident management', asset_tag: 'SW-008' },
    { title: 'Sentry Business', type: 'Software License', category: 'Software/Development Tools', description: 'Error tracking', asset_tag: 'SW-009' },
    // Productivity Software
    { title: 'Microsoft 365 E3', type: 'Software License', category: 'Software/Productivity', description: '75 seats - Office suite', asset_tag: 'SW-010' },
    { title: 'Slack Business+', type: 'Software License', category: 'Software/Productivity', description: 'Unlimited seats - Team communication', asset_tag: 'SW-011' },
    { title: 'Zoom Business', type: 'Software License', category: 'Software/Productivity', description: '50 hosts - Video conferencing', asset_tag: 'SW-012' },
    { title: 'Notion Team', type: 'Software License', category: 'Software/Productivity', description: 'Unlimited seats - Documentation', asset_tag: 'SW-013' },
    { title: 'Confluence Standard', type: 'Software License', category: 'Software/Productivity', description: '75 seats - Wiki', asset_tag: 'SW-014' },
    { title: 'Jira Software Premium', type: 'Software License', category: 'Software/Productivity', description: '50 seats - Project tracking', asset_tag: 'SW-015' },
    { title: '1Password Business', type: 'Software License', category: 'Software/Productivity', description: '75 seats - Password manager', asset_tag: 'SW-016' },
    { title: 'Grammarly Business', type: 'Software License', category: 'Software/Productivity', description: '30 seats - Writing assistant', asset_tag: 'SW-017' },
    { title: 'Calendly Teams', type: 'Software License', category: 'Software/Productivity', description: '20 seats - Scheduling', asset_tag: 'SW-018' },
    { title: 'Loom Business', type: 'Software License', category: 'Software/Productivity', description: '40 seats - Video messaging', asset_tag: 'SW-019' }
  ],
  'Office Equipment': [
    // Floor 1 - Open Space Monitors
    { title: 'Dell U2723QE 27" 4K - Desk 1A', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Engineering', asset_tag: 'MON-001' },
    { title: 'Dell U2723QE 27" 4K - Desk 1A', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Secondary monitor - Engineering', asset_tag: 'MON-002' },
    { title: 'Dell U2723QE 27" 4K - Desk 1B', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Engineering', asset_tag: 'MON-003' },
    { title: 'Dell U2723QE 27" 4K - Desk 1B', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Secondary monitor - Engineering', asset_tag: 'MON-004' },
    { title: 'Dell U2723QE 27" 4K - Desk 1C', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Engineering', asset_tag: 'MON-005' },
    { title: 'Dell U2723QE 27" 4K - Desk 1C', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Secondary monitor - Engineering', asset_tag: 'MON-006' },
    { title: 'LG 27UK850-W 27" 4K - Desk 1D', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Sales', asset_tag: 'MON-007' },
    { title: 'LG 27UK850-W 27" 4K - Desk 1E', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Sales', asset_tag: 'MON-008' },
    { title: 'LG 27UK850-W 27" 4K - Desk 1F', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Sales', asset_tag: 'MON-009' },
    { title: 'Dell P2422H 24" - Desk 1G', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Support', asset_tag: 'MON-010' },
    { title: 'Dell P2422H 24" - Desk 1H', type: 'Monitor', category: 'Floor 1/Open Space', description: 'Primary monitor - Support', asset_tag: 'MON-011' },
    // Floor 1 - Conference Room A
    { title: 'Samsung 75" QN75Q80 Display', type: 'Monitor', category: 'Floor 1/Conference Room A', description: 'Main presentation display', asset_tag: 'MON-012' },
    { title: 'Logitech Rally Bar', type: 'Monitor', category: 'Floor 1/Conference Room A', description: 'Video conferencing system', asset_tag: 'MON-013' },
    // Floor 2 Monitors
    { title: 'Dell U2723QE 27" 4K - Desk 2A', type: 'Monitor', category: 'Floor 2', description: 'Primary monitor - Executive', asset_tag: 'MON-014' },
    { title: 'Dell U2723QE 27" 4K - Desk 2A', type: 'Monitor', category: 'Floor 2', description: 'Secondary monitor - Executive', asset_tag: 'MON-015' },
    { title: 'Dell U2723QE 27" 4K - Desk 2B', type: 'Monitor', category: 'Floor 2', description: 'Primary monitor - HR', asset_tag: 'MON-016' },
    { title: 'Dell U2723QE 27" 4K - Desk 2C', type: 'Monitor', category: 'Floor 2', description: 'Primary monitor - Finance', asset_tag: 'MON-017' },
    { title: 'Samsung 65" Display - Conf Room B', type: 'Monitor', category: 'Floor 2', description: 'Conference room display', asset_tag: 'MON-018' },
    // Floor 1 - Open Space Desks
    { title: 'Uplift V2 Standing Desk - Eng 1', type: 'Desk', category: 'Floor 1/Open Space', description: '72x30 adjustable standing desk', asset_tag: 'DSK-001' },
    { title: 'Uplift V2 Standing Desk - Eng 2', type: 'Desk', category: 'Floor 1/Open Space', description: '72x30 adjustable standing desk', asset_tag: 'DSK-002' },
    { title: 'Uplift V2 Standing Desk - Eng 3', type: 'Desk', category: 'Floor 1/Open Space', description: '72x30 adjustable standing desk', asset_tag: 'DSK-003' },
    { title: 'Uplift V2 Standing Desk - Eng 4', type: 'Desk', category: 'Floor 1/Open Space', description: '60x30 adjustable standing desk', asset_tag: 'DSK-004' },
    { title: 'Uplift V2 Standing Desk - Eng 5', type: 'Desk', category: 'Floor 1/Open Space', description: '60x30 adjustable standing desk', asset_tag: 'DSK-005' },
    { title: 'IKEA Bekant Desk - Sales 1', type: 'Desk', category: 'Floor 1/Open Space', description: '63x31 standard desk', asset_tag: 'DSK-006' },
    { title: 'IKEA Bekant Desk - Sales 2', type: 'Desk', category: 'Floor 1/Open Space', description: '63x31 standard desk', asset_tag: 'DSK-007' },
    { title: 'IKEA Bekant Desk - Sales 3', type: 'Desk', category: 'Floor 1/Open Space', description: '63x31 standard desk', asset_tag: 'DSK-008' },
    { title: 'IKEA Bekant Desk - Support 1', type: 'Desk', category: 'Floor 1/Open Space', description: '63x31 standard desk', asset_tag: 'DSK-009' },
    { title: 'IKEA Bekant Desk - Support 2', type: 'Desk', category: 'Floor 1/Open Space', description: '63x31 standard desk', asset_tag: 'DSK-010' },
    // Floor 1 - Conference Room A
    { title: 'Conference Table - Room A', type: 'Desk', category: 'Floor 1/Conference Room A', description: '12-person boat-shaped conference table', asset_tag: 'DSK-011' },
    // Floor 2 Desks
    { title: 'Executive Desk - CEO Office', type: 'Desk', category: 'Floor 2', description: 'L-shaped executive desk with hutch', asset_tag: 'DSK-012' },
    { title: 'Executive Desk - CFO Office', type: 'Desk', category: 'Floor 2', description: 'L-shaped executive desk', asset_tag: 'DSK-013' },
    { title: 'IKEA Bekant Desk - HR 1', type: 'Desk', category: 'Floor 2', description: '63x31 standard desk', asset_tag: 'DSK-014' },
    { title: 'IKEA Bekant Desk - Finance 1', type: 'Desk', category: 'Floor 2', description: '63x31 standard desk', asset_tag: 'DSK-015' },
    { title: 'Conference Table - Room B', type: 'Desk', category: 'Floor 2', description: '8-person rectangular table', asset_tag: 'DSK-016' },
    // Floor 1 - Open Space Chairs
    { title: 'Herman Miller Aeron - Eng 1', type: 'Chair', category: 'Floor 1/Open Space', description: 'Size C fully loaded', asset_tag: 'CHR-001' },
    { title: 'Herman Miller Aeron - Eng 2', type: 'Chair', category: 'Floor 1/Open Space', description: 'Size B fully loaded', asset_tag: 'CHR-002' },
    { title: 'Herman Miller Aeron - Eng 3', type: 'Chair', category: 'Floor 1/Open Space', description: 'Size B fully loaded', asset_tag: 'CHR-003' },
    { title: 'Herman Miller Aeron - Eng 4', type: 'Chair', category: 'Floor 1/Open Space', description: 'Size B fully loaded', asset_tag: 'CHR-004' },
    { title: 'Herman Miller Aeron - Eng 5', type: 'Chair', category: 'Floor 1/Open Space', description: 'Size B fully loaded', asset_tag: 'CHR-005' },
    { title: 'Steelcase Leap V2 - Sales 1', type: 'Chair', category: 'Floor 1/Open Space', description: 'Ergonomic task chair', asset_tag: 'CHR-006' },
    { title: 'Steelcase Leap V2 - Sales 2', type: 'Chair', category: 'Floor 1/Open Space', description: 'Ergonomic task chair', asset_tag: 'CHR-007' },
    { title: 'Steelcase Leap V2 - Sales 3', type: 'Chair', category: 'Floor 1/Open Space', description: 'Ergonomic task chair', asset_tag: 'CHR-008' },
    { title: 'HON Ignition 2.0 - Support 1', type: 'Chair', category: 'Floor 1/Open Space', description: 'Mid-back task chair', asset_tag: 'CHR-009' },
    { title: 'HON Ignition 2.0 - Support 2', type: 'Chair', category: 'Floor 1/Open Space', description: 'Mid-back task chair', asset_tag: 'CHR-010' },
    // Floor 1 - Conference Room A Chairs
    { title: 'Humanscale Diffrient - Conf A', type: 'Chair', category: 'Floor 1/Conference Room A', description: 'Conference chair 1 of 12', asset_tag: 'CHR-011' },
    { title: 'Humanscale Diffrient - Conf A', type: 'Chair', category: 'Floor 1/Conference Room A', description: 'Conference chair 2 of 12', asset_tag: 'CHR-012' },
    { title: 'Humanscale Diffrient - Conf A', type: 'Chair', category: 'Floor 1/Conference Room A', description: 'Conference chair 3 of 12', asset_tag: 'CHR-013' },
    { title: 'Humanscale Diffrient - Conf A', type: 'Chair', category: 'Floor 1/Conference Room A', description: 'Conference chair 4 of 12', asset_tag: 'CHR-014' },
    // Floor 2 Chairs
    { title: 'Herman Miller Embody - CEO', type: 'Chair', category: 'Floor 2', description: 'Executive chair', asset_tag: 'CHR-015' },
    { title: 'Herman Miller Embody - CFO', type: 'Chair', category: 'Floor 2', description: 'Executive chair', asset_tag: 'CHR-016' },
    { title: 'Steelcase Gesture - HR', type: 'Chair', category: 'Floor 2', description: 'Ergonomic task chair', asset_tag: 'CHR-017' },
    { title: 'Steelcase Gesture - Finance', type: 'Chair', category: 'Floor 2', description: 'Ergonomic task chair', asset_tag: 'CHR-018' }
  ]
};

// Personal Tasks for admin user - dates are relative to current week (daysFromMonday)
export const personalTasks = [
  // Scheduled tasks (will appear on weekly calendar)
  {
    title: 'Review Q1 roadmap priorities',
    description: 'Go through the Q1 roadmap and prioritize features based on customer feedback',
    daysFromMonday: 0,  // Monday
    scheduledTime: '09:00',
    durationMinutes: 60
  },
  {
    title: 'Prepare sprint planning notes',
    description: 'Gather metrics and prepare talking points for sprint planning meeting',
    daysFromMonday: 1,  // Tuesday
    scheduledTime: '14:00',
    durationMinutes: 45
  },
  {
    title: '1:1 with Sarah - engineering sync',
    description: 'Discuss blockers and team capacity for next sprint',
    daysFromMonday: 2,  // Wednesday
    scheduledTime: '10:30',
    durationMinutes: 30
  },
  {
    title: 'Review pull requests',
    description: 'Catch up on open PRs and provide feedback to the team',
    daysFromMonday: 3,  // Thursday
    scheduledTime: '11:00',
    durationMinutes: 90
  },
  {
    title: 'Write weekly status update',
    description: 'Summarize progress and blockers for stakeholders',
    daysFromMonday: 4,  // Friday
    scheduledTime: '15:00',
    durationMinutes: 30
  },
  {
    title: 'Team standup',
    description: 'Daily standup with the development team',
    daysFromMonday: 0,  // Monday
    scheduledTime: '09:30',
    durationMinutes: 15
  },
  {
    title: 'Customer demo preparation',
    description: 'Prepare demo environment and talking points for enterprise customer',
    daysFromMonday: 2,  // Wednesday
    scheduledTime: '14:00',
    durationMinutes: 60
  },
  // Unscheduled tasks (have due dates but not on calendar)
  {
    title: 'Update documentation for API changes',
    description: 'Document the new authentication endpoints and update API reference',
    dueDaysFromMonday: 4  // Due Friday, but not scheduled on calendar
  },
  {
    title: 'Research competitor features',
    description: 'Look at recent updates from competitors and prepare analysis',
    dueDaysFromMonday: 6  // Due Sunday
  },
  {
    title: 'Clean up Jira backlog',
    description: 'Archive old tickets and update priorities for next quarter'
    // No due date, no schedule - just a task in the list
  },
  {
    title: 'Expense report submission',
    description: 'Submit Q4 expense reports before deadline',
    dueDaysFromMonday: 3  // Due Thursday
  },
  {
    title: 'Review security audit findings',
    description: 'Go through the latest security audit report and create action items'
    // No due date
  }
];
