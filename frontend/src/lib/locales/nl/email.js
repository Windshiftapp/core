/**
 * Transactional and notification email copy.
 *
 * Rendered by the Go backend (internal/i18n) via the `t` template function, so
 * the copy lives beside the UI catalogs and is covered by the same key and
 * placeholder parity checks. Placeholders use {name} syntax.
 */
export default {
  email: {
    footer: {
      automated: 'Dit is een geautomatiseerde e-mail — reageer hier niet op.',
      repliable: 'Je ontvangt dit omdat je een openstaand verzoek bij ons hebt.',
    },
    common: {
      greeting: 'Hallo {name},',
      openInWindshift: 'Bekijken in Windshift',
      manualLinkHint: 'Als de knop niet werkt, kopieer en plak dan deze URL in je browser:',
    },
    duration: {
      minute: '{count} minuut',
      minutes: '{count} minuten',
      hour: '{count} uur',
      hours: '{count} uur',
      day: '{count} dag',
      days: '{count} dagen',
    },
    magicLink: {
      subject: 'Aanmelden bij je portaal',
      heading: 'Aanmelden bij je portaal',
      body: 'Klik op de knop hieronder om je aan te melden. De link is {expires} geldig.',
      button: 'Aanmelden',
      note: 'Als je deze link niet hebt aangevraagd, kun je deze e-mail veilig negeren — zonder de link kan niemand zich aanmelden.',
    },
    verification: {
      subject: 'Verifieer je e-mailadres',
      heading: 'Verifieer je e-mail',
      body: 'Bevestig je e-mailadres om het instellen van je account af te ronden.',
      button: 'E-mail verifiëren',
      note: 'Deze link verloopt over {expires}. Als je geen Windshift-account hebt aangemaakt, kun je deze e-mail negeren.',
    },
    invitation: {
      subject: 'Je bent uitgenodigd voor Windshift',
      heading: 'Je bent uitgenodigd voor Windshift',
      body: 'Stel een wachtwoord in om je account te activeren en met je team samen te werken.',
      button: 'Stel je wachtwoord in',
      note: 'Deze uitnodiging verloopt over {expires}. Als je deze niet verwachtte, kun je deze e-mail veilig negeren.',
    },
    notificationBatch: {
      subjectOne: 'Windshift — Je hebt 1 nieuwe melding',
      subjectOther: 'Windshift — Je hebt {count} nieuwe meldingen',
      headingOne: '1 nieuwe melding',
      headingOther: '{count} nieuwe meldingen',
      intro: 'Hallo {name}, dit is er nieuw:',
      manage: 'Beheer hoe vaak je deze e-mails ontvangt in je meldingsvoorkeuren.',
    },
    mention: {
      subject: '{actor} heeft je vermeld in {itemKey}',
      body: '{actor} heeft je vermeld in {source} op {itemKey} · {itemTitle}',
      source: {
        comment: 'een reactie',
        description: 'de beschrijving',
        item: 'een item',
      },
    },
    portalReply: {
      subject: 'Re: {subject}',
      body: '{author} heeft gereageerd op {itemKey} · {itemTitle}',
      portalLink: 'Bekijk dit verzoek in je portaal',
      note: 'Om het gesprek voort te zetten, kun je gewoon op deze e-mail antwoorden.',
    },
    approval: {
      subject: 'Goedkeuring aangevraagd: {itemKey} — {itemTitle}',
      heading: 'Goedkeuring aangevraagd',
      body: 'Je goedkeuring is vereist voor {itemKey} · {itemTitle}.',
      body2: 'Klik op de knop hieronder om te beoordelen en te beslissen. De link is {expires} geldig.',
      button: 'Goedkeuring beoordelen',
      note: 'Als je dit verzoek niet verwachtte, kun je deze e-mail veilig negeren.',
    },
    intake: {
      subject: 'We hebben je inzending ontvangen ({itemKey})',
      heading: 'We hebben je inzending ontvangen',
      body: 'Bedankt voor je bericht. We hebben je verzoek geregistreerd als {itemKey} · {itemTitle}.',
      body2: 'Ons team beoordeelt het en neemt zo snel mogelijk contact met je op.',
      body3: 'Je kunt de voortgang volgen en details toevoegen in je portaal:',
      button: 'Je verzoek bekijken',
      note: 'Om meer informatie toe te voegen, kun je gewoon op deze e-mail antwoorden.',
    },
  },
  notification: {
    dueReminder: {
        title: 'Herinnering: {itemKey} moet binnenkort worden afgerond',
        message: '{itemTitle} moet worden afgerond op {dueDate}',
    },
    mention: {
        title: 'Je bent vermeld',
        comment: '{actor} heeft je vermeld in een reactie op {itemTitle} ({itemKey})',
        description: '{actor} heeft je vermeld in de beschrijving van {itemTitle} ({itemKey})',
        item: '{actor} heeft je vermeld in {itemTitle} ({itemKey})',
    },
    approvalStarted: {
        title: 'Goedkeuring vereist',
        message: 'Je goedkeuring is vereist voor {itemKey}: {itemTitle}',
    },
    approvalEscalated: {
        title: 'Goedkeuring aan jou geëscaleerd',
        message: 'Een goedkeuring is aan jou geëscaleerd voor {itemKey}: {itemTitle}',
    },
    incident: {
        title: 'Incident: {itemTitle}',
        message: '{itemKey} is geactiveerd en moet worden bevestigd',
    },
  },
};
