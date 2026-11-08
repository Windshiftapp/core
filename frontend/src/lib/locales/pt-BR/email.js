export default {
  email: {
    footer: {
      automated: 'Este é um e-mail automático — não responda.',
      repliable: 'Você recebe este e-mail porque tem uma solicitação aberta conosco.',
    },
    common: {
      greeting: 'Olá {name},',
      openInWindshift: 'Ver no Windshift',
      manualLinkHint: 'Se o botão não funcionar, copie e cole esta URL no navegador:',
    },
    duration: {
      minute: '{count} minuto',
      minutes: '{count} minutos',
      hour: '{count} hora',
      hours: '{count} horas',
      day: '{count} dia',
      days: '{count} dias',
    },
    magicLink: {
      subject: 'Entre no seu portal',
      heading: 'Entre no seu portal',
      body: 'Clique no botão abaixo para entrar. O link é válido por {expires}.',
      button: 'Entrar',
      note: 'Se você não solicitou este link, pode ignorar este e-mail — ninguém consegue entrar sem ele.',
    },
    verification: {
      subject: 'Verifique seu e-mail',
      heading: 'Verifique seu e-mail',
      body: 'Confirme seu endereço de e-mail para concluir a configuração da sua conta.',
      button: 'Verificar e-mail',
      note: 'Este link expira em {expires}. Se você não criou uma conta no Windshift, pode ignorar este e-mail.',
    },
    invitation: {
      subject: 'Você foi convidado para o Windshift',
      heading: 'Você foi convidado para o Windshift',
      body: 'Defina uma senha para ativar sua conta e começar a colaborar com sua equipe.',
      button: 'Definir sua senha',
      note: 'Este convite expira em {expires}. Se você não esperava por ele, pode ignorar este e-mail.',
    },
    notificationBatch: {
      subjectOne: 'Windshift — Você tem 1 nova notificação',
      subjectOther: 'Windshift — Você tem {count} novas notificações',
      headingOne: '1 nova notificação',
      headingOther: '{count} novas notificações',
      intro: 'Olá {name}, veja o que há de novo:',
      manage: 'Escolha a frequência destes e-mails nas suas preferências de notificação.',
    },
    mention: {
      subject: '{actor} mencionou você em {itemKey}',
      body: '{actor} mencionou você em {source} em {itemKey} · {itemTitle}',
      source: {
        comment: 'um comentário',
        description: 'a descrição',
        item: 'um item',
      },
    },
    portalReply: {
      subject: 'Re: {subject}',
      body: '{author} respondeu em {itemKey} · {itemTitle}',
      portalLink: 'Ver esta solicitação no seu portal',
      note: 'Para continuar a conversa, basta responder a este e-mail.',
    },
    approval: {
      subject: 'Aprovação solicitada: {itemKey} — {itemTitle}',
      heading: 'Aprovação solicitada',
      body: 'Sua aprovação é necessária em {itemKey} · {itemTitle}.',
      body2:
        'Clique no botão abaixo para revisar e decidir. O link é válido por {expires}.',
      button: 'Revisar aprovação',
      note: 'Se você não esperava esta solicitação, pode ignorar este e-mail.',
    },
    intake: {
      subject: 'Recebemos seu envio ({itemKey})',
      heading: 'Recebemos seu envio',
      body: 'Obrigado por entrar em contato. Registramos sua solicitação como {itemKey} · {itemTitle}.',
      body2: 'Nossa equipe vai analisá-la e responder o mais breve possível.',
      body3: 'Você pode acompanhar o progresso e adicionar detalhes no seu portal:',
      button: 'Ver sua solicitação',
      note: 'Para adicionar mais informações, basta responder a este e-mail.',
    },
  },
  notification: {
    dueReminder: {
        title: "Lembrete: {itemKey} vence em breve",
        message: "{itemTitle} vence em {dueDate}",
    },
    mention: {
        title: "Você foi mencionado",
        comment: "{actor} mencionou você em um comentário em {itemTitle} ({itemKey})",
        description: "{actor} mencionou você na descrição de {itemTitle} ({itemKey})",
        item: "{actor} mencionou você em {itemTitle} ({itemKey})",
    },
    approvalStarted: {
        title: "Aprovação necessária",
        message: "Sua aprovação é necessária em {itemKey}: {itemTitle}",
    },
    approvalEscalated: {
        title: "Aprovação escalada para você",
        message: "Uma aprovação foi escalada para você em {itemKey}: {itemTitle}",
    },
    incident: {
        title: "Incidente: {itemTitle}",
        message: "{itemKey} foi acionado e precisa de confirmação",
    },
  },
};
