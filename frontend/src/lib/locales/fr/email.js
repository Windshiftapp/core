export default {
  email: {
    footer: {
      automated: 'Ceci est un e-mail automatique — merci de ne pas y répondre.',
      repliable: 'Vous recevez cet e-mail car vous avez une demande ouverte chez nous.',
    },
    common: {
      greeting: 'Bonjour {name},',
      openInWindshift: 'Voir dans Windshift',
      manualLinkHint:
        'Si le bouton ne fonctionne pas, copiez et collez cette URL dans votre navigateur :',
    },
    duration: {
      minute: '{count} minute',
      minutes: '{count} minutes',
      hour: '{count} heure',
      hours: '{count} heures',
      day: '{count} jour',
      days: '{count} jours',
    },
    magicLink: {
      subject: 'Connectez-vous à votre portail',
      heading: 'Connectez-vous à votre portail',
      body: 'Cliquez sur le bouton ci-dessous pour vous connecter. Le lien est valable {expires}.',
      button: 'Se connecter',
      note: "Si vous n'avez pas demandé ce lien, vous pouvez ignorer cet e-mail — personne ne peut se connecter sans lui.",
    },
    verification: {
      subject: 'Vérifiez votre adresse e-mail',
      heading: 'Vérifiez votre adresse e-mail',
      body: 'Veuillez confirmer votre adresse e-mail pour terminer la configuration de votre compte.',
      button: "Vérifier l'e-mail",
      note: "Ce lien expire dans {expires}. Si vous n'avez pas créé de compte Windshift, vous pouvez ignorer cet e-mail.",
    },
    invitation: {
      subject: 'Vous avez été invité à Windshift',
      heading: 'Vous avez été invité à Windshift',
      body: 'Définissez un mot de passe pour activer votre compte et commencer à collaborer avec votre équipe.',
      button: 'Définir votre mot de passe',
      note: "Cette invitation expire dans {expires}. Si vous ne l'attendiez pas, vous pouvez ignorer cet e-mail.",
    },
    notificationBatch: {
      subjectOne: 'Windshift — Vous avez 1 nouvelle notification',
      subjectOther: 'Windshift — Vous avez {count} nouvelles notifications',
      headingOne: '1 nouvelle notification',
      headingOther: '{count} nouvelles notifications',
      intro: 'Bonjour {name}, voici les nouveautés :',
      manage: 'Choisissez la fréquence de ces e-mails dans vos préférences de notification.',
    },
    mention: {
      subject: '{actor} vous a mentionné dans {itemKey}',
      body: '{actor} vous a mentionné dans {source} sur {itemKey} · {itemTitle}',
      source: {
        comment: 'un commentaire',
        description: 'la description',
        item: 'un élément',
      },
    },
    portalReply: {
      subject: 'Re : {subject}',
      body: '{author} a répondu sur {itemKey} · {itemTitle}',
      portalLink: 'Voir cette demande dans votre portail',
      note: 'Pour poursuivre la conversation, répondez simplement à cet e-mail.',
    },
    approval: {
      subject: 'Approbation demandée : {itemKey} — {itemTitle}',
      heading: 'Approbation demandée',
      body: 'Votre approbation est requise pour {itemKey} · {itemTitle}.',
      body2:
        'Cliquez sur le bouton ci-dessous pour examiner et décider. Le lien est valable {expires}.',
      button: "Examiner l'approbation",
      note: "Si vous n'attendiez pas cette demande, vous pouvez ignorer cet e-mail.",
    },
    intake: {
      subject: 'Nous avons bien reçu votre demande ({itemKey})',
      heading: 'Nous avons bien reçu votre demande',
      body: 'Merci de nous avoir contactés. Nous avons enregistré votre demande sous {itemKey} · {itemTitle}.',
      body2: "Notre équipe va l'examiner et vous répondra dès que possible.",
      body3: 'Vous pouvez suivre son avancement et ajouter des détails dans votre portail :',
      button: 'Voir votre demande',
      note: 'Pour ajouter des informations, répondez simplement à cet e-mail.',
    },
  },
};
