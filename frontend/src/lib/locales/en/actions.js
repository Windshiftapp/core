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
      respondToCascades: 'Respond to action-triggered changes',
      respondToCascadesHint: 'When enabled, this action will also run when triggered by other actions, not just user changes.'
    },

    // Node types
    nodes: {
      trigger: 'Trigger',
      setField: 'Set Field',
      setStatus: 'Set Status',
      addComment: 'Add Comment',
      notifyUser: 'Notify User',
      condition: 'Condition'
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
      anyStatus: 'Any Status'
    },

    // Recipients
    recipients: {
      assignee: 'Assignee',
      creator: 'Creator',
      specific: 'Specific Users'
    },

    // Condition
    condition: {
      true: 'Yes',
      false: 'No'
    },

    // Operators
    operators: {
      equals: 'Equals',
      notEquals: 'Not Equals',
      contains: 'Contains',
      greaterThan: 'Greater Than',
      lessThan: 'Less Than',
      isEmpty: 'Is Empty',
      isNotEmpty: 'Is Not Empty'
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
      viewDetails: 'View Details'
    }
  }
};
