export default {
  email: {
    footer: {
      automated: 'Dies ist eine automatische E-Mail – bitte antworten Sie nicht darauf.',
      repliable: 'Sie erhalten diese E-Mail, weil Sie eine offene Anfrage bei uns haben.',
    },
    common: {
      greeting: 'Hallo {name},',
      openInWindshift: 'In Windshift ansehen',
      manualLinkHint:
        'Falls der Button nicht funktioniert, kopieren Sie diese URL in Ihren Browser:',
    },
    duration: {
      minute: '{count} Minute',
      minutes: '{count} Minuten',
      hour: '{count} Stunde',
      hours: '{count} Stunden',
      day: '{count} Tag',
      days: '{count} Tage',
    },
    magicLink: {
      subject: 'Bei Ihrem Portal anmelden',
      heading: 'Bei Ihrem Portal anmelden',
      body: 'Klicken Sie auf den Button, um sich anzumelden. Der Link ist {expires} gültig.',
      button: 'Anmelden',
      note: 'Wenn Sie diesen Link nicht angefordert haben, können Sie diese E-Mail ignorieren – ohne ihn kann sich niemand anmelden.',
    },
    verification: {
      subject: 'E-Mail-Adresse bestätigen',
      heading: 'E-Mail-Adresse bestätigen',
      body: 'Bitte bestätigen Sie Ihre E-Mail-Adresse, um die Einrichtung Ihres Kontos abzuschließen.',
      button: 'E-Mail bestätigen',
      note: 'Dieser Link läuft in {expires} ab. Wenn Sie kein Windshift-Konto erstellt haben, können Sie diese E-Mail ignorieren.',
    },
    invitation: {
      subject: 'Sie wurden zu Windshift eingeladen',
      heading: 'Sie wurden zu Windshift eingeladen',
      body: 'Legen Sie ein Passwort fest, um Ihr Konto zu aktivieren und mit Ihrem Team zusammenzuarbeiten.',
      button: 'Passwort festlegen',
      note: 'Diese Einladung läuft in {expires} ab. Wenn Sie sie nicht erwartet haben, können Sie diese E-Mail ignorieren.',
    },
    notificationBatch: {
      subjectOne: 'Windshift – Sie haben 1 neue Benachrichtigung',
      subjectOther: 'Windshift – Sie haben {count} neue Benachrichtigungen',
      headingOne: '1 neue Benachrichtigung',
      headingOther: '{count} neue Benachrichtigungen',
      intro: 'Hallo {name}, das ist neu:',
      manage:
        'Legen Sie in Ihren Benachrichtigungseinstellungen fest, wie oft Sie diese E-Mails erhalten.',
    },
    mention: {
      subject: '{actor} hat Sie in {itemKey} erwähnt',
      body: '{actor} hat Sie in {source} zu {itemKey} · {itemTitle} erwähnt',
      source: {
        comment: 'einem Kommentar',
        description: 'der Beschreibung',
        item: 'einem Element',
      },
    },
    portalReply: {
      subject: 'Re: {subject}',
      body: '{author} hat auf {itemKey} · {itemTitle} geantwortet',
      portalLink: 'Diese Anfrage in Ihrem Portal ansehen',
      note: 'Um die Konversation fortzusetzen, antworten Sie einfach auf diese E-Mail.',
    },
    approval: {
      subject: 'Genehmigung erforderlich: {itemKey} — {itemTitle}',
      heading: 'Genehmigung erforderlich',
      body: 'Ihre Genehmigung ist für {itemKey} · {itemTitle} erforderlich.',
      body2:
        'Klicken Sie auf den Button, um zu prüfen und zu entscheiden. Der Link ist {expires} gültig.',
      button: 'Genehmigung prüfen',
      note: 'Wenn Sie diese Anfrage nicht erwartet haben, können Sie diese E-Mail ignorieren.',
    },
    intake: {
      subject: 'Wir haben Ihre Anfrage erhalten ({itemKey})',
      heading: 'Wir haben Ihre Anfrage erhalten',
      body: 'Vielen Dank für Ihre Nachricht. Wir haben Ihre Anfrage als {itemKey} · {itemTitle} erfasst.',
      body2: 'Unser Team prüft sie und meldet sich so schnell wie möglich bei Ihnen.',
      body3: 'Den Fortschritt können Sie in Ihrem Portal verfolgen und Details ergänzen:',
      button: 'Ihre Anfrage ansehen',
      note: 'Um weitere Informationen hinzuzufügen, antworten Sie einfach auf diese E-Mail.',
    },
  },
  notification: {
    dueReminder: {
        title: "Erinnerung: {itemKey} ist bald fällig",
        message: "{itemTitle} ist am {dueDate} fällig",
    },
    mention: {
        title: "Sie wurden erwähnt",
        comment: "{actor} hat Sie in einem Kommentar zu {itemTitle} ({itemKey}) erwähnt",
        description: "{actor} hat Sie in der Beschreibung von {itemTitle} ({itemKey}) erwähnt",
        item: "{actor} hat Sie in {itemTitle} ({itemKey}) erwähnt",
    },
    approvalStarted: {
        title: "Genehmigung erforderlich",
        message: "Ihre Genehmigung ist für {itemKey} erforderlich: {itemTitle}",
    },
    approvalEscalated: {
        title: "Genehmigung an Sie eskaliert",
        message: "Eine Genehmigung wurde an Sie eskaliert: {itemKey}: {itemTitle}",
    },
    incident: {
        title: "Vorfall: {itemTitle}",
        message: "{itemKey} wurde ausgelöst und muss bestätigt werden",
    },
  },
};
