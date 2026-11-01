/** WI-726 work-item terminology overrides. */
export default {
  "common": {
    "noItems": "暂无工作项",
    "noItemsFound": "未找到工作项"
  },
  "errors": {
    "ITEM_NOT_FOUND": "未找到该工作项。"
  },
  "emptyStates": {
    "noItemsMatch": "没有匹配筛选条件的工作项",
    "configureFilter": "配置筛选器以查看工作项"
  },
  "users": {
  },
  "items": {
    "agentLogEmpty": "尚未为此工作项运行代理",
    "agentRerunTitle": "在此工作项上再次运行代理",
    "rollupItemCount": "汇总了 {count} 个工作项",
    "linkToPage": "将此工作项链接到同一工作区中的知识页面。"
  },
  "workspaceSettings": {
    "headers": {
      "recurrence": "管理重复工作项规则",
      "templates": "新工作项的可重用描述支架"
    }
  },
  "testing": {
    "noItemTypesAvailable": "无可用工作项类型"
  },
  "portal": {
    "itemType": "工作项类型"
  },
  "pickers": {
    "noItemsFound": "未找到工作项",
    "noItemsAvailable": "无可用工作项",
    "agentOffline": "代理离线 — 其跑步者池中没有实时跑步者；分配的工作项将排队"
  },
  "widgets": {
    "recentItems": {
      "loadingText": "正在加载最近的工作项...",
      "emptyTitle": "没有最近的工作项",
      "emptySubtitle": "将显示最近查看的工作项此处",
      "loadError": "无法加载最近的工作项"
    }
  },
  "commandPalette": {
    "commands": {
      "assets": {
        "description": "管理资产集和工作项"
      }
    },
    "recentlyViewed": {
      "label": "最近查看的工作项",
      "description": "跳转到最近查看的 20 个工作项之一",
      "empty": "没有最近查看的工作项",
      "loading": "正在加载最近的工作项..."
    }
  },
  "migration": {
    "migrationSuccess": "所有工作项已成功迁移"
  },
  "actions": {
    "nodes": {
      "transitionItem": "过渡工作项"
    }
  },
  "analytics": {
    "collectionLoadError": "无法加载集合。分析将显示工作区中的所有工作项。",
    "allItems": "所有工作区工作项",
    "items_one": "{count} 个工作项",
    "items_other": "{count} 个工作项",
    "scope": {
      "summary": "{items} 当前工作项 · {from}–{to}",
      "currentWorkspaceNote": "日期范围适用于流量和交付图表；健康状况和工龄为当前快照。历史图表使用今天位于此工作区的工作项，不包括已移动或已删除的工作项。",
      "currentCollectionNote": "日期范围适用于流量和交付图表；健康状况和工龄为当前快照。历史图表使用今天符合此集合条件的工作项。更改集合可能会改变分析队列。"
    },
    "health": {
      "attentionItems": "待检查工作项",
      "item": "工作项",
      "allClear": "当前没有未完成工作项符合关注信号。"
    },
    "throughput": {
      "description": "每周新增和首次完成数量。重新打开工作项不会改写其首次完成记录。"
    },
    "aging": {
      "description": "当前未完成工作项已经打开了多长时间。",
      "total": "活动工作项",
      "itemCount": "工作项数",
      "oldest": "最早的未完成工作项"
    },
    "deliveryTime": {
      "analyzed": "已完成工作项",
      "missingHistory_one": "由于缺少完成历史，已排除 1 个当前已完成工作项。",
      "missingHistory_other": "由于缺少完成历史，已排除 {count} 个当前已完成工作项。",
      "definition": "从工作项创建到首次转换到已完成状态。之后重新打开不会更改此值。",
      "missingHistory": "{count} 当前完成的工作项已被排除，因为缺少完成历史记录。"
    },
    "insufficientData": {
      "no_items": "此范围内尚无工作项。",
      "few_completed_items": "此范围内只完成了少量工作项，请将百分位数据视为趋势参考。"
    }
  },
  "pages": {
    "workItemsButton": "工作项",
    "workItemsAria": "显示链接的工作项",
    "workItemsEmpty": "无工作项链接还在这里"
  },
  "recurrence": {
    "emptyDesc": "当工作项配置了重复计划时，重复规则将显示在此处。",
    "createFromItem": "要创建重复规则，请打开一个工作项并从详细信息侧边栏设置重复。"
  },
  "conditionSets": {
    "scriptRefItemProps": "工作项属性",
    "errorMessagePlaceholder": "例如，只有测试人员可以将工作项移至 QA"
  },
  "approvalSets": {
    "setStatusesDesc": "当工作项进入其中一种状态时，将打开批准请求。用户无法再直接调用配置的批准/拒绝转换 - 只有批准引擎驱动它们。",
    "approverSourceCreator": "工作项的创建者",
    "allowSelfApproval": "允许自我批准 — 允许同一用户转换工作项并批准它"
  }
};

