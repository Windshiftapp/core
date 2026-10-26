<script>
  import { actionCredentials } from '../api/ai.js';
  import ActionCredentialsManager from './ActionCredentialsManager.svelte';

  const api = {
    list: () => actionCredentials.getAllGlobal(),
    create: (data) => actionCredentials.createGlobal(data),
    update: (id, data) => actionCredentials.updateGlobal(id, data),
    rotate: (id, secret) => actionCredentials.rotateGlobal(id, secret),
    remove: (id) => actionCredentials.deleteGlobal(id),
  };

  const i18n = {
    title: 'settings.adminOperations.actionCredentials.title',
    subtitle: 'settings.adminOperations.actionCredentials.subtitle',
    add: 'settings.adminOperations.actionCredentials.add',
    empty: 'settings.adminOperations.actionCredentials.empty',
    addFirst: 'settings.adminOperations.actionCredentials.addFirst',
  };
</script>

<ActionCredentialsManager mode="global" {api} {i18n} />
