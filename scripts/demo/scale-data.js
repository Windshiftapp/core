/**
 * Scale Demo Data Generator
 *
 * Generates 22 workspaces with 500+ hierarchical items each using template pools
 * and a seeded PRNG for deterministic output. Descriptions are multi-paragraph
 * with domain-specific terminology.
 */

// ─── Seeded PRNG (mulberry32) ────────────────────────────────────────────────

export function createRNG(seed) {
  let s = seed | 0;
  return function () {
    s = (s + 0x6d2b79f5) | 0;
    let t = Math.imul(s ^ (s >>> 15), 1 | s);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function pick(rng, arr) {
  return arr[Math.floor(rng() * arr.length)];
}

function pickN(rng, arr, min, max) {
  const n = min + Math.floor(rng() * (max - min + 1));
  const shuffled = [...arr].sort(() => rng() - 0.5);
  return shuffled.slice(0, Math.min(n, arr.length));
}

function rangeInt(rng, min, max) {
  return min + Math.floor(rng() * (max - min + 1));
}

// ─── Template Pools ──────────────────────────────────────────────────────────

const actionVerbs = [
  'Implement', 'Design', 'Build', 'Create', 'Develop', 'Configure', 'Integrate',
  'Optimize', 'Refactor', 'Migrate', 'Deploy', 'Monitor', 'Automate', 'Validate',
  'Establish', 'Enhance', 'Extend', 'Consolidate', 'Standardize', 'Provision'
];

const bugVerbs = [
  'Fix', 'Resolve', 'Debug', 'Patch', 'Address', 'Correct', 'Repair'
];

const technicalAdjectives = [
  'scalable', 'high-performance', 'fault-tolerant', 'distributed', 'real-time',
  'event-driven', 'asynchronous', 'modular', 'containerized', 'serverless',
  'microservice-based', 'cloud-native', 'multi-tenant', 'cross-platform',
  'resilient', 'observable', 'idempotent', 'stateless', 'declarative', 'composable'
];

const descriptionSections = {
  background: [
    'This initiative addresses a long-standing gap in our current architecture that has been identified through multiple quarterly reviews and stakeholder feedback sessions.',
    'Based on analysis of production metrics over the last three months, we have identified a significant opportunity to improve system reliability and user experience.',
    'Following discussions with the engineering leads and product management, this work has been prioritized to align with our Q2 strategic objectives.',
    'Our monitoring dashboards have consistently shown degraded performance in this area, and user surveys confirm that this is impacting customer satisfaction scores.',
    'This effort stems from our recent architecture review where the team identified several critical areas requiring modernization to support our growth targets.',
    'Customer feedback through support tickets and NPS surveys has highlighted this as a top priority area for improvement.',
    'The technical debt accumulated in this area has reached a critical threshold, making further feature development increasingly difficult and error-prone.',
    'Competitive analysis shows that industry leaders have already adopted similar capabilities, making this a strategic necessity to maintain our market position.'
  ],
  technicalApproach: [
    'The implementation will follow a phased approach, starting with core infrastructure changes and progressively layering in feature capabilities. We will use feature flags to enable gradual rollout.',
    'We plan to leverage existing service abstractions where possible, extending them with new interfaces to support the required functionality. All changes will be backward-compatible.',
    'The technical approach involves introducing a new abstraction layer that decouples the presentation logic from the underlying data processing pipeline, enabling independent scaling.',
    'Our strategy involves decomposing the monolithic component into smaller, independently deployable services with well-defined API contracts and shared schema definitions.',
    'The solution architecture employs an event-sourcing pattern with CQRS to ensure eventual consistency across distributed components while maintaining audit trail capabilities.',
    'We will adopt a test-driven development approach, establishing comprehensive integration test suites before making structural changes to the existing codebase.',
    'The implementation leverages the adapter pattern to abstract third-party dependencies, enabling easier testing and future provider migration without core logic changes.',
    'We will use a strangler fig migration strategy, gradually routing traffic to the new implementation while maintaining the legacy system as a fallback.'
  ],
  acceptanceCriteria: [
    'All existing automated tests must continue to pass without modification. New functionality must have minimum 80% code coverage with unit and integration tests.',
    'The solution must handle peak load of 10x normal traffic without degradation. P99 latency must remain under 200ms for all API endpoints.',
    'Documentation must be updated to reflect architectural changes. Runbooks must be created for all new operational procedures.',
    'The implementation must pass security review including OWASP top 10 assessment. All user inputs must be validated and sanitized.',
    'Rollback procedures must be documented and tested. The system must support instant rollback to the previous version without data loss.',
    'Performance benchmarks must show at minimum 20% improvement over the current implementation. Memory usage must not increase by more than 10%.',
    'The feature must be accessible (WCAG 2.1 AA compliance) and support internationalization for all supported locales.',
    'Error handling must be comprehensive with structured logging. All failures must produce actionable error messages with correlation IDs.'
  ],
  risksMitigations: [
    'Risk: Data migration could cause temporary inconsistencies. Mitigation: Implement dual-write pattern during transition period with automated reconciliation checks.',
    'Risk: Third-party API rate limits could impact throughput. Mitigation: Implement circuit breaker pattern with exponential backoff and request queuing.',
    'Risk: Schema changes could break downstream consumers. Mitigation: Use versioned API contracts with deprecation notices and backward-compatible field additions.',
    'Risk: Performance regression during refactoring. Mitigation: Establish baseline benchmarks and run continuous performance testing in CI pipeline.',
    'Risk: Team knowledge concentration. Mitigation: Conduct pair programming sessions and maintain detailed architecture decision records (ADRs).'
  ]
};

const commentTemplates = {
  review: [
    'I reviewed the implementation and it looks solid overall. A few minor suggestions:\n\n1. Consider extracting the validation logic into a separate utility function for reusability\n2. The error messages could be more descriptive for debugging purposes\n3. We should add a retry mechanism for transient failures\n\nOtherwise, great work on this.',
    'Code review complete. The architecture follows our established patterns well. One thing I noticed is that we might want to add caching at the service layer to avoid redundant database calls. The current implementation makes N+1 queries in the list endpoint.',
    'Reviewed the PR. The test coverage is good but I think we are missing edge cases around concurrent access. Can we add a test that simulates multiple users updating the same resource simultaneously?',
    'LGTM with minor comments. The logging is comprehensive which will help with debugging. Suggest we also add structured metrics for the new endpoints so we can track adoption.',
    'Good implementation. I would suggest using a builder pattern for the complex object construction in the factory method - it would make the code more readable and easier to extend in the future.'
  ],
  statusUpdate: [
    'Status update: Made good progress today. The core logic is implemented and passing all unit tests. Still need to wire up the API layer and add integration tests. Expecting to have a PR ready by end of week.',
    'Quick update - hit a blocker with the authentication middleware not properly propagating the user context to downstream services. Created a separate task to address this. Working on a temporary workaround.',
    'Completed the database migration scripts and tested them against a copy of production data. No issues found. Moving on to the service layer implementation.',
    'Sprint check-in: The feature is about 70% complete. API endpoints are done, frontend components are in progress. Main remaining work is the notification system integration.',
    'Update: Finished the proof of concept and the approach works well. Performance numbers look promising - seeing 3x improvement in query times with the new indexing strategy.'
  ],
  question: [
    'Question: Should we maintain backward compatibility with the v1 API or can we make breaking changes? The new data model does not map cleanly to the existing response format.',
    'I am unsure about the expected behavior when a user has conflicting permissions from multiple roles. Should the most permissive role win, or should we require explicit permission for each action?',
    'What is our strategy for handling timezone-sensitive data in this feature? Should we store everything in UTC and convert at the presentation layer, or respect the user\'s configured timezone?',
    'Do we have a preference for the caching strategy here? I see we use both Redis and in-memory caching in different parts of the codebase. Which is more appropriate for this use case?',
    'Should this endpoint support pagination from the start, or can we add it later? The dataset could grow to tens of thousands of records for enterprise customers.'
  ],
  benchmark: [
    'Benchmark results from the staging environment:\n\n- GET /api/resources: 45ms avg (was 180ms) - 75% improvement\n- POST /api/resources: 62ms avg (was 95ms) - 35% improvement\n- Batch import (1000 records): 3.2s (was 12.8s) - 75% improvement\n\nAll within our target SLA. Memory usage stayed flat during load testing.',
    'Performance testing results:\n\n- Concurrent users: 500 (target was 200)\n- P99 latency: 185ms (target < 200ms)\n- Error rate: 0.02% (target < 0.1%)\n- CPU utilization: 45% peak\n\nThe new connection pooling strategy is working well.',
    'Load test complete. Key findings:\n\n- Throughput: 2,400 requests/sec (up from 800)\n- Database connections: stable at 25 (was spiking to 100+)\n- Cache hit ratio: 92%\n\nThe main bottleneck is now the serialization layer. Filed a follow-up task to optimize.',
    'Profiling results after optimization:\n\n- Memory allocation reduced by 40%\n- GC pause times down from 15ms to 3ms\n- Average response time: 28ms (down from 95ms)\n\nThe object pooling pattern made the biggest difference.'
  ],
  blocker: [
    'Blocked: The upstream dependency service is returning inconsistent responses. I have filed a bug with the platform team. In the meantime, I have added defensive error handling so our service degrades gracefully.',
    'Heads up - the CI pipeline is failing due to a flaky test in the integration suite. The test depends on external service timing. I will fix it today but this blocks the merge.',
    'We discovered a schema conflict with the latest migration. The column type was changed in another branch that merged first. Need to reconcile before we can proceed.',
    'Blocked on infrastructure. The new service requires a dedicated message queue but the provisioning request is still pending with the platform team. ETA is end of day tomorrow.'
  ],
  suggestion: [
    'Thought: We could simplify this significantly by using the existing event bus instead of building a custom notification pipeline. The event bus already handles retry logic and dead letter queues.',
    'Looking at the implementation, I think we should consider using a write-ahead log pattern here. It would give us exactly-once delivery semantics without the complexity of distributed transactions.',
    'Have we considered using GraphQL for this endpoint? The clients need different subsets of the data, and the current REST approach leads to over-fetching or requires many specialized endpoints.',
    'I noticed we are building our own rate limiter. Should we consider using the token bucket implementation from our shared library? It is already battle-tested and supports distributed rate limiting.',
    'Might be worth looking into server-sent events instead of polling for the real-time updates. It would reduce our API load significantly and provide better UX with instant updates.'
  ]
};

// ─── Workspace Definitions ───────────────────────────────────────────────────

const workspaceDefinitions = [
  {
    name: 'Cloud Infrastructure',
    key: 'CLOUD',
    description: 'Cloud platform engineering, infrastructure automation, and reliability',
    domain: ['Kubernetes', 'Terraform', 'AWS', 'GCP', 'Azure', 'Docker', 'Helm', 'Istio', 'Prometheus', 'Grafana'],
    nouns: ['cluster', 'node', 'pod', 'service mesh', 'load balancer', 'VPC', 'subnet', 'firewall rule', 'IAM policy', 'certificate'],
    epicPrefixes: ['Infrastructure', 'Platform', 'Networking', 'Observability', 'Security', 'Cost Optimization', 'Disaster Recovery']
  },
  {
    name: 'E-Commerce Platform',
    key: 'ECOM',
    description: 'Online marketplace, shopping cart, payments, and order management',
    domain: ['Stripe', 'PayPal', 'Elasticsearch', 'Redis', 'CDN', 'A/B testing', 'recommendation engine', 'inventory API'],
    nouns: ['checkout flow', 'product catalog', 'shopping cart', 'payment gateway', 'order pipeline', 'shipping integration', 'tax calculator', 'wishlist', 'coupon engine', 'storefront'],
    epicPrefixes: ['Checkout', 'Catalog', 'Payments', 'Orders', 'Search', 'Personalization', 'Fulfillment']
  },
  {
    name: 'Healthcare Systems',
    key: 'HLTH',
    description: 'Patient management, clinical workflows, and healthcare compliance',
    domain: ['HL7 FHIR', 'HIPAA', 'EHR', 'DICOM', 'ICD-10', 'SNOMED CT', 'CDA', 'patient portal'],
    nouns: ['patient record', 'clinical workflow', 'appointment scheduler', 'prescription system', 'lab results interface', 'insurance claim', 'consent form', 'care plan', 'referral system', 'audit trail'],
    epicPrefixes: ['Patient Portal', 'Clinical', 'Compliance', 'Interoperability', 'Telehealth', 'Pharmacy', 'Billing']
  },
  {
    name: 'Financial Services',
    key: 'FINSV',
    description: 'Banking platform, transaction processing, and regulatory compliance',
    domain: ['PCI DSS', 'SOX', 'KYC', 'AML', 'SWIFT', 'ACH', 'ISO 20022', 'ledger system'],
    nouns: ['transaction engine', 'account ledger', 'fraud detection rule', 'compliance report', 'audit log', 'settlement batch', 'risk model', 'portfolio tracker', 'wire transfer', 'reconciliation'],
    epicPrefixes: ['Payments', 'Risk Management', 'Compliance', 'Account Management', 'Fraud Detection', 'Reporting', 'Onboarding']
  },
  {
    name: 'Education Platform',
    key: 'EDUC',
    description: 'Learning management, course delivery, and student engagement',
    domain: ['LTI', 'SCORM', 'xAPI', 'adaptive learning', 'video streaming', 'WebRTC', 'plagiarism detection'],
    nouns: ['course module', 'assessment engine', 'grade book', 'enrollment system', 'content library', 'discussion forum', 'quiz builder', 'certificate generator', 'progress tracker', 'syllabus'],
    epicPrefixes: ['Course Management', 'Assessment', 'Student Portal', 'Analytics', 'Content Authoring', 'Live Sessions', 'Certification']
  },
  {
    name: 'Supply Chain Management',
    key: 'SUPPLY',
    description: 'Logistics, inventory tracking, and supplier management',
    domain: ['EDI', 'RFID', 'WMS', 'TMS', 'barcode scanning', 'route optimization', 'demand forecasting'],
    nouns: ['warehouse module', 'shipment tracker', 'purchase order', 'vendor portal', 'inventory reconciliation', 'demand forecast', 'route plan', 'receiving dock', 'pick list', 'bill of lading'],
    epicPrefixes: ['Warehouse', 'Logistics', 'Procurement', 'Inventory', 'Supplier Portal', 'Forecasting', 'Returns']
  },
  {
    name: 'Human Resources',
    key: 'HRMS',
    description: 'Employee lifecycle, payroll, benefits, and talent management',
    domain: ['ATS', 'HRIS', 'benefits API', 'payroll engine', 'PTO tracking', 'performance review', 'compensation modeling'],
    nouns: ['onboarding workflow', 'performance review cycle', 'benefits enrollment', 'payroll batch', 'time-off request', 'job requisition', 'compensation plan', 'org chart', 'training module', 'exit interview'],
    epicPrefixes: ['Recruitment', 'Onboarding', 'Payroll', 'Benefits', 'Performance', 'Learning & Development', 'Employee Self-Service']
  },
  {
    name: 'Data Analytics Platform',
    key: 'DATAP',
    description: 'Data pipelines, warehousing, visualization, and business intelligence',
    domain: ['Apache Spark', 'dbt', 'Snowflake', 'BigQuery', 'Airflow', 'Kafka', 'Redshift', 'Looker', 'Tableau'],
    nouns: ['ETL pipeline', 'data warehouse', 'dashboard', 'metric definition', 'data model', 'scheduler job', 'data quality check', 'dimension table', 'fact table', 'report builder'],
    epicPrefixes: ['Data Pipeline', 'Warehouse', 'Visualization', 'Data Quality', 'Self-Service Analytics', 'Real-Time Streaming', 'Governance']
  },
  {
    name: 'Mobile Development',
    key: 'MOBIL',
    description: 'iOS and Android app development, push notifications, and mobile UX',
    domain: ['React Native', 'Swift', 'Kotlin', 'Flutter', 'Firebase', 'push notifications', 'deep linking', 'app store'],
    nouns: ['navigation stack', 'offline sync', 'push notification', 'biometric auth', 'deep link handler', 'app state manager', 'gesture handler', 'camera module', 'location service', 'in-app purchase'],
    epicPrefixes: ['iOS', 'Android', 'Cross-Platform', 'Offline Mode', 'Push Notifications', 'App Performance', 'Accessibility']
  },
  {
    name: 'DevOps & CI/CD',
    key: 'DEVOP',
    description: 'Build pipelines, deployment automation, and developer experience',
    domain: ['GitHub Actions', 'Jenkins', 'ArgoCD', 'Flux', 'Tekton', 'Spinnaker', 'SonarQube', 'Artifactory'],
    nouns: ['build pipeline', 'deployment manifest', 'release train', 'canary deployment', 'rollback procedure', 'artifact registry', 'environment config', 'secret manager', 'pipeline trigger', 'quality gate'],
    epicPrefixes: ['CI Pipeline', 'CD Automation', 'Environment Management', 'Release Process', 'Developer Tooling', 'Infrastructure as Code', 'Monitoring']
  },
  {
    name: 'Cybersecurity',
    key: 'SECUR',
    description: 'Security operations, vulnerability management, and incident response',
    domain: ['SIEM', 'SOAR', 'WAF', 'IDS/IPS', 'zero trust', 'OAuth 2.0', 'SAML', 'vulnerability scanner'],
    nouns: ['security policy', 'threat model', 'vulnerability report', 'incident playbook', 'access control list', 'encryption key', 'audit finding', 'penetration test', 'compliance check', 'security baseline'],
    epicPrefixes: ['Vulnerability Management', 'Incident Response', 'Access Control', 'Compliance', 'Threat Detection', 'Security Automation', 'Zero Trust']
  },
  {
    name: 'IoT Platform',
    key: 'IOTP',
    description: 'Device management, telemetry ingestion, and edge computing',
    domain: ['MQTT', 'CoAP', 'OPC-UA', 'edge computing', 'digital twin', 'device provisioning', 'OTA updates'],
    nouns: ['device registry', 'telemetry pipeline', 'firmware update', 'edge gateway', 'sensor data model', 'command queue', 'device twin', 'alert rule', 'geofence', 'fleet dashboard'],
    epicPrefixes: ['Device Management', 'Telemetry', 'Edge Computing', 'OTA Updates', 'Fleet Monitoring', 'Digital Twins', 'Protocol Support']
  },
  {
    name: 'Machine Learning',
    key: 'MLENG',
    description: 'ML model development, training infrastructure, and model serving',
    domain: ['PyTorch', 'TensorFlow', 'MLflow', 'Kubeflow', 'feature store', 'model registry', 'A/B testing', 'ONNX'],
    nouns: ['training pipeline', 'feature store', 'model registry', 'inference endpoint', 'experiment tracker', 'data labeling tool', 'hyperparameter tuner', 'model validator', 'drift detector', 'serving infrastructure'],
    epicPrefixes: ['Training Infrastructure', 'Feature Engineering', 'Model Serving', 'Experiment Tracking', 'Data Pipeline', 'MLOps', 'AutoML']
  },
  {
    name: 'Content Management',
    key: 'CMS',
    description: 'Content authoring, publishing workflows, and media management',
    domain: ['headless CMS', 'CDN', 'image optimization', 'SEO', 'rich text editor', 'version control', 'localization'],
    nouns: ['content model', 'publishing workflow', 'media library', 'template engine', 'SEO metadata', 'content preview', 'localization bundle', 'editorial calendar', 'taxonomy', 'content migration'],
    epicPrefixes: ['Authoring Experience', 'Publishing', 'Media Management', 'Localization', 'SEO', 'Content API', 'Personalization']
  },
  {
    name: 'Customer Success',
    key: 'CSUC',
    description: 'Customer health scoring, onboarding, and retention management',
    domain: ['NPS', 'CSAT', 'health score', 'churn prediction', 'Intercom', 'Zendesk', 'customer journey'],
    nouns: ['health score model', 'onboarding checklist', 'renewal tracker', 'expansion opportunity', 'churn risk alert', 'QBR template', 'success plan', 'adoption metric', 'feedback loop', 'escalation workflow'],
    epicPrefixes: ['Health Scoring', 'Onboarding', 'Renewal Management', 'Adoption Tracking', 'Feedback', 'Playbooks', 'Reporting']
  },
  {
    name: 'Legal Tech',
    key: 'LEGAL',
    description: 'Contract management, compliance tracking, and legal operations',
    domain: ['e-signature', 'CLM', 'GDPR', 'SOC 2', 'contract AI', 'document assembly', 'matter management'],
    nouns: ['contract template', 'approval workflow', 'clause library', 'compliance tracker', 'legal hold', 'matter docket', 'signature request', 'NDA generator', 'policy repository', 'audit checklist'],
    epicPrefixes: ['Contract Lifecycle', 'Compliance', 'Document Management', 'E-Signature', 'Legal Analytics', 'Policy Management', 'Workflow Automation']
  },
  {
    name: 'Design System',
    key: 'DESIG',
    description: 'Component library, design tokens, and UI/UX tooling',
    domain: ['Figma', 'Storybook', 'design tokens', 'accessibility', 'CSS-in-JS', 'component API', 'visual regression'],
    nouns: ['component library', 'design token', 'icon set', 'color palette', 'typography scale', 'spacing system', 'animation primitive', 'form component', 'layout grid', 'theme provider'],
    epicPrefixes: ['Core Components', 'Tokens & Theming', 'Accessibility', 'Documentation', 'Visual Testing', 'Migration', 'Design Tooling']
  },
  {
    name: 'QA & Testing',
    key: 'QAENG',
    description: 'Test automation frameworks, quality processes, and test infrastructure',
    domain: ['Playwright', 'Cypress', 'Selenium', 'k6', 'JMeter', 'Allure', 'TestRail', 'BrowserStack'],
    nouns: ['test framework', 'page object model', 'test data factory', 'CI integration', 'visual regression suite', 'API test harness', 'load test scenario', 'test report', 'flaky test detector', 'test environment'],
    epicPrefixes: ['E2E Automation', 'Performance Testing', 'API Testing', 'Visual Regression', 'Test Infrastructure', 'Test Data', 'Quality Metrics']
  },
  {
    name: 'R&D Innovation',
    key: 'RNDIV',
    description: 'Research initiatives, prototyping, and technology evaluation',
    domain: ['proof of concept', 'spike', 'tech radar', 'innovation lab', 'hackathon', 'prototype', 'feasibility study'],
    nouns: ['research spike', 'prototype', 'feasibility analysis', 'tech evaluation', 'benchmark study', 'architecture RFC', 'innovation proposal', 'demo application', 'competitive analysis', 'patent filing'],
    epicPrefixes: ['Research', 'Prototyping', 'Technology Evaluation', 'Architecture', 'Innovation', 'Competitive Analysis', 'Proof of Concept']
  },
  {
    name: 'Facilities & Operations',
    key: 'FACIL',
    description: 'Office management, physical infrastructure, and operational tooling',
    domain: ['access control', 'HVAC', 'space planning', 'visitor management', 'asset tracking', 'maintenance scheduling'],
    nouns: ['access badge system', 'meeting room booking', 'maintenance ticket', 'space allocation', 'visitor check-in', 'parking system', 'cleaning schedule', 'emergency procedure', 'equipment inventory', 'floor plan'],
    epicPrefixes: ['Access Management', 'Space Planning', 'Maintenance', 'Visitor Management', 'Safety & Compliance', 'Equipment Tracking', 'Sustainability']
  },
  {
    name: 'Internal Tooling',
    key: 'INTRL',
    description: 'Developer tools, admin dashboards, and internal productivity systems',
    domain: ['admin panel', 'CLI tool', 'internal API', 'feature flags', 'config management', 'developer portal'],
    nouns: ['admin dashboard', 'CLI command', 'feature flag', 'config service', 'developer portal', 'internal API', 'migration tool', 'health check', 'audit viewer', 'batch processor'],
    epicPrefixes: ['Admin Tools', 'Developer Experience', 'Configuration', 'Feature Flags', 'Monitoring', 'Automation', 'Documentation']
  },
  {
    name: 'Partner Integrations',
    key: 'PARTN',
    description: 'Third-party integrations, API partnerships, and ecosystem connectors',
    domain: ['OAuth', 'webhooks', 'REST API', 'GraphQL', 'SDK', 'rate limiting', 'API gateway', 'partner portal'],
    nouns: ['API connector', 'webhook handler', 'OAuth flow', 'rate limiter', 'partner portal', 'SDK module', 'API documentation', 'integration test suite', 'sandbox environment', 'data sync job'],
    epicPrefixes: ['API Gateway', 'Partner Portal', 'Webhooks', 'SDK Development', 'Data Sync', 'Authentication', 'Marketplace']
  }
];

// ─── Scale Users ─────────────────────────────────────────────────────────────

export const scaleUsers = [
  { email: 'oliver.chen@demo.com', username: 'oliverch', password: 'oliverch', first_name: 'Oliver', last_name: 'Chen', role: 'Principal Engineer', timezone: 'America/Los_Angeles', language: 'en' },
  { email: 'maya.patel@demo.com', username: 'mayap', password: 'mayap', first_name: 'Maya', last_name: 'Patel', role: 'Staff Engineer', timezone: 'Asia/Kolkata', language: 'en' },
  { email: 'lucas.silva@demo.com', username: 'lucass', password: 'lucass', first_name: 'Lucas', last_name: 'Silva', role: 'Senior Developer', timezone: 'America/Sao_Paulo', language: 'en' },
  { email: 'emma.mueller@demo.com', username: 'emmam', password: 'emmam', first_name: 'Emma', last_name: 'Mueller', role: 'Tech Lead', timezone: 'Europe/Berlin', language: 'de' },
  { email: 'yuki.tanaka@demo.com', username: 'yukit', password: 'yukit', first_name: 'Yuki', last_name: 'Tanaka', role: 'Backend Developer', timezone: 'Asia/Tokyo', language: 'en' },
  { email: 'sofia.russo@demo.com', username: 'sofiar', password: 'sofiar', first_name: 'Sofia', last_name: 'Russo', role: 'Frontend Developer', timezone: 'Europe/Rome', language: 'en' },
  { email: 'marcus.johnson@demo.com', username: 'marcusj', password: 'marcusj', first_name: 'Marcus', last_name: 'Johnson', role: 'DevOps Engineer', timezone: 'America/Chicago', language: 'en' },
  { email: 'anya.kowalski@demo.com', username: 'anyak', password: 'anyak', first_name: 'Anya', last_name: 'Kowalski', role: 'QA Lead', timezone: 'Europe/Warsaw', language: 'en' },
  { email: 'raj.gupta@demo.com', username: 'rajg', password: 'rajg', first_name: 'Raj', last_name: 'Gupta', role: 'Data Engineer', timezone: 'Asia/Kolkata', language: 'en' },
  { email: 'chloe.martin@demo.com', username: 'chloem', password: 'chloem', first_name: 'Chloe', last_name: 'Martin', role: 'Product Manager', timezone: 'Europe/Paris', language: 'en' },
  { email: 'hassan.ali@demo.com', username: 'hassana', password: 'hassana', first_name: 'Hassan', last_name: 'Ali', role: 'Security Engineer', timezone: 'Asia/Dubai', language: 'en' },
  { email: 'nina.berg@demo.com', username: 'ninab', password: 'ninab', first_name: 'Nina', last_name: 'Berg', role: 'UX Designer', timezone: 'Europe/Stockholm', language: 'en' },
  { email: 'daniel.wright@demo.com', username: 'danielw', password: 'danielw', first_name: 'Daniel', last_name: 'Wright', role: 'SRE', timezone: 'America/New_York', language: 'en' },
];

// All usernames (existing + scale) for comment assignment
const allUsernames = [
  'john', 'jane', 'mike', 'sarah', 'alex', 'emily', 'tom', 'lisa', 'david', 'maria',
  'oliverch', 'mayap', 'lucass', 'emmam', 'yukit', 'sofiar', 'marcusj', 'anyak', 'rajg', 'chloem', 'hassana', 'ninab', 'danielw'
];

// ─── Scale Time Customers ────────────────────────────────────────────────────

export const scaleTimeCustomers = [
  { name: 'NovaTech Solutions', email: 'pm@novatech.io', description: 'Enterprise SaaS platform client', active: true },
  { name: 'Pinnacle Health Group', email: 'it@pinnaclehealth.com', description: 'Healthcare network client', active: true },
  { name: 'Meridian Financial', email: 'tech@meridianfin.com', description: 'Financial services client', active: true },
  { name: 'Atlas Logistics Corp', email: 'support@atlaslogistics.com', description: 'Supply chain and logistics client', active: true },
  { name: 'Spark Education Inc', email: 'eng@sparkedu.org', description: 'EdTech platform client', active: true },
  { name: 'Quantum Retail', email: 'dev@quantumretail.com', description: 'E-commerce platform client', active: true },
];

// ─── Scale Workspaces ────────────────────────────────────────────────────────

export const scaleWorkspaces = workspaceDefinitions.map(ws => ({
  name: ws.name,
  key: ws.key,
  description: ws.description
}));

// ─── Scale Projects ──────────────────────────────────────────────────────────

const customerNames = ['NovaTech Solutions', 'Pinnacle Health Group', 'Meridian Financial', 'Atlas Logistics Corp', 'Spark Education Inc', 'Quantum Retail'];

export const scaleProjects = workspaceDefinitions.map((ws, i) => ({
  workspaceKey: ws.key,
  customerName: customerNames[i % customerNames.length],
  name: `${ws.name} Platform`,
  description: `Main project for ${ws.name.toLowerCase()} initiatives`,
  active: true
}));

// ─── Scale Milestones ────────────────────────────────────────────────────────

export const scaleMilestones = [
  // Global milestones
  { name: 'Scale Q1 Delivery', description: 'Q1 delivery targets for all scale workspaces', daysFromMonday: 60, status: 'in-progress', is_global: true, categoryName: 'Product' },
  { name: 'Scale Q2 Planning', description: 'Q2 planning milestone', daysFromMonday: 120, status: 'planning', is_global: true, categoryName: 'Business' },
  // Per-workspace local milestones
  ...workspaceDefinitions.flatMap(ws => [
    { name: `${ws.key} Alpha Release`, description: `Alpha release for ${ws.name}`, daysFromMonday: 30, status: 'in-progress', is_global: false, workspaceKey: ws.key, categoryName: 'Engineering' },
    { name: `${ws.key} Beta Release`, description: `Beta release for ${ws.name}`, daysFromMonday: 75, status: 'planning', is_global: false, workspaceKey: ws.key, categoryName: 'Product' },
  ])
];

// ─── Scale Iterations ────────────────────────────────────────────────────────

export const scaleIterations = [
  // Global iterations
  { name: 'Scale Sprint 1', description: 'First scale sprint', daysFromMonday: 0, durationDays: 14, status: 'active', type: 'Sprint', is_global: true },
  { name: 'Scale Sprint 2', description: 'Second scale sprint', daysFromMonday: 14, durationDays: 14, status: 'planned', type: 'Sprint', is_global: true },
  { name: 'Scale Q1', description: 'Q1 planning iteration', daysFromMonday: 0, durationDays: 90, status: 'active', type: 'PI / Quarter', is_global: true },
  // Per-workspace local iterations
  ...workspaceDefinitions.flatMap(ws => [
    { name: `${ws.key} Sprint 1`, description: `Sprint 1 for ${ws.name}`, daysFromMonday: 0, durationDays: 14, status: 'active', type: 'Sprint', is_global: false, workspaceKey: ws.key },
    { name: `${ws.key} Sprint 2`, description: `Sprint 2 for ${ws.name}`, daysFromMonday: 14, durationDays: 14, status: 'planned', type: 'Sprint', is_global: false, workspaceKey: ws.key },
    { name: `${ws.key} Release`, description: `Release iteration for ${ws.name}`, daysFromMonday: 0, durationDays: 60, status: 'planned', type: 'Release', is_global: false, workspaceKey: ws.key },
  ])
];

// ─── Item Generator ──────────────────────────────────────────────────────────

function generateDescription(rng, ws, title) {
  const bg = pick(rng, descriptionSections.background);
  const tech = pick(rng, descriptionSections.technicalApproach);
  const ac = pick(rng, descriptionSections.acceptanceCriteria);
  const domainTerms = pickN(rng, ws.domain, 2, 4).join(', ');
  const adj = pick(rng, technicalAdjectives);

  return `## Background\n\n${bg}\n\nThis work involves building a ${adj} solution leveraging ${domainTerms}.\n\n## Technical Approach\n\n${tech}\n\n## Acceptance Criteria\n\n${ac}`;
}

function generateStoryDescription(rng, ws) {
  const noun = pick(rng, ws.nouns);
  const adj = pick(rng, technicalAdjectives);
  const domainTerms = pickN(rng, ws.domain, 2, 3).join(' and ');
  const noun2 = pick(rng, ws.nouns);

  const contextVariants = [
    `As a developer working on the ${ws.name.toLowerCase()} platform, I need to ${pick(rng, ['implement', 'enhance', 'extend', 'refactor'])} the ${noun} so that the system can handle ${adj} workloads reliably.`,
    `The ${noun} currently lacks support for ${adj} operations. Users have reported that integration with ${domainTerms} is unreliable, and this story addresses those gaps.`,
    `Product requirements call for a ${adj} ${noun} that integrates with ${domainTerms}. This will unblock downstream work on the ${noun2} and improve overall platform stability.`,
    `Following the recent architecture review, the team identified that the ${noun} needs to be redesigned to support ${adj} patterns when working with ${domainTerms}.`,
  ];

  const detailVariants = [
    `The implementation should leverage ${domainTerms} to ensure the ${noun} meets our ${adj} requirements. Pay special attention to error boundaries and graceful degradation when upstream services are unavailable. Coordinate with the team working on the ${noun2} to ensure interface compatibility.`,
    `This involves updating the existing ${noun} abstraction layer and adding integration points for ${domainTerms}. The ${adj} nature of the solution requires careful handling of concurrency and state management. Existing consumers of the ${noun} API should not require changes.`,
    `The scope includes modifying the ${noun} service layer, adding ${domainTerms} integration hooks, and updating the data model to support ${adj} operations. The ${noun2} depends on this work, so API contracts should be finalized early.`,
    `Work should be split into two phases: first, refactor the ${noun} internals to support ${adj} patterns; second, wire up ${domainTerms} as the backing infrastructure. Include feature flags so the rollout can be controlled independently of deployment.`,
  ];

  const acItems = pickN(rng, [
    `${noun} handles concurrent requests without data corruption`,
    `Integration with ${pick(rng, ws.domain)} is covered by automated tests`,
    `Error responses include structured messages with correlation IDs`,
    `P95 latency remains under 150ms for all affected endpoints`,
    `Existing API consumers are not broken (backward compatible)`,
    `Logging captures all state transitions for debugging`,
    `Feature flag allows disabling the new behavior in production`,
    `Documentation is updated with usage examples and migration notes`,
    `Database migrations are reversible and tested against production schema`,
    `Monitoring dashboard is updated to include new metrics`,
  ], 3, 5);

  const context = pick(rng, contextVariants);
  const details = pick(rng, detailVariants);
  const acList = acItems.map(item => `- [ ] ${item}`).join('\n');

  return `## Context\n\n${context}\n\n## Implementation Notes\n\n${details}\n\n## Acceptance Criteria\n\n${acList}`;
}

function generateShortDescription(rng, ws) {
  const noun = pick(rng, ws.nouns);
  const adj = pick(rng, technicalAdjectives);
  const domainTerm = pick(rng, ws.domain);
  return `${pick(rng, ['Implement', 'Update', 'Configure', 'Add', 'Extend'])} ${adj} ${noun} using ${domainTerm}. Ensure all tests pass and documentation is updated.`;
}

function generateSubtaskDescription(rng, ws) {
  const noun = pick(rng, ws.nouns);
  const adj = pick(rng, technicalAdjectives);
  const domainTerm = pick(rng, ws.domain);
  const taskScope = pick(rng, [
    `Update the ${noun} component to support ${adj} operations with ${domainTerm}.`,
    `Refactor the ${noun} internals to align with the ${adj} architecture pattern.`,
    `Add ${domainTerm} integration to the ${noun} module.`,
    `Harden the ${noun} against edge cases identified during code review.`,
  ]);
  const steps = pick(rng, [
    `Write unit tests covering edge cases and verify integration with ${domainTerm}. Update inline documentation.`,
    `Implement error handling and structured logging. Validate behavior under ${adj} load conditions.`,
    `Add input validation, sanitization, and retry logic for transient ${domainTerm} failures.`,
    `Set up integration tests with mocked dependencies and update the API documentation with examples.`,
    `Review database migration scripts and ensure rollback safety. Add monitoring for key metrics.`,
  ]);
  return `${taskScope} ${steps}`;
}

function generateBugDescription(rng, ws) {
  const noun = pick(rng, ws.nouns);
  const domainTerm = pick(rng, ws.domain);
  const adj = pick(rng, technicalAdjectives);

  const stepsVariants = [
    [`Navigate to the ${noun} management page`, `Trigger a ${adj} operation involving ${domainTerm}`, `Observe the response or UI state`],
    [`Create or open an existing ${noun}`, `Perform an update that involves ${domainTerm} integration`, `Check the result in the API response or database`],
    [`Configure the ${noun} with ${domainTerm} settings`, `Submit the form or trigger the API call`, `Monitor the logs for unexpected behavior`],
  ];
  const steps = pick(rng, stepsVariants);
  const stepsList = steps.map((s, i) => `${i + 1}. ${s}`).join('\n');

  const expectedVariants = [
    `The ${noun} should complete the operation successfully and return a valid response with correct data.`,
    `The system should process the ${adj} request and update the ${noun} state consistently.`,
    `The ${noun} should handle the ${domainTerm} interaction gracefully and reflect the changes immediately.`,
  ];

  const actualVariants = [
    `The ${noun} returns incorrect data or fails silently, causing downstream inconsistencies.`,
    `The operation times out or throws an unhandled exception under ${adj} conditions.`,
    `The ${noun} produces duplicate entries or ignores validation rules when ${domainTerm} is involved.`,
  ];

  const envVariants = [
    `Observed in staging environment with ${domainTerm} ${pick(rng, ['v2.1', 'v3.0', 'latest'])} configured.`,
    `Reproducible on both local development and CI environments using ${domainTerm}.`,
    `Occurs intermittently under ${adj} load; consistent on staging with ${domainTerm} enabled.`,
  ];

  return `## Steps to Reproduce\n\n${stepsList}\n\n## Expected Behavior\n\n${pick(rng, expectedVariants)}\n\n## Actual Behavior\n\n${pick(rng, actualVariants)}\n\n## Environment\n\n${pick(rng, envVariants)}`;
}

function generateEpicTitle(rng, ws, index) {
  const prefix = ws.epicPrefixes[index % ws.epicPrefixes.length];
  const noun = pick(rng, ws.nouns);
  const verb = pick(rng, actionVerbs);
  return `${prefix}: ${verb} ${noun}`;
}

function generateStoryTitle(rng, ws) {
  const verb = pick(rng, actionVerbs);
  const noun = pick(rng, ws.nouns);
  const adj = pick(rng, technicalAdjectives);
  return `${verb} ${adj} ${noun}`;
}

function generateSubtaskTitle(rng, ws) {
  const actions = ['Write tests for', 'Document', 'Review', 'Configure', 'Update', 'Validate', 'Set up', 'Add logging to'];
  return `${pick(rng, actions)} ${pick(rng, ws.nouns)}`;
}

function generateBugTitle(rng, ws) {
  const verb = pick(rng, bugVerbs);
  const noun = pick(rng, ws.nouns);
  const issues = ['returns incorrect data', 'fails under high load', 'throws unhandled exception', 'has memory leak', 'times out intermittently', 'produces duplicate entries', 'ignores validation rules', 'breaks with special characters'];
  return `${verb} bug where ${noun} ${pick(rng, issues)}`;
}

const priorities = ['Critical', 'High', 'Medium', 'Low'];
const priorityWeights = [0.05, 0.2, 0.5, 0.25]; // weighted distribution

function pickPriority(rng) {
  const r = rng();
  let cumulative = 0;
  for (let i = 0; i < priorities.length; i++) {
    cumulative += priorityWeights[i];
    if (r < cumulative) return priorities[i];
  }
  return 'Medium';
}

function pickStatus(rng) {
  const r = rng();
  if (r < 0.3) return 'Open';
  if (r < 0.6) return 'In Progress';
  return 'Done';
}

export function generateWorkspaceItems(wsKey, seed) {
  const ws = workspaceDefinitions.find(w => w.key === wsKey);
  if (!ws) return [];

  const rng = createRNG(seed);
  const items = [];

  // Track used titles to avoid duplicates (causes 400 errors via itemMap key collision)
  const usedTitles = new Set();
  function uniqueTitle(baseTitle) {
    if (!usedTitles.has(baseTitle)) {
      usedTitles.add(baseTitle);
      return baseTitle;
    }
    let counter = 2;
    while (usedTitles.has(`${baseTitle} (#${counter})`)) counter++;
    const unique = `${baseTitle} (#${counter})`;
    usedTitles.add(unique);
    return unique;
  }

  // Generate 40 epics
  const epicCount = 40;
  for (let e = 0; e < epicCount; e++) {
    const epicTitle = uniqueTitle(generateEpicTitle(rng, ws, e));
    const epic = {
      title: epicTitle,
      description: generateDescription(rng, ws, epicTitle),
      status_name: pickStatus(rng),
      priority: pickPriority(rng),
      milestoneName: rng() < 0.4 ? `${wsKey} Alpha Release` : (rng() < 0.5 ? `${wsKey} Beta Release` : undefined),
      iterationName: rng() < 0.3 ? `${wsKey} Sprint 1` : (rng() < 0.3 ? `${wsKey} Sprint 2` : undefined),
      children: []
    };

    // 4-7 stories per epic
    const storyCount = rangeInt(rng, 4, 7);
    for (let s = 0; s < storyCount; s++) {
      const storyTitle = uniqueTitle(generateStoryTitle(rng, ws));
      const story = {
        title: storyTitle,
        description: generateStoryDescription(rng, ws),
        status_name: pickStatus(rng),
        priority: pickPriority(rng),
        iterationName: rng() < 0.4 ? `${wsKey} Sprint 1` : (rng() < 0.3 ? `${wsKey} Sprint 2` : undefined),
        children: []
      };

      // ~50% of stories have 2-3 subtasks
      if (rng() < 0.5) {
        const subtaskCount = rangeInt(rng, 2, 3);
        for (let t = 0; t < subtaskCount; t++) {
          story.children.push({
            title: uniqueTitle(generateSubtaskTitle(rng, ws)),
            description: generateSubtaskDescription(rng, ws),
            status_name: pickStatus(rng),
            priority: pickPriority(rng),
          });
        }
      }

      epic.children.push(story);
    }

    items.push(epic);
  }

  // 25 standalone bugs/tasks
  for (let b = 0; b < 25; b++) {
    if (rng() < 0.6) {
      // Bug
      items.push({
        title: uniqueTitle(generateBugTitle(rng, ws)),
        description: generateBugDescription(rng, ws),
        status_name: pickStatus(rng),
        priority: pickPriority(rng),
      });
    } else {
      // Standalone task (tasks can only have status Open=1 or Done=3)
      items.push({
        title: uniqueTitle(`${pick(rng, actionVerbs)} ${pick(rng, ws.nouns)}`),
        description: generateShortDescription(rng, ws),
        status_name: rng() < 0.5 ? 'Open' : 'Done',
        priority: pickPriority(rng),
        is_task: true,
      });
    }
  }

  return items;
}

// ─── Generate all scale work items ──────────────────────────────────────────

function buildScaleWorkItems() {
  const result = {};
  workspaceDefinitions.forEach((ws, index) => {
    // Use workspace index as seed offset for deterministic but unique data per workspace
    result[ws.key] = generateWorkspaceItems(ws.key, 42000 + index * 1000);
  });
  return result;
}

export const scaleWorkItems = buildScaleWorkItems();

// ─── Comment Generator ──────────────────────────────────────────────────────

/**
 * Generates comment data for ~35% of items.
 * Returns an array of { itemKey, comments: [{ content, username, is_private }] }
 * where itemKey is "WORKSPACE:title".
 *
 * @param {Object} itemMap - Map of "WORKSPACE:title" -> itemId (from createWorkItems)
 * @param {Object} userMap - Map of username -> userId (from createUsers)
 * @param {function} rng - Seeded PRNG function
 * @returns {Array<{itemKey: string, comments: Array<{content: string, username: string, is_private: boolean}>}>}
 */
export function generateCommentsForItems(itemMap, userMap, rng) {
  const commentData = [];
  const categories = Object.keys(commentTemplates);

  for (const itemKey of Object.keys(itemMap)) {
    // ~35% of items get comments
    if (rng() > 0.35) continue;

    const commentCount = rangeInt(rng, 1, 4);
    const comments = [];

    for (let c = 0; c < commentCount; c++) {
      const category = pick(rng, categories);
      const template = pick(rng, commentTemplates[category]);
      const username = pick(rng, allUsernames);

      comments.push({
        content: template,
        username,
        is_private: rng() < 0.1,  // 10% are private
      });
    }

    commentData.push({ itemKey, comments });
  }

  return commentData;
}
