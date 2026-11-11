/**
 * Treść transakcyjnych i powiadomieniowych wiadomości e-mail.
 *
 * Renderowana przez backend Go (internal/i18n) za pomocą funkcji szablonu `t`,
 * dlatego treść znajduje się obok katalogów interfejsu i podlega tym samym
 * kontrolom zgodności kluczy i symboli zastępczych. Symbole zastępcze używają
 * składni {name}.
 */
export default {
  email: {
    footer: {
      automated: 'To jest automatyczna wiadomość e-mail — nie odpowiadaj na nią.',
      repliable: 'Otrzymujesz tę wiadomość, ponieważ masz u nas otwarte zgłoszenie.',
    },
    common: {
      greeting: 'Cześć {name},',
      openInWindshift: 'Zobacz w Windshift',
      manualLinkHint: 'Jeśli przycisk nie działa, skopiuj i wklej ten adres URL do przeglądarki:',
    },
    duration: {
      minute: '{count} minuta',
      minutes: '{count} minut',
      hour: '{count} godzina',
      hours: '{count} godzin',
      day: '{count} dzień',
      days: '{count} dni',
    },
    magicLink: {
      subject: 'Zaloguj się do swojego portalu',
      heading: 'Zaloguj się do swojego portalu',
      body: 'Kliknij przycisk poniżej, aby się zalogować. Link jest ważny przez {expires}.',
      button: 'Zaloguj się',
      note: 'Jeśli nie prosiłeś o ten link, możesz bezpiecznie zignorować tę wiadomość — bez niego nikt nie może się zalogować.',
    },
    verification: {
      subject: 'Potwierdź swój adres e-mail',
      heading: 'Potwierdź swój adres e-mail',
      body: 'Potwierdź swój adres e-mail, aby dokończyć konfigurację konta.',
      button: 'Potwierdź e-mail',
      note: 'Ten link wygaśnie za {expires}. Jeśli nie utworzyłeś konta Windshift, możesz zignorować tę wiadomość.',
    },
    invitation: {
      subject: 'Zaproszenie do Windshift',
      heading: 'Zaproszenie do Windshift',
      body: 'Ustaw hasło, aby aktywować konto i rozpocząć współpracę z zespołem.',
      button: 'Ustaw hasło',
      note: 'To zaproszenie wygaśnie za {expires}. Jeśli go nie oczekiwałeś, możesz bezpiecznie zignorować tę wiadomość.',
    },
    notificationBatch: {
      subjectOne: 'Windshift — masz 1 nowe powiadomienie',
      subjectOther: 'Windshift — masz {count} nowych powiadomień',
      headingOne: '1 nowe powiadomienie',
      headingOther: '{count} nowych powiadomień',
      intro: 'Cześć {name}, oto co nowego:',
      manage: 'Częstotliwość tych wiadomości możesz zmienić w ustawieniach powiadomień.',
    },
    mention: {
      subject: '{actor} wspomniał o Tobie w {itemKey}',
      body: '{actor} wspomniał o Tobie w {source} przy {itemKey} · {itemTitle}',
      source: {
        comment: 'komentarzu',
        description: 'opisie',
        item: 'elemencie',
      },
    },
    portalReply: {
      subject: 'Odp.: {subject}',
      body: '{author} odpowiedział w {itemKey} · {itemTitle}',
      portalLink: 'Zobacz to zgłoszenie w swoim portalu',
      note: 'Aby kontynuować rozmowę, po prostu odpowiedz na tę wiadomość.',
    },
    approval: {
      subject: 'Prośba o zatwierdzenie: {itemKey} — {itemTitle}',
      heading: 'Prośba o zatwierdzenie',
      body: 'Twoje zatwierdzenie jest wymagane dla {itemKey} · {itemTitle}.',
      body2: 'Kliknij przycisk poniżej, aby przejrzeć i podjąć decyzję. Link jest ważny przez {expires}.',
      button: 'Przejrzyj zatwierdzenie',
      note: 'Jeśli nie oczekiwałeś tej prośby, możesz bezpiecznie zignorować tę wiadomość.',
    },
    intake: {
      subject: 'Otrzymaliśmy Twoje zgłoszenie ({itemKey})',
      heading: 'Otrzymaliśmy Twoje zgłoszenie',
      body: 'Dziękujemy za kontakt. Zarejestrowaliśmy Twoją prośbę jako {itemKey} · {itemTitle}.',
      body2: 'Nasz zespół przejrzy ją i odpowie tak szybko, jak to możliwe.',
      body3: 'Postępy możesz śledzić i dodawać szczegóły w swoim portalu:',
      button: 'Zobacz swoje zgłoszenie',
      note: 'Aby dodać więcej informacji, po prostu odpowiedz na tę wiadomość.',
    },
  },
  notification: {
    dueReminder: {
        title: 'Przypomnienie: termin {itemKey} się zbliża',
        message: 'Termin {itemTitle} upływa {dueDate}',
    },
    mention: {
        title: 'Wspomniano o Tobie',
        comment: '{actor} wspomniał o Tobie w komentarzu do {itemTitle} ({itemKey})',
        description: '{actor} wspomniał o Tobie w opisie {itemTitle} ({itemKey})',
        item: '{actor} wspomniał o Tobie w {itemTitle} ({itemKey})',
    },
    approvalStarted: {
        title: 'Wymagane zatwierdzenie',
        message: 'Twoje zatwierdzenie jest wymagane dla {itemKey}: {itemTitle}',
    },
    approvalEscalated: {
        title: 'Zatwierdzenie przekazane Tobie',
        message: 'Zatwierdzenie zostało przekazane Tobie w {itemKey}: {itemTitle}',
    },
    incident: {
        title: 'Incydent: {itemTitle}',
        message: '{itemKey} został wywołany i wymaga potwierdzenia',
    },
  },
};
