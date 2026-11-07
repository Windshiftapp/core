export default {
  email: {
    footer: {
      automated: 'Este es un correo automático: no respondas a este mensaje.',
      repliable: 'Recibes este correo porque tienes una solicitud abierta con nosotros.',
    },
    common: {
      greeting: 'Hola {name},',
      openInWindshift: 'Ver en Windshift',
      manualLinkHint: 'Si el botón no funciona, copia y pega esta URL en tu navegador:',
    },
    duration: {
      minute: '{count} minuto',
      minutes: '{count} minutos',
      hour: '{count} hora',
      hours: '{count} horas',
      day: '{count} día',
      days: '{count} días',
    },
    magicLink: {
      subject: 'Inicia sesión en tu portal',
      heading: 'Inicia sesión en tu portal',
      body: 'Haz clic en el botón para iniciar sesión. El enlace es válido durante {expires}.',
      button: 'Iniciar sesión',
      note: 'Si no solicitaste este enlace, puedes ignorar este correo: nadie puede iniciar sesión sin él.',
    },
    verification: {
      subject: 'Verifica tu correo electrónico',
      heading: 'Verifica tu correo electrónico',
      body: 'Confirma tu correo electrónico para terminar de configurar tu cuenta.',
      button: 'Verificar correo',
      note: 'Este enlace caduca en {expires}. Si no creaste una cuenta de Windshift, puedes ignorar este correo.',
    },
    invitation: {
      subject: 'Te han invitado a Windshift',
      heading: 'Te han invitado a Windshift',
      body: 'Establece una contraseña para activar tu cuenta y empezar a colaborar con tu equipo.',
      button: 'Establecer contraseña',
      note: 'Esta invitación caduca en {expires}. Si no la esperabas, puedes ignorar este correo.',
    },
    notificationBatch: {
      subjectOne: 'Windshift: tienes 1 notificación nueva',
      subjectOther: 'Windshift: tienes {count} notificaciones nuevas',
      headingOne: '1 notificación nueva',
      headingOther: '{count} notificaciones nuevas',
      intro: 'Hola {name}, esto es lo nuevo:',
      manage: 'Configura la frecuencia de estos correos en tus preferencias de notificaciones.',
    },
    mention: {
      subject: '{actor} te ha mencionado en {itemKey}',
      body: '{actor} te ha mencionado en {source} en {itemKey} · {itemTitle}',
      source: {
        comment: 'un comentario',
        description: 'la descripción',
        item: 'un elemento',
      },
    },
    portalReply: {
      subject: 'Re: {subject}',
      body: '{author} respondió en {itemKey} · {itemTitle}',
      portalLink: 'Ver esta solicitud en tu portal',
      note: 'Para continuar la conversación, responde a este correo.',
    },
    approval: {
      subject: 'Aprobación solicitada: {itemKey} — {itemTitle}',
      heading: 'Aprobación solicitada',
      body: 'Se requiere tu aprobación en {itemKey} · {itemTitle}.',
      body2:
        'Haz clic en el botón para revisar y decidir. El enlace es válido durante {expires}.',
      button: 'Revisar aprobación',
      note: 'Si no esperabas esta solicitud, puedes ignorar este correo.',
    },
    intake: {
      subject: 'Hemos recibido tu envío ({itemKey})',
      heading: 'Hemos recibido tu envío',
      body: 'Gracias por contactarnos. Hemos registrado tu solicitud como {itemKey} · {itemTitle}.',
      body2: 'Nuestro equipo la revisará y te responderá lo antes posible.',
      body3: 'Puedes seguir su progreso y añadir detalles en tu portal:',
      button: 'Ver tu solicitud',
      note: 'Para añadir más información, responde a este correo.',
    },
  },
  notification: {
    dueReminder: {
        title: "Recordatorio: {itemKey} vence pronto",
        message: "{itemTitle} vence el {dueDate}",
    },
    mention: {
        title: "Te han mencionado",
        comment: "{actor} te ha mencionado en un comentario en {itemTitle} ({itemKey})",
        description: "{actor} te ha mencionado en la descripción de {itemTitle} ({itemKey})",
        item: "{actor} te ha mencionado en {itemTitle} ({itemKey})",
    },
    approvalStarted: {
        title: "Aprobación requerida",
        message: "Se requiere tu aprobación en {itemKey}: {itemTitle}",
    },
    approvalEscalated: {
        title: "Aprobación escalada a ti",
        message: "Se te ha escalado una aprobación en {itemKey}: {itemTitle}",
    },
    incident: {
        title: "Incidente: {itemTitle}",
        message: "{itemKey} se ha activado y necesita confirmación",
    },
  },
};
