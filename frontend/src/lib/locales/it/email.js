export default {
  email: {
    footer: {
      automated: "Questa è un'e-mail automatica: non rispondere.",
      repliable: 'Ricevi questa e-mail perché hai una richiesta aperta con noi.',
    },
    common: {
      greeting: 'Ciao {name},',
      openInWindshift: 'Visualizza in Windshift',
      manualLinkHint: 'Se il pulsante non funziona, copia e incolla questo URL nel browser:',
    },
    duration: {
      minute: '{count} minuto',
      minutes: '{count} minuti',
      hour: '{count} ora',
      hours: '{count} ore',
      day: '{count} giorno',
      days: '{count} giorni',
    },
    magicLink: {
      subject: 'Accedi al tuo portale',
      heading: 'Accedi al tuo portale',
      body: 'Fai clic sul pulsante per accedere. Il link è valido per {expires}.',
      button: 'Accedi',
      note: 'Se non hai richiesto questo link, puoi ignorare questa e-mail: nessuno può accedere senza di esso.',
    },
    verification: {
      subject: 'Verifica il tuo indirizzo e-mail',
      heading: 'Verifica il tuo indirizzo e-mail',
      body: "Conferma il tuo indirizzo e-mail per completare la configurazione dell'account.",
      button: 'Verifica e-mail',
      note: 'Questo link scade tra {expires}. Se non hai creato un account Windshift, puoi ignorare questa e-mail.',
    },
    invitation: {
      subject: 'Sei stato invitato a Windshift',
      heading: 'Sei stato invitato a Windshift',
      body: "Imposta una password per attivare l'account e iniziare a collaborare con il tuo team.",
      button: 'Imposta la password',
      note: 'Questo invito scade tra {expires}. Se non lo aspettavi, puoi ignorare questa e-mail.',
    },
    notificationBatch: {
      subjectOne: 'Windshift — Hai 1 nuova notifica',
      subjectOther: 'Windshift — Hai {count} nuove notifiche',
      headingOne: '1 nuova notifica',
      headingOther: '{count} nuove notifiche',
      intro: 'Ciao {name}, ecco le novità:',
      manage: 'Scegli la frequenza di queste e-mail nelle tue preferenze di notifica.',
    },
    mention: {
      subject: '{actor} ti ha menzionato in {itemKey}',
      body: '{actor} ti ha menzionato in {source} su {itemKey} · {itemTitle}',
      source: {
        comment: 'un commento',
        description: 'la descrizione',
        item: 'un elemento',
      },
    },
    portalReply: {
      subject: 'Re: {subject}',
      body: '{author} ha risposto su {itemKey} · {itemTitle}',
      portalLink: 'Visualizza questa richiesta nel tuo portale',
      note: 'Per continuare la conversazione, rispondi semplicemente a questa e-mail.',
    },
    approval: {
      subject: 'Approvazione richiesta: {itemKey} — {itemTitle}',
      heading: 'Approvazione richiesta',
      body: 'La tua approvazione è richiesta per {itemKey} · {itemTitle}.',
      body2:
        'Fai clic sul pulsante per esaminare e decidere. Il link è valido per {expires}.',
      button: 'Esamina approvazione',
      note: 'Se non aspettavi questa richiesta, puoi ignorare questa e-mail.',
    },
    intake: {
      subject: 'Abbiamo ricevuto la tua richiesta ({itemKey})',
      heading: 'Abbiamo ricevuto la tua richiesta',
      body: 'Grazie per averci contattato. Abbiamo registrato la tua richiesta come {itemKey} · {itemTitle}.',
      body2: 'Il nostro team la esaminerà e ti risponderà il prima possibile.',
      body3: 'Puoi seguire i progressi e aggiungere dettagli nel tuo portale:',
      button: 'Visualizza la tua richiesta',
      note: 'Per aggiungere altre informazioni, rispondi semplicemente a questa e-mail.',
    },
  },
  notification: {
    dueReminder: {
        title: "Promemoria: {itemKey} scade presto",
        message: "{itemTitle} scade il {dueDate}",
    },
    mention: {
        title: "Sei stato menzionato",
        comment: "{actor} ti ha menzionato in un commento su {itemTitle} ({itemKey})",
        description: "{actor} ti ha menzionato nella descrizione di {itemTitle} ({itemKey})",
        item: "{actor} ti ha menzionato in {itemTitle} ({itemKey})",
    },
    approvalStarted: {
        title: "Approvazione richiesta",
        message: "La tua approvazione è richiesta per {itemKey}: {itemTitle}",
    },
    approvalEscalated: {
        title: "Approvazione escalata a te",
        message: "Un'approvazione è stata escalata a te su {itemKey}: {itemTitle}",
    },
    incident: {
        title: "Incidente: {itemTitle}",
        message: "{itemKey} è stato attivato e richiede una conferma",
    },
  },
};
