export default {
  dashboard: {
    salutation: {
      withName: '{salutation}，{name}！',
      withoutName: '{salutation}！',
    },
    sections: {
      yourDay: {
        title: '你的一天',
        subtitle: '快速了解需要你关注的事项',
      },
      work: {
        title: '工作',
        subtitle: '你的个人列表和分配给你的事项',
      },
      workspaces: {
        title: '工作区',
        subtitle: '继续之前的工作',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: '每日简报',
        description: '由 AI 生成的今日重点摘要',
      },
      yourActivity: {
        name: '你的活动',
        description: '你最近查看、编辑或评论的事项',
      },
      whatsNew: {
        name: '最新动态',
        description: '最新通知和未读更新',
      },
      personalTasks: {
        name: '个人任务',
        description: '个人待办列表中的事项',
      },
      savedSearch: {
        name: '已保存的搜索',
        description: '显示已保存集合中的工作事项',
      },
      assignedToMe: {
        name: '分配给我的',
        description: '所有工作区中分配给你的未完成事项',
      },
      watchedItems: {
        name: '关注的事项',
        description: '你正在关注的事项',
      },
      upcomingMilestones: {
        name: '即将到来的里程碑',
        description: '目标日期临近的里程碑',
      },
      recentWorkspaces: {
        name: '最近的工作区',
        description: '你最近访问过的工作区',
      },
      quickAccess: {
        name: '快速访问',
        description: '快速打开你有权访问的工作区',
      },
    },
    customization: {
      widgets: '小部件',
      activity: {
        name: '活动',
        description: '简报、活动记录和通知',
      },
      work: {
        name: '工作',
        description: '事项、里程碑和分配给你的任务',
      },
      navigation: {
        name: '导航',
        description: '快速访问工作区',
      },
      tipLabel: '提示',
      tip: '将小部件从这里拖到面板中的任意分区。',
    },
    editor: {
      newSection: '新建分区',
      deleteSectionConfirm: '要删除此分区吗？其中的所有小部件都将被移除。',
      doneEditing: '完成编辑',
      customize: '自定义',
      editModeDescription: '编辑模式：添加、重命名、重新排序或删除分区和小部件',
      addSection: '添加分区',
      sectionLabel: '面板分区',
      sectionTitlePlaceholder: '分区标题',
      sectionSubtitlePlaceholder: '副标题（可选）',
      renameSection: '重命名分区',
      deleteSection: '删除分区',
      unknownWidgetType: '未知的小部件类型：{type}',
      noWidgets: '此分区中还没有小部件',
      addWidgetsHint: '选择“自定义”以添加小部件',
      noSections: '尚未配置分区',
      addSectionsHint: '选择“编辑”以向面板添加分区',
    },
    states: {
      assignedLoadError: '无法加载分配给你的事项',
      assignedEmpty: '目前没有分配给你的事项',
      personalTasksLoadError: '无法加载你的个人任务',
      personalTasksEmpty: '你的个人待办列表为空',
      dailyBriefingUnavailable: '你的每日简报目前不可用。它依赖 AI 集成。如果你刚完成设置，请稍后再试。',
      updatedAt: '更新于 {time}',
      priorityLabel: '优先级：{priority}',
      noWorkspaces: '还没有工作区',
      createWorkspace: '创建一个',
      workspaceAvatarAlt: '{name} 的头像',
      visited: '访问于 {time}',
      noUpcomingMilestones: '没有即将到来的里程碑',
      milestoneProgress: '已完成 {done}/{total}',
      daysOverdue: '逾期 {days} 天',
      daysLeft: '剩余 {days} 天',
      watchedItemsEmpty: '你尚未关注任何事项',
      workspaceWithId: '工作区 {id}',
    },
  },
};
