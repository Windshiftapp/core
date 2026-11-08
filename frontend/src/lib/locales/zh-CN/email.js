export default {
  email: {
    footer: {
      automated: '这是一封自动发送的邮件，请勿回复。',
      repliable: '您收到此邮件是因为您有一个未结的请求。',
    },
    common: {
      greeting: '您好，{name}：',
      openInWindshift: '在 Windshift 中查看',
      manualLinkHint: '如果按钮无法使用，请将以下链接复制并粘贴到浏览器中：',
    },
    duration: {
      minute: '{count} 分钟',
      minutes: '{count} 分钟',
      hour: '{count} 小时',
      hours: '{count} 小时',
      day: '{count} 天',
      days: '{count} 天',
    },
    magicLink: {
      subject: '登录您的门户',
      heading: '登录您的门户',
      body: '点击下方按钮登录。该链接有效期为 {expires}。',
      button: '登录',
      note: '如果您没有请求此链接，可以放心忽略此邮件——没有它任何人都无法登录。',
    },
    verification: {
      subject: '验证您的电子邮箱',
      heading: '验证您的电子邮箱',
      body: '请确认您的电子邮箱以完成账户设置。',
      button: '验证邮箱',
      note: '该链接将在 {expires} 后过期。如果您没有创建 Windshift 账户，可以忽略此邮件。',
    },
    invitation: {
      subject: '您已被邀请加入 Windshift',
      heading: '您已被邀请加入 Windshift',
      body: '设置密码以激活账户并开始与团队协作。',
      button: '设置密码',
      note: '此邀请将在 {expires} 后过期。如果您并未期待它，可以放心忽略此邮件。',
    },
    notificationBatch: {
      subjectOne: 'Windshift——您有 1 条新通知',
      subjectOther: 'Windshift——您有 {count} 条新通知',
      headingOne: '1 条新通知',
      headingOther: '{count} 条新通知',
      intro: '您好，{name}，以下是新动态：',
      manage: '您可以在通知偏好设置中管理接收这些邮件的频率。',
    },
    mention: {
      subject: '{actor} 在 {itemKey} 中提到了您',
      body: '{actor} 在 {itemKey} · {itemTitle} 的{source}中提到了您',
      source: {
        comment: '评论',
        description: '描述',
        item: '工作项',
      },
    },
    portalReply: {
      subject: 'Re: {subject}',
      body: '{author} 回复了 {itemKey} · {itemTitle}',
      portalLink: '在您的门户中查看此请求',
      note: '如需继续对话，直接回复此邮件即可。',
    },
    approval: {
      subject: '需要审批：{itemKey} — {itemTitle}',
      heading: '需要审批',
      body: '需要您审批 {itemKey} · {itemTitle}。',
      body2: '点击下方按钮进行查看并做出决定。该链接有效期为 {expires}。',
      button: '查看审批',
      note: '如果您并未期待此请求，可以放心忽略此邮件。',
    },
    intake: {
      subject: '我们已收到您的提交（{itemKey}）',
      heading: '我们已收到您的提交',
      body: '感谢您联系我们。我们已将您的请求记录为 {itemKey} · {itemTitle}。',
      body2: '我们的团队将尽快审核并回复您。',
      body3: '您可以在门户中跟踪进度并补充详细信息：',
      button: '查看您的请求',
      note: '如需补充更多信息，直接回复此邮件即可。',
    },
  },
  notification: {
    dueReminder: {
        title: "提醒：{itemKey} 即将到期",
        message: "{itemTitle} 将于 {dueDate} 到期",
    },
    mention: {
        title: "有人提到了您",
        comment: "{actor} 在 {itemTitle}（{itemKey}）的评论中提到了您",
        description: "{actor} 在 {itemTitle}（{itemKey}）的描述中提到了您",
        item: "{actor} 在 {itemTitle}（{itemKey}）中提到了您",
    },
    approvalStarted: {
        title: "需要审批",
        message: "需要您审批 {itemKey}：{itemTitle}",
    },
    approvalEscalated: {
        title: "审批已升级给您",
        message: "有一项审批已升级给您：{itemKey}：{itemTitle}",
    },
    incident: {
        title: "事件：{itemTitle}",
        message: "{itemKey} 已触发，需要确认",
    },
  },
};
