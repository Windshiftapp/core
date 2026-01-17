/**
 * German (de) - Authentication and user related translations
 * Includes: auth, users, security, portalLogin
 */
export default {
  auth: {
    login: 'Anmelden',
    logout: 'Abmelden',
    register: 'Registrieren',
    signIn: 'Anmelden',
    signOut: 'Abmelden',
    signUp: 'Registrieren',
    forgotPassword: 'Passwort vergessen?',
    resetPassword: 'Passwort zurücksetzen',
    changePassword: 'Passwort ändern',
    email: 'E-Mail',
    password: 'Passwort',
    confirmPassword: 'Passwort bestätigen',
    currentPassword: 'Aktuelles Passwort',
    newPassword: 'Neues Passwort',
    rememberMe: 'Angemeldet bleiben',
    or: 'oder',
    continueWith: 'Fortfahren mit {provider}',

    // Login form
    welcomeBack: 'Willkommen zurück',
    loginSubtitle: 'Geben Sie Ihre Anmeldedaten ein, um fortzufahren',
    emailOrUsername: 'E-Mail oder Benutzername',
    staySignedIn: 'Angemeldet bleiben',
    signInToContinue: 'Melden Sie sich an, um fortzufahren',
    noAccount: 'Noch kein Konto?',
    hasAccount: 'Bereits ein Konto?',

    // Registration
    createAccount: 'Konto erstellen',
    firstName: 'Vorname',
    lastName: 'Nachname',
    username: 'Benutzername',
    agreeToTerms: 'Ich stimme den Nutzungsbedingungen zu',

    // Password reset
    resetPasswordTitle: 'Passwort zurücksetzen',
    resetPasswordInstructions: 'Geben Sie Ihre E-Mail-Adresse ein und wir senden Ihnen einen Link zum Zurücksetzen Ihres Passworts.',
    sendResetLink: 'Link senden',
    backToLogin: 'Zurück zur Anmeldung',
    resetLinkSent: 'Link zum Zurücksetzen gesendet',
    checkYourEmail: 'Überprüfen Sie Ihre E-Mail',

    // Two-factor authentication
    twoFactorAuth: 'Zwei-Faktor-Authentifizierung',
    enterCode: 'Code eingeben',
    verifyCode: 'Code verifizieren',
    useBackupCode: 'Backup-Code verwenden',
    trustDevice: 'Diesem Gerät vertrauen',

    // Errors
    invalidCredentials: 'Ungültige E-Mail oder Passwort',
    accountLocked: 'Konto gesperrt. Bitte kontaktieren Sie den Support.',
    sessionExpired: 'Sitzung abgelaufen. Bitte erneut anmelden.',
    emailRequired: 'E-Mail ist erforderlich',
    passwordRequired: 'Passwort ist erforderlich',

    // Loading states
    signingIn: 'Anmeldung läuft...',
    signingUp: 'Registrierung läuft...',
    signingOut: 'Abmeldung läuft...',

    // Security key
    touchSecurityKey: 'Berühren Sie Ihren Sicherheitsschlüssel...',
    signInWithSecurityKey: 'Mit Sicherheitsschlüssel anmelden',
    webAuthnNotSupported: 'WebAuthn wird von diesem Browser nicht unterstützt'
  },

  users: {
    title: 'Benutzerverwaltung',
    subtitle: 'Benutzerkonfigurationen verwalten',
    user: 'Benutzer',
    users_one: '{count} Benutzer',
    users_other: '{count} Benutzer',
    addUser: 'Benutzer hinzufügen',
    createUser: 'Benutzer erstellen',
    editUser: 'Benutzer bearbeiten',
    deleteUser: 'Benutzer löschen',
    inviteUser: 'Benutzer einladen',
    profile: 'Profil',
    profiles: 'Profile',

    // User properties
    firstName: 'Vorname',
    lastName: 'Nachname',
    displayName: 'Anzeigename',
    email: 'E-Mail',
    username: 'Benutzername',
    role: 'Rolle',
    roles: 'Rollen',
    status: 'Status',
    avatar: 'Avatar',
    jobTitle: 'Berufsbezeichnung',
    department: 'Abteilung',
    location: 'Standort',
    timezone: 'Zeitzone',
    language: 'Sprache',
    lastLogin: 'Letzte Anmeldung',
    createdAt: 'Erstellt am',

    // User states
    active: 'Aktiv',
    inactive: 'Inaktiv',
    pending: 'Ausstehend',
    invited: 'Eingeladen',
    suspended: 'Gesperrt',

    // Actions
    activate: 'Aktivieren',
    deactivate: 'Deaktivieren',
    suspend: 'Sperren',
    resendInvite: 'Einladung erneut senden',
    resetPassword: 'Passwort zurücksetzen',

    // Messages
    noUsers: 'Keine Benutzer gefunden',
    userCreated: 'Benutzer erfolgreich erstellt',
    userUpdated: 'Benutzer erfolgreich aktualisiert',
    userDeleted: 'Benutzer erfolgreich gelöscht',
    inviteSent: 'Einladung gesendet',

    // Confirmations & Errors
    activateUser: 'Benutzer aktivieren',
    deactivateUser: 'Benutzer deaktivieren',
    confirmDelete: 'Möchten Sie {name} wirklich löschen? Diese Aktion kann nicht rückgängig gemacht werden.',
    confirmActivate: 'Möchten Sie {name} wirklich aktivieren? Der Benutzer kann dann auf das System zugreifen.',
    confirmDeactivate: 'Möchten Sie {name} wirklich deaktivieren? Der Benutzer kann nicht mehr auf das System zugreifen.',
    failedToLoad: 'Benutzer konnten nicht geladen werden',
    failedToSave: 'Benutzer konnte nicht gespeichert werden',
    failedToDelete: 'Benutzer konnte nicht gelöscht werden',
    failedToActivate: 'Benutzer konnte nicht aktiviert werden',
    failedToDeactivate: 'Benutzer konnte nicht deaktiviert werden',
    failedToResetPassword: 'Passwort konnte nicht zurückgesetzt werden',

    // Search & Filter
    searchUsers: 'Benutzer suchen...',
    filterByRole: 'Nach Rolle filtern',
    filterByStatus: 'Nach Status filtern',

    // Profile page
    editProfile: 'Profil bearbeiten',
    changeAvatar: 'Avatar ändern',
    removeAvatar: 'Avatar entfernen',
    personalInfo: 'Persönliche Informationen',
    preferences: 'Einstellungen',
    accountSettings: 'Kontoeinstellungen',
    fullName: 'Vollständiger Name',
    myProfile: 'Mein Profil',
    profileSubtitle: 'Verwalten Sie Ihre Profilinformationen, Ihr Avatar und Ihre regionalen Einstellungen',
    profileInformation: 'Profilinformationen',
    profilePicture: 'Profilbild',
    uploadAndManageAvatar: 'Ihr Avatarbild hochladen und verwalten',
    currentProfilePicture: 'Aktuelles Profilbild',
    customAvatarActive: 'Ihr benutzerdefinierter Avatar ist aktiv',
    usingDefaultAvatar: 'Standard-Avatar wird verwendet',
    avatarRecommendation: 'Empfohlen: Quadratisches Bild, mindestens 200x200 Pixel',
    uploadNewAvatar: 'Neuen Avatar hochladen',
    uploadAvatar: 'Avatar hochladen',
    uploadingAvatar: 'Avatar wird hochgeladen...',
    avatarFileHint: 'Wählen Sie eine Bilddatei (JPEG, PNG, GIF, WebP). Maximal 50MB.',
    removeProfilePicture: 'Möchten Sie Ihr Profilbild wirklich entfernen?',
    regionalSettings: 'Regionale Einstellungen',
    regionalSettingsDesc: 'Konfigurieren Sie Ihre Zeitzone und Sprachpräferenzen',
    timezoneHint: 'Wird für die Anzeige von Datum und Uhrzeit in Ihrer lokalen Zeitzone verwendet',
    languageHint: 'Ihre bevorzugte Sprache für die Anwendungsoberfläche',
    saveSettings: 'Einstellungen speichern',
    settingsSaved: 'Einstellungen erfolgreich gespeichert',
    connectedAccounts: 'Verbundene Konten',
    connectedAccountsDesc: 'Verbinden Sie Ihre Quellcodeverwaltungskonten, um Branches und Pull Requests zu erstellen',
    calendarIntegration: 'Kalenderintegration',
    calendarIntegrationDesc: 'Abonnieren Sie Ihre geplanten Elemente in externen Kalender-Apps',
    loadCalendarFeedSettings: 'Kalender-Feed-Einstellungen laden',
    enableCalendarSubscription: 'Kalenderabonnement aktivieren',
    calendarSubscriptionDesc: 'Generieren Sie eine Abonnement-URL, um Ihre geplanten Arbeitselemente mit externen Kalenderanwendungen zu synchronisieren.',
    generateCalendarFeedUrl: 'Kalender-Feed-URL generieren',
    yourCalendarFeedUrl: 'Ihre Kalender-Feed-URL',
    showFullUrl: 'Vollständige URL anzeigen',
    calendarFeedWarning: 'Teilen Sie diese URL nicht, da sie Zugriff auf Ihre geplanten Elemente gewährt.',
    lastSynced: 'Zuletzt synchronisiert',
    howToSubscribe: 'Wie abonnieren',
    copyFeedUrlStep: 'Kopieren Sie die Feed-URL oben',
    regenerateUrl: 'URL neu generieren',
    revokeFeed: 'Feed widerrufen',
    regenerateUrlNote: 'Das Neugenerieren der URL macht Ihre aktuelle URL ungültig. Kalender, die die alte URL verwenden, müssen aktualisiert werden.',
    calendarFeedsDisabled: 'Kalender-Feeds wurden von Ihrem Administrator deaktiviert.',
    googleCalendar: 'Google Kalender',
    googleCalendarInstructions: 'Einstellungen > Kalender hinzufügen > Von URL > URL einfügen',
    outlook: 'Outlook',
    outlookInstructions: 'Kalender hinzufügen > Aus dem Web abonnieren > URL einfügen',
    appleCalendar: 'Apple Kalender',
    appleCalendarInstructions: 'Ablage > Neues Kalenderabonnement > URL einfügen'
  },

  security: {
    title: 'Sicherheit',
    subtitle: 'Ihre Sicherheitseinstellungen verwalten',
    credentials: 'Sicherheits-Anmeldedaten',
    credentialsSubtitle: 'Ihre Authentifizierungsmethoden verwalten',
    apiTokens: 'API-Token',
    apiTokensSubtitle: 'Token erstellen, um programmatisch auf Ihr Konto zuzugreifen',
    createToken: 'Token erstellen',
    revokeToken: 'Token widerrufen',
    tokenName: 'Token-Name',
    tokenCreated: 'Token erfolgreich erstellt',
    tokenRevoked: 'Token erfolgreich widerrufen',
    copyToken: 'Token kopieren',
    tokenWarning: 'Kopieren Sie Ihren Token jetzt. Sie werden ihn nicht mehr sehen können.'
  },

  portalLogin: {
    welcomeBack: 'Willkommen zurück',
    signInToCustomize: 'Melden Sie sich an, um dieses Portal anzupassen',
    emailOrUsername: 'E-Mail oder Benutzername',
    enterEmailOrUsername: 'E-Mail oder Benutzername eingeben',
    password: 'Passwort',
    enterPassword: 'Passwort eingeben',
    keepMeSignedIn: '30 Tage angemeldet bleiben',
    or: 'oder',
    touchSecurityKey: 'Berühren Sie Ihren Sicherheitsschlüssel...',
    signInWithSecurityKey: 'Mit Sicherheitsschlüssel anmelden',
    signingIn: 'Anmelden...',
    signIn: 'Anmelden',
    emailRequired: 'E-Mail oder Benutzername ist erforderlich',
    passwordRequired: 'Passwort ist erforderlich',
    webAuthnNotSupported: 'WebAuthn wird von diesem Browser nicht unterstützt'
  }
};
