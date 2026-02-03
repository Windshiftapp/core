/**
 * Actions automation translations (Spanish)
 */
export default {
  actions: {
    title: 'Acciones',
    description: 'Automatizar flujos de trabajo con acciones basadas en reglas',
    create: 'Crear Acción',
    createFirst: 'Crear Tu Primera Acción',
    noActions: 'Aún no hay acciones',
    noActionsDescription:
      'Crea acciones para automatizar tus flujos de trabajo basados en eventos de elementos',
    enabled: 'Activado',
    disabled: 'Desactivado',
    enable: 'Activar',
    disable: 'Desactivar',
    viewLogs: 'Ver Registros',
    confirmDelete: '¿Estás seguro de que deseas eliminar la acción "{name}"?',
    failedToSave: 'Error al guardar la acción',
    newAction: 'Nueva Acción',

    trigger: {
      statusTransition: 'Cambio de Estado',
      itemCreated: 'Elemento Creado',
      itemUpdated: 'Elemento Actualizado',
      itemLinked: 'Elemento Vinculado',
      respondToCascades: 'Responder a cambios activados por acciones',
      respondToCascadesHint:
        'Cuando está activado, esta acción también se ejecutará cuando sea activada por otras acciones, no solo por cambios del usuario.',
    },

    nodes: {
      trigger: 'Disparador',
      setField: 'Establecer Campo',
      setStatus: 'Establecer Estado',
      addComment: 'Agregar Comentario',
      notifyUser: 'Notificar Usuario',
      condition: 'Condición',
    },

    addNodes: 'Agregar Nodos',
    tips: 'Consejos',
    tipDragToConnect: 'Arrastra desde los conectores para conectar nodos',
    tipClickToEdit: 'Haz clic en un nodo para configurarlo',
    tipConditionBranches: 'Las condiciones tienen ramas verdadero/falso',

    nodeConfig: 'Configuración del Nodo',
    config: {
      from: 'Desde',
      to: 'Hasta',
      selectField: 'Seleccionar campo...',
      selectStatus: 'Seleccionar estado...',
      enterComment: 'Ingresar comentario...',
      selectRecipient: 'Seleccionar destinatario...',
      setCondition: 'Establecer condición...',
      targetStatus: 'Estado Destino',
      fieldName: 'Nombre del Campo',
      value: 'Valor',
      commentContent: 'Contenido del Comentario',
      commentPlaceholder: 'Ingresa el texto del comentario. Usa {{item.title}} para variables.',
      privateComment: 'Comentario privado (solo interno)',
      fieldToCheck: 'Campo a Verificar',
      operator: 'Operador',
      compareValue: 'Valor de Comparación',
      private: 'Privado',
      triggerType: 'Tipo de Disparador',
      fromStatus: 'Desde Estado',
      toStatus: 'Hasta Estado',
      anyStatus: 'Cualquier Estado',
    },

    recipients: {
      assignee: 'Asignado',
      creator: 'Creador',
      specific: 'Usuarios Específicos',
    },

    condition: {
      true: 'Sí',
      false: 'No',
    },

    operators: {
      equals: 'Igual a',
      notEquals: 'Diferente de',
      contains: 'Contiene',
      greaterThan: 'Mayor que',
      lessThan: 'Menor que',
      isEmpty: 'Está Vacío',
      isNotEmpty: 'No Está Vacío',
    },

    logs: {
      title: 'Registros de Ejecución',
      noLogs: 'Sin registros de ejecución',
      status: 'Estado',
      running: 'Ejecutando',
      completed: 'Completado',
      failed: 'Fallido',
      skipped: 'Omitido',
      startedAt: 'Iniciado a las',
      completedAt: 'Completado a las',
      error: 'Error',
      details: 'Detalles',
      viewDetails: 'Ver Detalles',
    },

    trace: {
      title: 'Detalles de Ejecución',
      noSteps: 'No se registraron pasos de ejecución',
      setStatus: 'Estado cambiado de "{from}" a "{to}"',
      setField: '{field} establecido de "{from}" a "{to}"',
      addComment: 'Comentario {prefix}agregado: "{content}"',
      notifyUser: 'Notificación enviada a {count} usuario(s)',
      notifySkipped: 'Notificación omitida: {reason}',
      conditionResult: 'La condición resultó en {result}',
    },

    test: {
      title: 'Probar Acción',
      description:
        'Selecciona un elemento para ejecutar esta acción. La acción se ejecutará inmediatamente, sin esperar el disparador normal.',
      selectItem: 'Seleccionar Elemento',
      itemPlaceholder: 'Buscar un elemento...',
      execute: 'Ejecutar Acción',
      run: 'Prueba',
      executionFailed: 'Error al ejecutar la acción',
      executionQueued: 'Acción en cola para ejecución',
    },

    placeholders: {
      title: 'Marcadores Disponibles',
      description:
        'Usa estos marcadores en tu plantilla. Se reemplazarán con valores reales cuando se ejecute la acción.',
      showReference: 'Mostrar referencia de marcadores',
      categories: {
        item: 'Campos del Elemento',
        user: 'Usuario Actual',
        old: 'Valores Anteriores',
        trigger: 'Contexto del Disparador',
      },
      item: {
        title: 'Título del elemento',
        id: 'ID del elemento',
        statusId: 'ID del estado',
        assigneeId: 'ID del usuario asignado',
        any: 'Cualquier campo del elemento',
      },
      user: {
        name: 'Nombre completo del usuario',
        email: 'Correo del usuario',
        id: 'ID del usuario',
      },
      old: {
        description: 'Valor anterior antes del cambio',
        example: 'Valor anterior de cualquier campo',
      },
      trigger: {
        itemId: 'ID del elemento disparador',
        workspaceId: 'ID del espacio de trabajo',
      },
    },
  },
};
