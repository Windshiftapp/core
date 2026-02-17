/**
 * Challenge Data Definitions
 *
 * Edge-case and security testing data for the demo generator.
 * Run with: node generate-demo.js --challenge
 *
 * Categories tested:
 * - i18n names (international characters)
 * - Long strings (titles, descriptions)
 * - XSS payloads
 * - SQL injection strings
 * - Unicode edge cases (RTL, zero-width, combining chars)
 * - Path traversal strings
 * - CRLF injection
 * - Null bytes
 * - Markdown/HTML content
 * - Emoji-heavy content
 */

// Challenge users with international names and edge cases
export const challengeUsers = [
  // Latin American
  {
    email: 'jose.garcia@demo.com',
    username: 'joseg',
    password: 'joseg',
    first_name: 'José',
    last_name: 'García',
    role: 'Developer',
    timezone: 'America/Mexico_City',
    language: 'es'
  },
  {
    email: 'maria.gonzalez@demo.com',
    username: 'mariag',
    password: 'mariag',
    first_name: 'María',
    last_name: 'González Núñez',
    role: 'QA Engineer',
    timezone: 'America/Bogota',
    language: 'es'
  },
  // French
  {
    email: 'francois.lefevre@demo.com',
    username: 'francoisl',
    password: 'francoisl',
    first_name: 'François',
    last_name: 'Lefèvre',
    role: 'Developer',
    timezone: 'Europe/Paris',
    language: 'fr'
  },
  {
    email: 'amelie.cote@demo.com',
    username: 'ameliec',
    password: 'ameliec',
    first_name: 'Amélie',
    last_name: 'Côté',
    role: 'Designer',
    timezone: 'America/Toronto',
    language: 'fr'
  },
  // German
  {
    email: 'gunther.muller@demo.com',
    username: 'guntherm',
    password: 'guntherm',
    first_name: 'Günther',
    last_name: 'Müller',
    role: 'Developer',
    timezone: 'Europe/Berlin',
    language: 'de'
  },
  {
    email: 'jurgen.schafer@demo.com',
    username: 'jurgens',
    password: 'jurgens',
    first_name: 'Jürgen',
    last_name: 'Schäfer',
    role: 'Tech Lead',
    timezone: 'Europe/Berlin',
    language: 'de'
  },
  // Polish
  {
    email: 'malgorzata.s@demo.com',
    username: 'malgorzatas',
    password: 'malgorzatas',
    first_name: 'Małgorzata',
    last_name: 'Śmiałkowska',
    role: 'Developer',
    timezone: 'Europe/Warsaw',
    language: 'pl'
  },
  {
    email: 'lukasz.z@demo.com',
    username: 'lukaszz',
    password: 'lukaszz',
    first_name: 'Łukasz',
    last_name: 'Żółkiewski',
    role: 'DevOps Engineer',
    timezone: 'Europe/Warsaw',
    language: 'pl'
  },
  // Chinese
  {
    email: 'wangwei@demo.com',
    username: 'wangwei',
    password: 'wangwei',
    first_name: '伟',
    last_name: '王',
    role: 'Developer',
    timezone: 'Asia/Shanghai',
    language: 'zh'
  },
  {
    email: 'lina@demo.com',
    username: 'lina',
    password: 'lina',
    first_name: '娜',
    last_name: '李',
    role: 'Product Manager',
    timezone: 'Asia/Shanghai',
    language: 'zh'
  },
  // Japanese
  {
    email: 'tanaka@demo.com',
    username: 'tanakat',
    password: 'tanakat',
    first_name: '太郎',
    last_name: '田中',
    role: 'Developer',
    timezone: 'Asia/Tokyo',
    language: 'ja'
  },
  // Arabic
  {
    email: 'ahmed.m@demo.com',
    username: 'ahmedm',
    password: 'ahmedm',
    first_name: 'أحمد',
    last_name: 'محمد',
    role: 'Developer',
    timezone: 'Asia/Dubai',
    language: 'ar'
  },
  // Hebrew
  {
    email: 'yosi.k@demo.com',
    username: 'yosik',
    password: 'yosik',
    first_name: 'יוסי',
    last_name: 'כהן',
    role: 'Developer',
    timezone: 'Asia/Jerusalem',
    language: 'he'
  },
  // Emoji in name
  {
    email: 'rocket.john@demo.com',
    username: 'rocketjohn',
    password: 'rocketjohn',
    first_name: 'John',
    last_name: 'Rocket 🚀',
    role: 'Developer',
    timezone: 'America/New_York',
    language: 'en'
  }
];

// Challenge workspaces with edge-case names
export const challengeWorkspaces = [
  {
    name: 'Test & QA (Überprüfung)',
    key: 'TEST',
    description: 'Testing workspace with special characters: <test> & "quotes" \'apostrophe\''
  },
  {
    name: '日本語ワークスペース',
    key: 'JPWS',
    description: 'Japanese workspace for testing i18n: テスト環境'
  }
];

// Challenge projects
export const challengeProjects = [
  {
    workspaceKey: 'TEST',
    customerName: 'Acme Corporation',
    name: 'Security & Performance <Test>',
    description: 'Project testing XSS: <script>alert("test")</script> and SQL: \'; DROP TABLE users; --',
    active: true
  },
  {
    workspaceKey: 'JPWS',
    customerName: 'TechStart Inc',
    name: '品質保証プロジェクト',
    description: 'Quality assurance project with Japanese text: テスト、検証、品質管理',
    active: true
  }
];

// Challenge work items organized by workspace
export const challengeWorkItems = {
  'SOFT': [
    // XSS payloads in titles
    {
      title: '<script>alert("XSS")</script> Test Item',
      description: 'Testing XSS in title - this should be escaped and display as text',
      status_name: 'Open',
      is_task: false,
      children: [
        {
          title: '<img src=x onerror=alert("XSS")> Image XSS Test',
          description: 'Image tag XSS payload in title',
          status_name: 'Open'
        },
        {
          title: '<svg onload=alert("XSS")> SVG XSS Test',
          description: 'SVG onload XSS payload',
          status_name: 'Open'
        },
        {
          title: 'javascript:alert("XSS") Link Test',
          description: 'JavaScript protocol in title',
          status_name: 'Open'
        }
      ]
    },
    // SQL injection in titles
    {
      title: "SQL Test: ' OR '1'='1",
      description: "Testing SQL injection payload: '; DROP TABLE users; --",
      status_name: 'Open',
      is_task: false,
      children: [
        {
          title: "'; DELETE FROM items WHERE '1'='1",
          description: 'DELETE injection test',
          status_name: 'Open'
        },
        {
          title: '1; SELECT * FROM users--',
          description: 'SELECT injection test',
          status_name: 'Open'
        }
      ]
    },
    // Very long title (200+ characters)
    {
      title: 'This is an extremely long title that exceeds the typical length limits and tests how the UI handles very long text content without any word breaks or natural wrapping points which could cause layout issues in various components',
      description: 'Testing long title truncation and display',
      status_name: 'Open',
      is_task: false
    },
    // Continuous string without spaces
    {
      title: 'ThisIsAContinuousStringWithNoSpacesOrBreakPointsThatCouldCauseHorizontalOverflowInTheUIWhenDisplayed',
      description: 'Testing word-break and overflow handling',
      status_name: 'Open',
      is_task: false
    },
    // Very long description
    {
      title: 'Item with very long description',
      description: 'Lorem ipsum dolor sit amet, consectetur adipiscing elit. '.repeat(200) + 'End of description.',
      status_name: 'Open',
      is_task: false
    },
    // Unicode edge cases
    {
      title: 'Unicode Test: Mixed RTL/LTR שלום Hello مرحبا',
      description: 'Testing bidirectional text: This is English שזו עברית and this هذا عربي',
      status_name: 'Open',
      is_task: false,
      children: [
        {
          title: 'Zero-width chars: test\u200Bhidden\u200Btext',
          description: 'Contains zero-width space characters (U+200B)',
          status_name: 'Open'
        },
        {
          title: 'RTL Override: test\u202Eesrever',
          description: 'Contains RTL override character (U+202E)',
          status_name: 'Open'
        },
        {
          title: 'Combining chars: e\u0301 vs é (cafe\u0301 vs café)',
          description: 'Testing combining diacritical marks',
          status_name: 'Open'
        },
        {
          title: 'Surrogate pairs: 𝕳𝖊𝖑𝖑𝖔 (Mathematical Bold Fraktur)',
          description: 'Testing Unicode surrogate pairs and mathematical symbols',
          status_name: 'Open'
        }
      ]
    },
    // Path traversal strings
    {
      title: 'Path Test: ../../../etc/passwd',
      description: 'Testing path traversal: ..\\..\\..\\windows\\system32\\config\\sam',
      status_name: 'Open',
      is_task: false
    },
    // CRLF injection
    {
      title: 'CRLF Test\r\nX-Injected-Header: value',
      description: 'Testing CRLF injection:\r\nSet-Cookie: malicious=true',
      status_name: 'Open',
      is_task: false
    },
    // Null bytes
    {
      title: 'Null byte test\x00hidden',
      description: 'Testing null byte handling: before\x00after',
      status_name: 'Open',
      is_task: false
    },
    // Markdown/HTML in description
    {
      title: 'Markdown Test Item',
      description: '# Heading\n\n**Bold text** and *italic text*\n\n- List item 1\n- List item 2\n\n```javascript\nconst code = "test";\n```\n\n[Link text](https://example.com)\n\n![Image alt](https://example.com/image.png)',
      status_name: 'Open',
      is_task: false
    },
    // HTML in description
    {
      title: 'HTML Content Test',
      description: '<h1>HTML Heading</h1><p>Paragraph with <strong>bold</strong> and <em>italic</em></p><script>alert("XSS")</script><img src="x" onerror="alert(1)"><a href="javascript:alert(1)">Click me</a>',
      status_name: 'Open',
      is_task: false
    },
    // Emoji-heavy content
    {
      title: '🚀 Launch 🎉 Party 💻 Coding 🔥 Fire 💯 Perfect',
      description: '🎯 Goal: Ship features!\n\n📋 Tasks:\n- ✅ Complete\n- ⏳ In progress\n- ❌ Blocked\n\n👨‍💻 Team: 🧑‍🔬 Scientists 🧑‍🎨 Designers',
      status_name: 'Open',
      is_task: false
    },
    // International characters in title
    {
      title: '日本語タイトル - Japanese Title Test',
      description: 'テストの説明: This tests Japanese characters in work items',
      status_name: 'Open',
      is_task: false
    },
    {
      title: 'العنوان العربي - Arabic Title Test',
      description: 'وصف الاختبار: Testing Arabic RTL text handling',
      status_name: 'Open',
      is_task: false
    },
    {
      title: 'כותרת עברית - Hebrew Title Test',
      description: 'תיאור הבדיקה: Testing Hebrew RTL text handling',
      status_name: 'Open',
      is_task: false
    },
    // Special characters
    {
      title: 'Special chars: @#$%^&*()_+-=[]{}|;:\'",.<>?/',
      description: 'Testing all keyboard special characters: `~!@#$%^&*()_+-=[]{}\\|;:\'",.<>?/',
      status_name: 'Open',
      is_task: false
    },
    // Whitespace edge cases
    {
      title: '   Leading and trailing spaces   ',
      description: '   Description with leading/trailing whitespace   \n\n   And extra newlines   ',
      status_name: 'Open',
      is_task: false
    },
    // Tab characters
    {
      title: 'Tab\tcharacter\tin\ttitle',
      description: 'Tab\tcharacters\tin\tdescription\ttoo',
      status_name: 'Open',
      is_task: false
    },
    // Newlines in title (should probably be stripped)
    {
      title: 'Line\nbreak\nin\ntitle',
      description: 'Testing newline characters in title field',
      status_name: 'Open',
      is_task: false
    }
  ],
  'SUPP': [
    // Customer-reported edge case issues
    {
      title: 'Customer report: "App crashes" <needs review>',
      description: 'Customer said: "When I click <button> it shows error \'undefined\'"',
      status_name: 'Open',
      is_task: false,
      children: [
        {
          title: "Customer email: test@example.com' OR '1'='1",
          description: 'Testing customer email field handling',
          status_name: 'Open'
        }
      ]
    },
    // Support ticket with emoji
    {
      title: '😡 URGENT: Customer very upset about 🐛 bug',
      description: '⚠️ Priority: HIGH\n\n📞 Customer called 3 times today!\n\n💔 Losing patience with us',
      status_name: 'Open',
      is_task: false
    }
  ],
  'MKTG': [
    // Marketing with special characters
    {
      title: 'Campaign: "Summer Sale" & 50% OFF!!!',
      description: 'Marketing copy with <special> formatting & "quotes"',
      status_name: 'Open',
      is_task: false
    },
    // Long campaign name
    {
      title: 'Multi-Channel Integrated Digital Marketing Campaign for Q4 2025 Holiday Season with Social Media, Email, and PPC Components Across All Target Demographics and Geographic Regions',
      description: 'Testing very long marketing campaign names',
      status_name: 'Open',
      is_task: false
    }
  ],
  'TEST': [
    // Special characters in titles
    {
      title: 'Test Case: Special Characters <>&"\'',
      description: 'Testing special character handling in various fields',
      status_name: 'Open',
      priority: 'High',
      is_task: false
    },
    {
      title: 'Performance Benchmark Suite',
      description: 'Automated performance testing framework',
      status_name: 'Done',
      priority: 'Medium',
      is_task: false
    }
  ],
  'JPWS': [
    // Japanese workspace items
    {
      title: '品質テスト計画',
      description: 'Quality test planning with Japanese text: テスト仕様書の作成',
      status_name: 'Open',
      priority: 'High',
      is_task: false
    },
    {
      title: 'ローカライゼーション検証',
      description: 'Localization verification: 日本語UIの確認',
      status_name: 'Done',
      priority: 'Medium',
      is_task: false
    }
  ]
};

// Challenge test labels
export const challengeTestLabels = [
  {
    name: '<script>alert("XSS")</script>',
    color: '#ff0000',
    description: 'XSS test label'
  },
  {
    name: "'; DROP TABLE labels; --",
    color: '#00ff00',
    description: 'SQL injection test label'
  },
  {
    name: '日本語ラベル',
    color: '#0000ff',
    description: 'Japanese label'
  },
  {
    name: '🔥 Critical 🚨',
    color: '#ff6600',
    description: 'Emoji label'
  }
];

// Challenge test folders
export const challengeTestFolders = [
  {
    name: '<Security Tests>',
    description: 'Folder with angle brackets in name',
    children: [
      {
        name: "SQL' OR '1'='1",
        description: 'SQL injection in folder name'
      },
      {
        name: '../../../etc',
        description: 'Path traversal in folder name'
      }
    ]
  },
  {
    name: '国際化テスト',
    description: 'Japanese folder name: Internationalization tests',
    children: [
      {
        name: '子フォルダ',
        description: 'Japanese child folder'
      }
    ]
  }
];

// Challenge test cases
export const challengeTestCases = [
  {
    folderPath: '<Security Tests>',
    title: '<script>alert("XSS in test case")</script>',
    preconditions: 'Preconditions with <script>alert("XSS")</script>',
    steps: [
      {
        action: 'Enter <script>alert(1)</script> in field',
        data: '<img onerror=alert(1) src=x>',
        expected: 'Input should be sanitized'
      },
      {
        action: "Enter ' OR '1'='1 in search",
        data: "'; DELETE FROM users; --",
        expected: 'SQL should be escaped'
      }
    ],
    labels: ['<script>alert("XSS")</script>']
  },
  {
    folderPath: '国際化テスト',
    title: '日本語テストケース',
    preconditions: 'テストの前提条件',
    steps: [
      {
        action: 'テストアクション',
        data: 'テストデータ',
        expected: '期待される結果'
      }
    ],
    labels: ['日本語ラベル']
  },
  {
    folderPath: '<Security Tests>',
    title: 'Test with very long title that exceeds normal display limits and should be truncated properly by the UI to prevent layout breaking issues in list views and detail panels',
    preconditions: 'Lorem ipsum '.repeat(100),
    steps: [
      {
        action: 'Action '.repeat(50),
        data: 'Data '.repeat(50),
        expected: 'Expected '.repeat(50)
      }
    ],
    labels: []
  }
];

// Challenge assets
export const challengeAssets = {
  'IT Equipment': [
    {
      type: 'Laptop',
      category: 'Computers/Laptops',
      title: '<script>alert("XSS")</script> MacBook',
      description: 'Asset with XSS in title',
      asset_tag: 'XSS-001',
      ownerUsername: 'joseg'
    },
    {
      type: 'Laptop',
      category: 'Computers/Laptops',
      title: "Asset'; DROP TABLE assets; --",
      description: 'Asset with SQL injection in title',
      asset_tag: 'SQL-001',
      ownerUsername: 'tanakat'
    },
    {
      type: 'Laptop',
      category: 'Computers/Laptops',
      title: '日本語資産名 - Japanese Asset',
      description: '資産の説明: Asset with Japanese name',
      asset_tag: 'JP-001',
      ownerUsername: 'wangwei'
    },
    {
      type: 'Laptop',
      category: 'Computers/Laptops',
      title: '🖥️ Developer Machine 💻 with 🚀 Performance',
      description: 'Asset with emojis in title',
      asset_tag: 'EMOJI-001',
      ownerUsername: 'rocketjohn'
    },
    {
      type: 'Monitor',
      category: 'Computers/Monitors',
      title: 'Monitor with RTL text: شاشة العرض',
      description: 'Testing Arabic RTL in asset name',
      asset_tag: 'RTL-001'
    },
    {
      type: 'Monitor',
      category: 'Computers/Monitors',
      title: 'This is an extremely long asset title that exceeds typical display limits and should test the truncation and overflow handling in various UI components including tables lists and detail views',
      description: 'Testing long asset names',
      asset_tag: 'LONG-001'
    }
  ]
};

// Challenge milestones
export const challengeMilestones = [
  {
    name: '<script>alert("XSS")</script> Milestone',
    description: 'Milestone with XSS in name',
    daysFromMonday: 30,
    status: 'planning',
    is_global: true
  },
  {
    name: '日本語マイルストーン',
    description: 'Japanese milestone: マイルストーンの説明',
    daysFromMonday: 45,
    status: 'planning',
    workspaceKey: 'SOFT',
    is_global: false
  },
  {
    name: "Milestone'; DROP TABLE milestones; --",
    description: 'SQL injection test milestone',
    daysFromMonday: 60,
    status: 'planning',
    is_global: true
  }
];

// Challenge iterations
export const challengeIterations = [
  {
    name: '<script>alert("XSS")</script> Sprint',
    description: 'Sprint with XSS in name',
    daysFromMonday: 0,
    durationDays: 14,
    status: 'active',
    type: 'Sprint',
    is_global: false,
    workspaceKey: 'SOFT'
  },
  {
    name: '日本語スプリント',
    description: 'Japanese sprint: スプリントの説明',
    daysFromMonday: 14,
    durationDays: 14,
    status: 'planning',
    type: 'Sprint',
    is_global: false,
    workspaceKey: 'SOFT'
  }
];

// Challenge personal tasks
export const challengePersonalTasks = [
  {
    title: '<script>alert("XSS")</script> Personal Task',
    description: 'Personal task with XSS payload',
    dueDaysFromMonday: 2,
    daysFromMonday: 0,
    scheduledTime: '10:00',
    durationMinutes: 60
  },
  {
    title: '個人タスク - Japanese Personal Task',
    description: '日本語の説明: Testing i18n in personal tasks',
    dueDaysFromMonday: 3,
    daysFromMonday: 1,
    scheduledTime: '14:00',
    durationMinutes: 30
  },
  {
    title: "Task'; DROP TABLE tasks; --",
    description: 'SQL injection in personal task',
    dueDaysFromMonday: 4
  },
  {
    title: '🎯 Goal Task 🚀 with 💪 Motivation',
    description: 'Task with lots of emojis: 📅 Due soon! ⚡ Priority! 🔥 Important!',
    dueDaysFromMonday: 1,
    daysFromMonday: 0,
    scheduledTime: '09:00',
    durationMinutes: 45
  },
  {
    title: 'Task with RTL: مهمة عربية وעברית',
    description: 'Mixed RTL/LTR: English عربي עברית English',
    dueDaysFromMonday: 5
  }
];

// Challenge time customers
export const challengeTimeCustomers = [
  {
    name: '<script>alert("XSS")</script> Corp',
    email: 'xss@example.com',
    description: 'Customer with XSS in name',
    active: true
  },
  {
    name: '日本企業株式会社',
    email: 'nihon@example.jp',
    description: '日本の顧客: Japanese customer',
    active: true
  },
  {
    name: "Customer'; DROP TABLE customers; --",
    email: 'sql@example.com',
    description: 'SQL injection test customer',
    active: true
  }
];
