/**
 * Spanish (es) - Workspace-related translations
 *
 * This module contains the following sections:
 * - workspaces: Workspace management translations
 * - items: Work items (issues, tasks, etc.) translations
 * - comments: Comment-related translations
 * - todo: Personal tasks and todo list translations
 * - collectionTree: Tree view translations
 * - collections: Saved queries and filters translations
 * - links: Item links translations
 */

export default {
  workspaces: {
    title: 'Espacios de trabajo',
    subtitle: 'Administrar sus espacios de trabajo y proyectos',
    workspace: 'Espacio de trabajo',
    workspaces_one: '{count} espacio de trabajo',
    workspaces_other: '{count} espacios de trabajo',
    createWorkspace: 'Crear espacio de trabajo',
    editWorkspace: 'Editar espacio de trabajo',
    deleteWorkspace: 'Eliminar espacio de trabajo',
    switchWorkspace: 'Cambiar espacio de trabajo',
    workspaceName: 'Nombre del espacio de trabajo',
    workspaceKey: 'Clave del espacio de trabajo',
    workspaceDescription: 'Descripcion',
    members: 'Miembros',
    settings: 'Configuracion del espacio de trabajo',
    noWorkspaces: 'No se encontraron espacios de trabajo',
    selectWorkspace: 'Seleccionar espacio de trabajo',
    currentWorkspace: 'Espacio de trabajo actual',
    workspaceCreated: 'Espacio de trabajo creado correctamente',
    workspaceUpdated: 'Espacio de trabajo actualizado correctamente',
    workspaceDeleted: 'Espacio de trabajo eliminado correctamente',
    customers: {
      title: 'Clientes',
      subtitle: 'Administrar clientes y organizaciones del portal',
      addCustomer: 'Agregar cliente',
      unassignedCustomers: 'Clientes no asignados',
      customerCount: '{count} cliente{count === 1 ? "" : "s"}',
      failedToLoadCustomers: 'Error al cargar clientes',
      failedToLoadOrganisations: 'Error al cargar organizaciones',
      failedToAssignCustomer: 'Error al asignar cliente a organizacion',
      deleteCustomer: 'Eliminar cliente',
      confirmDeleteCustomer: 'Esta seguro de que desea eliminar "{name}"?',
      manageOrganisations: 'Administrar organizaciones',
      searchOrganisations: 'Buscar organizaciones...',
      noOrganisationsFound: 'No se encontraron organizaciones',
      noCustomersFound: 'No se encontraron clientes',
      unassigned: 'No asignado',
      allCustomersAssigned: 'Todos los clientes estan asignados a organizaciones'
    }
  },

  items: {
    title: 'Elementos',
    subtitle: 'Ver y administrar elementos de trabajo',
    item: 'Elemento',
    items_one: '{count} elemento',
    items_other: '{count} elementos',
    createItem: 'Crear elemento',
    editItem: 'Editar elemento',
    deleteItem: 'Eliminar elemento',
    viewItem: 'Ver elemento',
    itemKey: 'Clave',
    itemTitle: 'Titulo',
    itemDescription: 'Descripcion',
    itemType: 'Tipo de elemento',
    itemStatus: 'Estado',
    itemPriority: 'Prioridad',
    assignee: 'Responsable',
    reporter: 'Reportador',
    dueDate: 'Fecha de vencimiento',
    startDate: 'Fecha de inicio',
    estimate: 'Estimacion',
    timeSpent: 'Tiempo empleado',
    remaining: 'Restante',
    parent: 'Principal',
    children: 'Secundarios',
    subtasks: 'Subtareas',
    linkedItems: 'Elementos vinculados',
    attachments: 'Adjuntos',
    comments: 'Comentarios',
    activity: 'Actividad',
    history: 'Historial',
    noItems: 'No se encontraron elementos',
    noItemsInFilter: 'Ningun elemento coincide con el filtro actual',
    createToStart: 'Cree un elemento para comenzar',
    itemCreated: 'Elemento creado correctamente',
    itemUpdated: 'Elemento actualizado correctamente',
    itemDeleted: 'Elemento eliminado correctamente',
    assignedToYou: 'Asignado a usted',
    createdByYou: 'Creado por usted',
    recentlyViewed: 'Visto recientemente',
    recentlyUpdated: 'Actualizado recientemente'
  },

  comments: {},

  todo: {},

  collectionTree: {},

  collections: {},

  links: {
    title: 'Enlaces',
    subtitle: 'Administrar enlaces de elementos',
    addLink: 'Agregar enlace',
    removeLink: 'Quitar enlace',
    linkText: 'Texto del enlace',
    linkUrl: 'URL'
  }
};
