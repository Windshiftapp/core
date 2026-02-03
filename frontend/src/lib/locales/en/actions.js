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
    },

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
  },
};
