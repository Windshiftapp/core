/** WI-726 terminology, orthography, and register overrides. */
export default {
  "auth": {
    "login": "Iniciar sesión",
    "logout": "Cerrar sesión",
    "signIn": "Iniciar sesión",
    "signOut": "Cerrar sesión",
    "forgotPassword": "¿Olvidó su contraseña?",
    "resetPassword": "Restablecer contraseña",
    "changePassword": "Cambiar contraseña",
    "currentPassword": "Contraseña actual",
    "newPassword": "Nueva contraseña",
    "confirmPassword": "Confirmar contraseña",
    "passwordRequired": "La contraseña es obligatoria",
    "emailOrUsername": "Correo electrónico o nombre de usuario",
    "loginTitle": "Inicie sesión en su cuenta",
    "loggingIn": "Iniciando sesión...",
    "logoutConfirm": "¿Está seguro de que desea cerrar sesión?",
    "sessionExpired": "Su sesión ha expirado. Por favor, inicie sesión de nuevo.",
    "twoFactor": "Autenticación de dos factores",
    "enterCode": "Ingrese el código de verificación",
    "resendCode": "Reenviar código",
    "verificationSent": "Código de verificación enviado a su correo",
    "sso": "Inicio de sesión único",
    "orSignInWithPassword": "o inicie sesión con contraseña",
    "signInWithSecurityKey": "Iniciar sesión con llave de seguridad"
  },
  "users": {
    "email": "Correo electrónico",
    "phone": "Teléfono",
    "lastLogin": "Último acceso",
    "lastActive": "Última actividad",
    "confirmDelete": "¿Está seguro de que desea eliminar a {name}? Esta acción no se puede deshacer.",
    "confirmActivate": "¿Está seguro de que desea activar a {name}? Podrán acceder al sistema.",
    "confirmDeactivate": "¿Está seguro de que desea desactivar a {name}? Ya no podran acceder al sistema.",
    "failedToResetPassword": "Error al restablecer contraseña",
    "profileSubtitle": "Administre su información de perfil, avatar y configuración regional",
    "passwordResetRequired": "Se requiere restablecer la contraseña",
    "removeProfilePicture": "¿Está seguro de que desea eliminar su foto de perfil?",
    "languageHint": "Su idioma preferido para la interfaz de la aplicación",
    "saveSettings": "Guardar configuración",
    "connectedAccountsDesc": "Conecte sus cuentas de control de código fuente para crear ramas y solicitudes de incorporación",
    "loadCalendarFeedSettings": "Cargar configuración del feed de calendario",
    "enableCalendarSubscription": "Habilitar suscripción de calendario",
    "calendarSubscriptionDesc": "Genere una URL de suscripción para sincronizar sus elementos de trabajo programados con aplicaciones de calendario externas.",
    "lastSynced": "Última sincronizacion",
    "regenerateUrlNote": "Regenerar la URL invalidará su URL actual. Los calendarios que usen la URL anterior deberán actualizarse.",
    "appleCalendarInstructions": "Archivo > Nueva suscripción de calendario > Pegue la URL",
  },
  "security": {
    "subtitle": "Administrar su configuración de seguridad",
    "credentialsSubtitle": "Administrar sus métodos de autenticación",
    "apiTokensSubtitle": "Crear tokens para acceder a su cuenta programáticamente",
    "tokenWarning": "Asegúrese de copiar su token ahora. No podrá verlo de nuevo.",
    "authenticationFailed": "La autenticación fallo",
    "authenticationCancelled": "La autenticación fue cancelada o fallo. Intente de nuevo.",
    "failedToTestFidoLogin": "Error al probar el inicio de sesión con FIDO",
    "confirmRevokeToken": "¿Está seguro de que desea revocar \"{name}\"? Esta acción no se puede deshacer.",
    "authenticatorAppTotp": "Aplicacion de autenticación (TOTP)"
  },
  "portalLogin": {
    "signInToCustomize": "Inicie sesión para personalizar este portal",
    "emailOrUsername": "Correo electrónico o nombre de usuario",
    "password": "Contraseña",
    "enterPassword": "Ingrese contraseña",
    "signInWithSecurityKey": "Iniciar sesión con llave de seguridad",
    "signingIn": "Iniciando sesión...",
    "signIn": "Iniciar sesión",
    "emailRequired": "Se requiere correo electrónico o nombre de usuario",
    "passwordRequired": "Se requiere contraseña"
  },
  "issueSync": {
    "repositoryDescription": "Seleccione un repositorio vinculado para sincronizar issues.",
    "itemTypeDescription": "Seleccione el tipo de elemento para issues sincronizados. Esto determina qué estados de flujo de trabajo están disponibles para el mapeo.",
    "selectItemTypeHint": "Seleccione un tipo de elemento arriba para configurar el mapeo de estados.",
    "confirmDelete": "¿Estás seguro de que desea eliminar la configuración de sincronización? Los elementos sincronizados permanecerán pero ya no se actualizarán."
  },
  "settings": {
    "boardConfig": {
      "noStatusesMatchSearch": "Ningún estado coincide con su búsqueda"
    },
    "actionCapabilities": {
      "addFirst": "Agregue su primera capacidad",
      "confirmDelete": "¿Estás seguro de que desea eliminar"
    }
  },
  "testing": {
    "createTestRun": "Crear ejecución de prueba",
    "deleteTestPlanConfirm": "¿Está seguro de que desea eliminar este plan de prueba? Esta acción no se puede deshacer.",
    "lastRun": "Última ejecución",
    "finishExecution": "Finalizar ejecución",
    "testRunNotFound": "No se encontro la ejecución de prueba o no hay casos de prueba disponibles",
    "finishTestExecution": "Finalizar ejecución de prueba",
    "finishConfirmMessage": "¿Está seguro de que desea finalizar esta ejecución de prueba? Esto marcara la ejecución como completada.",
    "failedToFinish": "No se pudo finalizar la ejecución de prueba. Intente nuevamente.",
    "deleteTestCaseConfirm": "¿Está seguro de que desea eliminar este caso de prueba? Esta acción no se puede deshacer.",
    "deleteFolderConfirm": "¿Está seguro de que desea eliminar esta carpeta? Esta acción no se puede deshacer.",
    "testCaseStepsInfo": "Despues de crear este caso de prueba, puede agregar pasos individuales con acciones, datos y resultados esperados especificos para una ejecución de prueba precisa.",
    "noLabelsMatchSearch": "Ninguna etiqueta coincide con su búsqueda",
    "adjustSearchOrCreate": "Intente ajustar su búsqueda o cree una nueva etiqueta.",
    "deleteStepConfirm": "¿Está seguro de que desea eliminar este paso de prueba?",
    "actionPlaceholder": "Describa la acción a realizar...",
    "selectPlanAndEnterName": "Seleccione un plan de prueba e ingrese un nombre de ejecución",
    "createTestRunSubtitle": "Iniciar una nueva ejecución de prueba desde un plan de prueba",
    "runName": "Nombre de ejecución",
    "createRun": "Crear ejecución",
    "deleteTestRun": "Eliminar ejecución de prueba",
    "deleteRunConfirm": "¿Está seguro de que desea eliminar \"{name}\"?",
    "failedToDeleteRun": "No se pudo eliminar la ejecución de prueba",
    "failedToCreateRun": "No se pudo crear la ejecución de prueba",
    "createTestRunToExecute": "Cree una ejecución de prueba para comenzar a ejecutar casos de prueba.",
    "continueExecution": "Continuar ejecución",
    "testRunTemplates": "Plantillas de ejecución de prueba",
    "testRunTemplatesSubtitle": "Crear configuraciones de ejecución de prueba reutilizables",
    "createTestRunTemplate": "Crear plantilla de ejecución de prueba",
    "deleteTemplateConfirm": "¿Está seguro de que desea eliminar \"{name}\"? Esto no eliminara las ejecuciones de prueba existentes creadas a partir de esta plantilla.",
    "failedToStartExecution": "No se pudo iniciar la ejecución. Intente nuevamente.",
    "newExecution": "Nueva ejecución",
    "clickExecuteTemplate": "Haga clic en \"Ejecutar plantilla\" para crear su primera ejecución de prueba desde esta plantilla",
    "latestResult": "Último resultado",
    "adjustFiltersOrBroader": "Intente ajustar los filtros o ampliar su búsqueda.",
    "deleteSetConfirm": "¿Está seguro de que desea eliminar \"{name}\"?",
    "startRun": "Iniciar ejecución",
    "useSearchToAddTestCases": "Use la búsqueda de arriba para agregar casos de prueba.",
    "testReportsSubtitle": "Analizar tendencias y resultados de ejecución de pruebas",
    "testRunReport": "Reporte de ejecución de prueba",
    "testRunReportSubtitle": "Ver resultados detallados de una ejecución de prueba especifica",
    "saveConfiguration": "Guardar configuración",
    "executionTrend": "Tendencia de ejecución",
    "lastFailed": "Último fallo",
    "runsCount": "{count} ejecución(es)"
  },
  "testCase": {
    "noExecutions": "Este caso de prueba no se ha ejecutado en ninguna ejecución de prueba reciente."
  },
  "time": {
    "durationHelperText": "Ingrese hora de inicio + duración (2h) para calcular la hora de fin automáticamente, o ingresa hora de inicio + fin para calcular la duración automáticamente. Formatos de tiempo: 1h, 30m, 1h30m, 2h15m, 1d (=8h)",
    "onboarding": {
      "subtitle": "Vamos a crear su primera organización de cliente y proyecto para comenzar",
      "createCustomerTitle": "Cree su primera organización de cliente",
      "createCustomerDescription": "Una organización de cliente representa la empresa o entidad para la que está trabajando. Puede ser una organización cliente, su empleador o su propia empresa.",
      "createProjectTitle": "Cree su primer proyecto",
      "createProjectDescription": "Los proyectos ayudan a organizar su trabajo dentro de un cliente. Puedes registrar tiempo en proyectos específicos.",
      "hourlyRateHint": "Puedes configurar esto después si no está seguro"
    },
    "categories": {
      "createFirstHint": "Cree su primera categoría para organizar proyectos",
      "confirmDelete": "¿Estás seguro de que desea eliminar \"{name}\"?"
    },
    "reports": {
      "subtitle": "Analiza sus datos de seguimiento de tiempo y exporta reportes",
      "selectProject": "Seleccione un proyecto",
      "noProjectSelected": "Seleccione un proyecto para ver su reporte"
    },
    "projects": {
      "noProjects": "No se encontraron proyectos. Crea su primer proyecto para comenzar a registrar tiempo.",
      "confirmDelete": "¿Estás seguro de que desea eliminar \"{name}\"?"
    },
    "entry": {
      "subtitle": "Registre sus horas de trabajo y administra entradas de tiempo",
      "failedToSave": "Error al guardar la entrada de tiempo. Por favor, verifique su entrada.",
      "confirmDelete": "¿Estás seguro de que desea eliminar esta entrada de tiempo?",
      "noEntries": "No se encontraron entradas de tiempo. Registra su primera entrada de tiempo para comenzar."
    },
    "organizations": {
      "subtitle": "Administre sus organizaciones de clientes",
      "noOrganizations": "No se encontraron organizaciones. Crea su primera organización para comenzar.",
      "confirmDelete": "¿Estás seguro de que desea eliminar \"{name}\"?"
    },
    "permissions": {
      "confirmRemove": "¿Estás seguro de que desea eliminar a {name}?"
    }
  },
  "notifications": {
    "verifyEmail": "Por favor verifique su dirección de correo electrónico",
    "verifyEmailDescription": "Hemos enviado un enlace de verificación a su correo. Haz clic en el enlace para completar la configuración de su cuenta."
  },
  "channel": {
    "secretPlaceholder": "Ingrese un secreto para actualizar, deja en blanco para conservar el actual",
    "tenantIdHelp": "Use \"common\" para permitir cualquier cuenta de Microsoft, o ingresa un ID de inquilino específico",
    "saveAndConnect": "Guarde la configuración, luego conecta su buzón",
    "selectWorkspaceFirst": "Seleccione un espacio de trabajo primero",
    "testEmailPlaceholder": "Ingrese un correo para enviar la prueba"
  },
  "portal": {
    "signInTitle": "Inicie sesión en su cuenta",
    "signInDescription": "Ingrese su correo electrónico para recibir un enlace de inicio de sesión",
    "enterEmail": "Ingrese su dirección de correo electrónico",
    "noAccountNeeded": "No necesita crear una cuenta. Solo ingresa su correo electrónico y te enviaremos un enlace de inicio de sesión.",
    "checkYourEmail": "Revise su correo electrónico",
    "magicLinkSent": "Hemos enviado un enlace de inicio de sesión a su correo. Haz clic en el enlace para acceder a su cuenta del portal.",
    "verifying": "Verificando su enlace...",
    "pleaseWait": "Por favor espera mientras iniciamos su sesión.",
    "signInToAccess": "Inicie sesión para acceder al portal y enviar solicitudes",
    "enterPassword": "Ingrese su contraseña",
    "addRequestTypeSubtitle": "Agregar un nuevo tipo de solicitud para su portal",
    "describeRequest": "Por favor, describe su solicitud",
    "createsItemType": "Cree tipo de elemento",
    "customize": {
      "confirmDeleteRequestType": "¿Estás seguro de que desea eliminar este tipo de solicitud? Esta acción no se puede deshacer.",
      "docmostDescription": "Conecte su base de conocimientos Docmost para habilitar la búsqueda en el portal",
      "docmostShareLinkHelp": "Ingrese el enlace de compartir completo de Docmost (ej., https://wiki.example.com/share/u1gkl0jk1u)",
      "expectedFormat": "Formato esperado: https://su-dominio.com/share/share-id",
      "docmostStep1": "Abra su espacio de Docmost",
      "docmostStep2": "Haga clic en el botón Compartir"
    }
  },
  "requestForm": {
    "enterTitle": "Ingrese un título para su solicitud",
    "describeRequest": "Por favor, describe su solicitud",
    "yourName": "Su nombre",
    "yourEmail": "Su correo electrónico",
    "emailPlaceholder": "su@correo.com",
    "emailFollowUp": "Usaremos esto para dar seguimiento a su solicitud"
  },
  "requestTypeFields": {
    "helpTextPlaceholder": "Ingrese texto de ayuda para mostrar debajo del campo...",
    "noFieldsMatch": "No hay campos que coincidan con su búsqueda"
  },
  "forms": {
    "createsItemType": "Cree tipo de elemento"
  },
  "hub": {
    "noRequestsDescription": "Las solicitudes enviadas a través de sus portales aparecerán aquí"
  },
  "workflows": {
    "noWorkflowsFound": "No se encontraron flujos de trabajo. Crea su primer flujo de trabajo para comenzar.",
    "confirmDeleteWorkflow": "¿Estás seguro de que desea eliminar \"{name}\"? Esta acción no se puede deshacer.",
    "confirmDeleteTransition": "¿Estás seguro de que desea eliminar esta transición?",
    "failedToLoadDesigner": "No se pudo cargar el diseñador de flujo de trabajo. Por favor, actualiza la página e intente de nuevo.",
    "refreshAndTryAgain": "Por favor, actualiza la página e intente de nuevo",
    "transitionHint3": "Haga clic y arrastra los bordes para reconectarlos",
    "startDesigning": "Comience a diseñar su flujo de trabajo",
    "clickStatusesToAdd": "Haga clic en los estados del panel izquierdo para agregarlos al lienzo",
    "connectByDragging": "Conecte los estados arrastrando desde los puntos de conexión"
  },
  "screensPage": {
    "noFieldsMatch": "Ningún campo coincide con su búsqueda",
    "noScreens": "No se encontraron pantallas. Crea su primera pantalla para comenzar.",
    "confirmDeleteScreen": "¿Estás seguro de que desea eliminar la pantalla \"{name}\"? Esto afectará a todos los espacios de trabajo que usen esta pantalla."
  },
  "pickers": {
    "defaultConfigurationDescription": "Use la configuración predeterminada del espacio de trabajo",
    "noEntitiesMatchSearch": "Ningún {entities} coincide con su búsqueda",
    "noRepositoriesMatchSearch": "Ningún repositorio coincide con su búsqueda"
  },
  "editors": {
    "enterText": "Ingrese texto...",
    "selectDate": "Seleccione una fecha...",
    "noFieldsMatchSearch": "Ningún campo coincide con su búsqueda"
  },
  "dialogs": {
    "confirmations": {
      "deleteItem": "¿Estás seguro de que desea eliminar \"{name}\"? Esta acción no se puede deshacer.",
      "deleteSection": "¿Estás seguro de que desea eliminar esta sección?",
      "discardChanges": "Tienes cambios sin guardar. ¿Estás seguro de que desea cancelar?",
      "dismissAllNotifications": "¿Estás seguro de que desea descartar todas las notificaciones? Esta acción no se puede deshacer.",
      "removeAvatar": "¿Estás seguro de que desea eliminar su foto de perfil?",
      "revokeCalendarFeed": "¿Estás seguro de que desea revocar la URL de su feed de calendario? Los calendarios que usen esta URL dejarán de sincronizarse.",
      "deleteTheme": "¿Estás seguro de que desea eliminar este tema? Esta acción no se puede deshacer.",
      "resetBoardConfig": "¿Estás seguro de que desea restablecer la configuración del tablero por defecto? Esto eliminará su configuración personalizada.",
      "deleteCustomField": "¿Estás seguro de que desea eliminar el campo personalizado \"{name}\"? Se eliminará de todos los proyectos.",
      "deleteLinkType": "¿Estás seguro de que desea eliminar este tipo de enlace? También se eliminarán todos los enlaces de este tipo.",
      "deleteAsset": "¿Estás seguro de que desea eliminar este activo?",
      "deleteAssetSet": "¿Estás seguro de que desea eliminar este conjunto de activos? Se eliminarán todos los activos, tipos y categorías dentro de él.",
      "deleteAssetType": "¿Estás seguro de que desea eliminar este tipo de activo? Los activos que usen este tipo ya no tendrán un tipo asignado.",
      "deleteCategory": "¿Estás seguro de que desea eliminar esta categoría? Las subcategorías se moverán a la categoría principal.",
      "revokeRole": "¿Estás seguro de que desea revocar este rol?",
      "quitApplication": "¿Estás seguro de que desea salir de la aplicación? El servidor se apagará.",
      "deleteConnection": "¿Estás seguro de que desea eliminar esta conexión? Esta acción no se puede deshacer.",
      "deleteScreen": "¿Estás seguro de que desea eliminar la pantalla \"{name}\"? Esto afectará a todos los espacios de trabajo que usen esta pantalla."
    },
    "alerts": {
      "timerSyncing": "El temporizador se está sincronizando. Por favor espera e intente de nuevo.",
      "failedToGeneratePdf": "Error al generar PDF. Por favor intente de nuevo.",
      "failedToSaveWorkspace": "Error al guardar proyecto. Por favor verifique su entrada e intente de nuevo.",
      "statusInUseByTransitions": "No se puede eliminar \"{name}\" porque está siendo usada en {count} transición(es) del flujo de trabajo. Para eliminar este estado, ve a Gestión de flujos de trabajo, elimina todas las transiciones que usen este estado e intente eliminarlo de nuevo."
    }
  },
  "components": {
    "diagram": {
      "confirmDelete": "¿Estás seguro de que desea eliminar este diagrama?",
      "unsavedChangesConfirm": "Tienes cambios sin guardar. ¿Estás seguro de que desea cerrar?"
    },
    "userAvatar": {
      "profileSubtitle": "Administre su perfil y configuración",
      "securitySubtitle": "Administre contraseñas, 2FA y tokens de API"
    }
  },
  "widgets": {
    "milestoneProgress": {
      "emptySubtitle": "Cree hitos para seguir el progreso"
    },
    "myTasks": {
      "loadingText": "Cargando sus tareas..."
    }
  },
  "onboarding": {
    "selectWorkspace": "Seleccione un espacio de trabajo para comenzar",
    "noWorkspacesAvailable": "Aún no hay espacios de trabajo disponibles. Contacta a su administrador para obtener acceso a un espacio de trabajo."
  },
  "jiraImport": {
    "form": {
      "tokenHelpCloud": "desde la configuración de su cuenta de Atlassian",
      "tokenHelpDatacenter": "Cree un Token de Acceso Personal en la configuración de su perfil de Jira"
    },
    "messages": {
      "selectConnection": "Seleccione una conexión existente o crea una nueva",
      "credentialsHelpCloud": "Ingrese sus credenciales de Jira Cloud. Puedes generar un token de API desde la configuración de su cuenta de Atlassian.",
      "credentialsHelpDatacenter": "Ingrese un Token de Acceso Personal desde la configuración de su perfil de Jira Data Center.",
      "reviewSummary": "Revise el resumen de importación antes de continuar. Esta operación puede tardar varios minutos para proyectos grandes."
    },
    "projects": {
      "selected": "Seleccione proyectos para importar ({selected} de {total} seleccionados)"
    },
    "import": {
      "readyDesc": "Haga clic en \"Iniciar importación\" para comenzar a importar {count} elementos."
    }
  },
  "fields": {
    "subtitle": "Definir campos personalizados para sus elementos",
    "assetHint": "Los campos de activo permiten a los usuarios seleccionar activos de un conjunto de activos especificado. Opcionalmente puede filtrar los activos disponibles usando una consulta QL."
  },
  "categories": {
    "addFirstCategoryHint": "Agregue su primera categoría arriba."
  },
  "projects": {
    "subtitle": "Administrar sus proyectos"
  },
  "iterations": {
    "confirmDelete": "¿Estás seguro de que desea eliminar la iteración \"{name}\"?"
  },
  "milestones": {
    "noMilestonesDescription": "Cree su primer hito para hacer seguimiento de lanzamientos y fechas límite.",
    "confirmDelete": "¿Estás seguro de que desea eliminar el hito \"{name}\"?"
  },
  "assets": {
    "noAssetSetsDesc": "Cree su primer conjunto de activos para comenzar a administrar activos.",
    "selectAnAssetSet": "Seleccione un conjunto de activos",
    "noAssetTypesDesc": "Cree tipos de activos para categorizar sus activos.",
    "noCategoriesDesc": "Cree categorías para organizar sus activos.",
    "noRoleAssignmentsDesc": "Agregue asignaciones de roles para controlar quién puede acceder a este conjunto de activos."
  },
  "personal": {
    "placeholderAccomplishments": "Describa sus logros principales...",
    "startWriting": "Comience a escribir su reflexión..."
  },
  "migrationAssistant": {
    "pleaseSelectTargetStatuses": "Seleccione los estados de destino para todos los elementos que requieren migración.",
    "pleaseSelectTargetItemTypes": "Seleccione los tipos de elementos de destino para todos los elementos que requieren migración.",
    "pleaseSelectTargetPriorities": "Seleccione las prioridades de destino para todos los elementos que requieren migración."
  },
  "setup": {
    "setupMessage": "Vamos a configurar su sistema de gestión de trabajo",
    "adminAccountDesc": "Esta cuenta tendrá acceso completo para administrar su instalación de {appName}.",
    "whatsNextCreateWorkspace": "Crear su primer espacio de trabajo",
    "invalidEmail": "Ingrese un correo electrónico válido"
  },
  "createModal": {
    "selectWorkspaceFirst": "Seleccione un espacio de trabajo primero"
  },
  "scm": {
    "confirmRemoveLink": "¿Estás seguro de que desea eliminar este vínculo?",
    "connectYourAccount": "Conecte su cuenta de {provider}"
  },
  "organization": {
    "pleaseSelectImage": "Seleccione un archivo de imagen"
  },
  "actions": {
    "createFirst": "Crear Su Primera Acción",
    "noActionsDescription": "Cree acciones para automatizar sus flujos de trabajo basados en eventos de elementos",
    "confirmDelete": "¿Estás seguro de que desea eliminar la acción \"{name}\"?",
    "templates": {
      "help": "Aplica una plantilla de automatización integrada a este espacio de trabajo. Cada aplicación crea una nueva acción que puede editar después."
    },
    "tipClickToEdit": "Haga clic en un nodo para configurarlo",
    "config": {
      "commentPlaceholder": "Ingrese el texto del comentario. Usa {{item.title}} para variables.",
      "notifyPlaceholder": "Ingrese el mensaje. Usa {{item.title}} para variables.",
      "sourceAssetFieldHint": "Seleccione el campo del elemento que contiene el activo vinculado",
      "milestonePickerHint": "Guarde los IDs de hitos para la acción; los nombres solo se muestran al editar.",
      "assetTitleHint": "Use {{item.title}} u otras variables"
    },
    "test": {
      "description": "Seleccione un elemento para ejecutar esta acción. La acción se ejecutará inmediatamente, sin esperar el disparador normal."
    },
    "placeholders": {
      "description": "Use estos marcadores en su plantilla. Se reemplazarán con valores reales cuando se ejecute la acción."
    }
  },
  "profile": {
    "leave": {
      "subtitle": "Programa sus ausencias y elige un suplente que cubra sus guardias.",
      "cannotBeSelf": "No puede elegirte a ti mismo como suplente."
    }
  },
  "conditionSets": {
    "modeValidatorDesc": "Muestra un error cuando se intente la transición"
  }
};
