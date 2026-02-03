/**
 * Spanish (es) - Admin locale strings
 * Contains settings, roles, and permissions translations
 */

export default {
  // Settings (merged from general and detailed settings sections)
  settings: {
    title: 'Configuracion',
    subtitle: 'Configure las opciones de la aplicacion',
    general: 'General',
    appearance: 'Apariencia',
    notifications: 'Notificaciones',
    security: 'Seguridad',
    privacy: 'Privacidad',
    language: 'Idioma',
    timezone: 'Zona horaria',
    dateFormat: 'Formato de fecha',
    timeFormat: 'Formato de hora',
    theme: 'Tema',
    lightMode: 'Modo claro',
    darkMode: 'Modo oscuro',
    systemDefault: 'Sistema predeterminado',
    admin: 'Administracion',
    systemSettings: 'Configuracion del sistema',
    organizationSettings: 'Configuracion de la organizacion',
    workspaceSettings: 'Configuracion del espacio de trabajo',
  },

  // Roles and Permissions
  roles: {
    title: 'Roles',
    subtitle: 'Administrar roles y niveles de acceso',
    role: 'Rol',
    roles_one: '{count} rol',
    roles_other: '{count} roles',
    createRole: 'Crear rol',
    editRole: 'Editar rol',
    deleteRole: 'Eliminar rol',
    roleName: 'Nombre del rol',
    roleDescription: 'Descripcion',
    permissions: 'Permisos',
    members: 'Miembros',
    assignRole: 'Asignar rol',
    removeRole: 'Quitar rol',
    noRoles: 'No se encontraron roles',
    roleCreated: 'Rol creado correctamente',
    roleUpdated: 'Rol actualizado correctamente',
    roleDeleted: 'Rol eliminado correctamente',
    cannotDeleteSystemRole: 'Los roles del sistema no se pueden eliminar',
  },

  // Permissions
  permissions: {
    title: 'Permisos',
    subtitle: 'Configurar conjuntos de permisos',
    permission: 'Permiso',
    permissionSet: 'Conjunto de permisos',
    grantPermission: 'Otorgar permiso',
    revokePermission: 'Revocar permiso',
    read: 'Leer',
    write: 'Escribir',
    delete: 'Eliminar',
    admin: 'Admin',
    manage: 'Administrar',
    view: 'Ver',
    edit: 'Editar',
    create: 'Crear',
  },
};
