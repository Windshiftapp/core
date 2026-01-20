<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import { HelpCircle } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let { onclose } = $props();

  const placeholderCategories = [
    {
      key: 'item',
      items: [
        { placeholder: '{{item.title}}', descKey: 'item.title' },
        { placeholder: '{{item.id}}', descKey: 'item.id' },
        { placeholder: '{{item.status_id}}', descKey: 'item.statusId' },
        { placeholder: '{{item.assignee_id}}', descKey: 'item.assigneeId' },
        { placeholder: '{{item.*}}', descKey: 'item.any' }
      ]
    },
    {
      key: 'user',
      items: [
        { placeholder: '{{user.name}}', descKey: 'user.name' },
        { placeholder: '{{user.email}}', descKey: 'user.email' },
        { placeholder: '{{user.id}}', descKey: 'user.id' }
      ]
    },
    {
      key: 'old',
      items: [
        { placeholder: '{{old.status_id}}', descKey: 'old.description' },
        { placeholder: '{{old.*}}', descKey: 'old.example' }
      ]
    },
    {
      key: 'trigger',
      items: [
        { placeholder: '{{trigger.item_id}}', descKey: 'trigger.itemId' },
        { placeholder: '{{trigger.workspace_id}}', descKey: 'trigger.workspaceId' }
      ]
    }
  ];
</script>

<Modal isOpen={true} onclose={onclose} maxWidth="max-w-lg">
  <ModalHeader
    title={t('actions.placeholders.title')}
    icon={HelpCircle}
    onClose={onclose}
  />
  <div class="p-6">
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      {t('actions.placeholders.description')}
    </p>

    <div class="space-y-5">
      {#each placeholderCategories as category}
        <div>
          <h4 class="text-xs font-semibold uppercase tracking-wide mb-2" style="color: var(--ds-text-subtlest);">
            {t(`actions.placeholders.categories.${category.key}`)}
          </h4>
          <div class="rounded-lg border overflow-hidden" style="border-color: var(--ds-border);">
            <table class="w-full text-sm">
              <tbody>
                {#each category.items as item, i}
                  <tr class={i > 0 ? 'border-t' : ''} style="border-color: var(--ds-border);">
                    <td class="px-3 py-2 font-mono text-xs" style="background-color: var(--ds-surface); color: var(--ds-interactive); width: 45%;">
                      {item.placeholder}
                    </td>
                    <td class="px-3 py-2" style="color: var(--ds-text-subtle);">
                      {t(`actions.placeholders.${item.descKey}`)}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      {/each}
    </div>
  </div>
</Modal>
