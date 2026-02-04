/**
 * Spanish (es) - Channels, Notifications, Portal, and Request Form translations
 *
 * This module contains translations for:
 * - notifications: Notification UI and email verification
 * - channels: Channel management and integration settings
 * - channel: Channel configuration (portal, webhook, email)
 * - portal: Customer portal settings and request types
 * - requestForm: Portal request submission form
 * - requestTypeFields: Field configuration for request types
 */

export default {
  notifications: {
    title: 'Notificaciones',
    subtitle: 'Administrar configuracion de notificaciones',
    notification: 'Notificacion',
    notifications_one: '{count} notificacion',
    notifications_other: '{count} notificaciones',
    markAsRead: 'Marcar como leida',
    markAllAsRead: 'Marcar todas como leidas',
    clearAll: 'Limpiar todas',
    noNotifications: 'Sin notificaciones',
    notificationSettings: 'Configuracion de notificaciones',
    emailNotifications: 'Notificaciones por correo',
    pushNotifications: 'Notificaciones push',
    channels: 'Canales',
    searchNotifications: 'Buscar notificaciones...',
  },

  channels: {
    title: 'Canales',
    subtitle: 'Configurar canales de notificacion',
    channel: 'Canal',
    channels_one: '{count} canal',
    channels_other: '{count} canales',
    createChannel: 'Crear canal',
    editChannel: 'Editar canal',
    deleteChannel: 'Eliminar canal',
    channelName: 'Nombre del canal',
    channelType: 'Tipo de canal',
    noChannels: 'No se encontraron canales',
    channelCreated: 'Canal creado correctamente',
    channelUpdated: 'Canal actualizado correctamente',
    channelDeleted: 'Canal eliminado correctamente',
    cannotDeleteDefault: 'No se puede eliminar el canal de notificacion predeterminado',
    searchChannels: 'Buscar canales...',
    testWebhook: 'Probar webhook',
    webhookSent: 'Webhook de prueba enviado correctamente!',
    webhookFailed: 'Prueba de webhook fallida',
  },

  channel: {
    enableEmail: 'Activar canal de correo electrónico',
    emailIsActive: 'El canal de correo electrónico está activo y procesando correos',
    emailIsInactive: 'El canal de correo electrónico está desactivado',

    // Processing Log
    processingLog: 'Registro de procesamiento',
    emailLog: {
      syncStatus: 'Estado de sincronización',
      lastChecked: 'Última verificación',
      never: 'Nunca',
      errors: 'Errores',
      noErrors: 'Sin errores',
      from: 'De',
      subject: 'Asunto',
      result: 'Resultado',
      processedAt: 'Procesado',
      newItem: 'Nuevo elemento {key}',
      commentOn: 'Comentario en {key}',
      noEmails: 'No se han procesado correos aún',
      previous: 'Anterior',
      next: 'Siguiente',
      page: 'Página {page} de {total}',
    },
  },

  portal: {
    title: 'Portal',
    subtitle: 'Configuracion del portal de clientes',
    portalTitle: 'Titulo del portal',
    portalDescription: 'Descripcion del portal (opcional)',
    portalSlug: 'Slug del portal',
    requestTypes: 'Tipos de solicitud',
    createRequestType: 'Crear tipo de solicitud',
    editRequestType: 'Editar tipo de solicitud',
    addRequestTypeSubtitle: 'Agregar un nuevo tipo de solicitud para su portal',
    editRequestTypeSubtitle: 'Actualizar detalles del tipo de solicitud',
    requestTypeCreated: 'Tipo de solicitud creado correctamente!',
    requestTypeUpdated: 'Tipo de solicitud actualizado correctamente!',
    exampleTitle: 'ej., Portal de soporte al cliente',
    exampleStatus: 'ej., Abierto, En progreso, Resuelto',
    describeRequest: 'Por favor, describa su solicitud',
    removeFromSection: 'Quitar de la seccion',
    removeLink: 'Quitar enlace',
    backToApp: 'Volver a la aplicacion',
    customize: 'Personalizar',
    lightMode: 'Modo claro',
    darkMode: 'Modo oscuro',
    signIn: 'Iniciar sesion',
    signOut: 'Cerrar sesion',
    myRequests: 'Mis solicitudes',
    backToPortal: 'Volver al portal',
    guestUser: 'Usuario invitado',
    notSignedIn: 'No ha iniciado sesion',
    addLink: 'Agregar enlace',
    addSection: 'Agregar seccion',
    noContentSections: 'Aun no hay secciones de contenido configuradas.',
    dropHereToAdd: 'Soltar aqui para agregar tipo de solicitud',
    noRequestTypesInSection:
      'Aun no hay tipos de solicitud en esta seccion. Arrastre tipos de solicitud aqui desde la barra lateral.',
  },

  requestForm: {},

  requestTypeFields: {
    addNewStep: 'Agregar nuevo paso',
    removeCurrentStep: 'Quitar paso actual',
    removeField: 'Quitar campo',
  },
};
