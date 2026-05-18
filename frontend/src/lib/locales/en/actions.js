/**
 * Actions automation translations (English)
 */
export default {
  actions: {
    title: 'Actions',
    description: 'Automate workflows with rule-based actions',
    create: 'Create Action',
    createFirst: 'Create Your First Action',
    noActions: 'No actions yet',
    noActionsDescription: 'Create actions to automate your workflows based on item events',
    enabled: 'Enabled',
    disabled: 'Disabled',
    enable: 'Enable',
    disable: 'Disable',
    viewLogs: 'View Logs',
    confirmDelete: 'Are you sure you want to delete the action "{name}"?',
    failedToSave: 'Failed to save action',
    newAction: 'New Action',

    // Action templates (shipped automation blueprints)
    templates: {
      pickTitle: 'Choose an action template',
      fromTemplate: 'From template',
      empty: 'No templates available.',
      help: 'Apply a shipped automation blueprint to this workspace. Each apply creates a new action you can edit afterwards.',
      apply: 'Apply',
    },

    // Trigger types
    trigger: {
      statusTransition: 'Status Transition',
      itemCreated: 'Item Created',
      itemUpdated: 'Item Updated',
      itemLinked: 'Item Linked',
      manual: 'Manual',
      respondToCascades: 'Respond to action-triggered changes',
      respondToCascadesHint:
        'When enabled, this action will also run when triggered by other actions, not just user changes.',
    },

    // Node types
    nodes: {
      trigger: 'Trigger',
      setField: 'Set Field',
      setStatus: 'Set Status',
      addComment: 'Add Comment',
      notifyUser: 'Notify User',
      condition: 'Condition',
      updateAsset: 'Update Asset',
      createAsset: 'Create Asset',
      httpRequest: 'HTTP Request',
      containerRun: 'Run Container',
      aiExtract: 'AI Extract',
      aiAgent: 'AI Agent',
      relatedItems: 'For each related item',
      transitionItem: 'Transition item',
    },

    // Toast shown when the AI chat updates the open action via update_action.
    aiUpdated: 'Action updated by AI',

    // Actor override (run-as)
    runAs: 'Run as',
    runAsTriggerUser: 'Run as triggering user',
    runAsHint: 'The action executes with this user\u2019s permissions. Leave blank to run as whoever triggered it.',
    runAsReadonlyHint: 'Requires the Set Action Actor permission to change.',

    // Node palette and tips
    addNodes: 'Add Nodes',
    tips: 'Tips',
    tipDragToConnect: 'Drag from handles to connect nodes',
    tipClickToEdit: 'Click a node to configure it',
    tipConditionBranches: 'Conditions have true/false branches',

    // Config panel
    nodeConfig: 'Node Configuration',
    config: {
      from: 'From',
      to: 'To',
      selectField: 'Select field...',
      selectStatus: 'Select status...',
      enterComment: 'Enter comment...',
      selectRecipient: 'Select recipient...',
      setCondition: 'Set condition...',
      targetStatus: 'Target Status',
      fieldName: 'Field Name',
      value: 'Value',
      commentContent: 'Comment Content',
      commentPlaceholder: 'Enter comment text. Use {{item.title}} for variables.',
      privateComment: 'Private comment (internal only)',
      fieldToCheck: 'Field to Check',
      operator: 'Operator',
      compareValue: 'Compare Value',
      private: 'Private',
      triggerType: 'Trigger Type',
      fromStatus: 'From Status',
      toStatus: 'To Status',
      anyStatus: 'Any Status',
      triggerField: 'Changed Field',
      anyField: 'Any field (all changes)',
      recipientType: 'Recipient',
      notifyMessage: 'Message',
      notifyPlaceholder: 'Enter message. Use {{item.title}} for variables.',
      includeLink: 'Include link to item',
      // Update Asset config
      sourceAssetField: 'Asset Field on Item',
      selectAssetField: 'Select asset field...',
      sourceAssetFieldHint: 'Select the item field that contains the linked asset',
      targetAssetType: 'Target Asset Type',
      selectAssetType: 'Select asset type...',
      fieldMappingsLabel: 'Field Mappings',
      fieldMappings: '{count} field mapping(s)',
      configureAssetUpdate: 'Configure asset update...',
      fromField: 'From field',
      sourceTypeVariable: 'Variable/Template',
      sourceTypeItemField: 'Item Field',
      sourceTypeLiteral: 'Literal Value',
      selectTargetField: 'Select target field...',
      addMapping: 'Add Mapping',
      // Create Asset config
      assetSet: 'Asset Set',
      selectAssetSet: 'Select asset set...',
      assetTitle: 'Asset Title',
      assetTitleHint: 'Use {{item.title}} or other variables',
      assetDescription: 'Description',
      assetTagLabel: 'Asset Tag',
      assetCategory: 'Category',
      selectCategory: 'Select category (optional)...',
      assetStatus: 'Status',
      selectStatusOptional: 'Select status (optional)...',
      requiredField: 'Required',
      configureAssetCreation: 'Configure asset creation...',
      // Capability picker (HTTP, Docker, LLM nodes)
      capability: 'Capability',
      selectCapability: 'Select capability...',
      noCapabilitiesForWorkspace: 'No capabilities available in this workspace. Ask an admin to provision one.',
      configureRequest: 'Configure HTTP request...',
      configureExtract: 'Configure AI extract...',
      selectModelAndTools: 'Select model and tools...',
      // HTTP request node
      httpCapability: 'HTTP Client Capability',
      httpMethod: 'Method',
      urlTemplate: 'URL Template',
      requestBody: 'Request Body',
      requestBodyPlaceholder: 'Optional. JSON body, may use {{variables}}.',
      // Container run node
      dockerCapability: 'Docker Environment',
      timeoutSecs: 'Timeout (seconds)',
      // AI nodes
      llmCapability: 'LLM Connection',
      model: 'Model',
      tools: 'tools',
      aiPrompt: 'Prompt',
      aiExtractPromptPlaceholder: 'Extract structured data from the input. Be specific about what to extract.',
      systemPrompt: 'System Prompt',
      systemPromptPlaceholder: 'You are a helpful assistant. Use the tools to ...',
      inputField: 'Input Field',
      inputFieldPlaceholder: 'name of variable to read input from',
      inputFields: 'Input Fields',
      inputFieldsPlaceholder: 'comma-separated variable names',
      outputField: 'Output Field',
      outputFieldPlaceholder: 'name of variable to write output to',
      outputSchema: 'Output JSON Schema',
      agentTools: 'Tools',
      agentToolsHint: 'HTTP-client capabilities the agent may call. Only capabilities scoped to this workspace are listed.',
      noToolsAvailable: 'No HTTP-client capabilities available for this workspace.',
      maxSteps: 'Max Iterations',
    },

    // Recipients
    recipients: {
      assignee: 'Assignee',
      creator: 'Creator',
      specific: 'Specific Users',
    },

    // Condition
    condition: {
      true: 'Yes',
      false: 'No',
    },

    // Operators
    operators: {
      equals: 'Equals',
      notEquals: 'Not Equals',
      contains: 'Contains',
      greaterThan: 'Greater Than',
      lessThan: 'Less Than',
      isEmpty: 'Is Empty',
      isNotEmpty: 'Is Not Empty',
    },

    // Execution logs
    logs: {
      title: 'Execution Logs',
      noLogs: 'No execution logs',
      status: 'Status',
      running: 'Running',
      completed: 'Completed',
      failed: 'Failed',
      skipped: 'Skipped',
      startedAt: 'Started At',
      completedAt: 'Completed At',
      error: 'Error',
      details: 'Details',
      viewDetails: 'View Details',
    },

    // Execution trace
    trace: {
      title: 'Execution Details',
      noSteps: 'No execution steps recorded',
      setStatus: 'Changed status from "{from}" to "{to}"',
      setField: 'Set {field} from "{from}" to "{to}"',
      addComment: 'Added {prefix}comment: "{content}"',
      notifyUser: 'Sent notification to {count} user(s)',
      notifySkipped: 'Notification skipped: {reason}',
      conditionResult: 'Condition evaluated to {result}',
      updateAsset: 'Updated asset #{asset_id}',
      updateAssetSkipped: 'Asset update skipped: {reason}',
      createAsset: 'Created asset #{asset_id}: {title}',
      createAssetFailed: 'Asset creation failed: {reason}',
    },

    // Test/manual execution
    test: {
      title: 'Test Action',
      description:
        'Select an item to run this action against. This will execute the action immediately, bypassing the normal trigger.',
      selectItem: 'Select Item',
      itemPlaceholder: 'Search for an item...',
      execute: 'Run Action',
      run: 'Test Run',
      executionFailed: 'Failed to execute action',
      executionQueued: 'Action queued for execution',
    },

    // Placeholder reference
    placeholders: {
      title: 'Available Placeholders',
      description:
        'Use these placeholders in your template. They will be replaced with actual values when the action runs.',
      showReference: 'Show placeholder reference',
      categories: {
        item: 'Item Fields',
        user: 'Current User',
        old: 'Previous Values',
        trigger: 'Trigger Context',
      },
      item: {
        title: 'Item title',
        id: 'Item ID',
        statusId: 'Status ID',
        assigneeId: 'Assignee user ID',
        any: 'Any item field',
      },
      user: {
        name: "User's full name",
        email: "User's email",
        id: 'User ID',
      },
      old: {
        description: 'Previous value before change',
        example: "Any field's previous value",
      },
      trigger: {
        itemId: 'Triggering item ID',
        workspaceId: 'Workspace ID',
      },
    },
    switchToVertical: 'Switch to vertical layout',
    switchToHorizontal: 'Switch to horizontal layout',
  },
};
