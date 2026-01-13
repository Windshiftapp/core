<script>
  import { Sun, Moon, Palette, Type, Ruler, Square, TextCursor, AlertCircle, List, AlignLeft, FormInput, Search, Loader, MessageCircle, Tag, Circle, CircleDot, Inbox, PanelTop, LayoutPanelTop, Columns3, PanelLeft, Navigation, Link2, ChevronsLeftRight, Layers, ChevronDown, Menu, Hash, MousePointer, Sliders, Building, Users, Shield, Folder, AtSign, Flag, Grid, FileText, HelpCircle, BarChart3, BadgeCheck, Disc, SectionIcon, MousePointer2, RefreshCw, XCircle } from 'lucide-svelte'

  // Token Pages
  import Colors from './pages/Colors.svelte'
  import Typography from './pages/Typography.svelte'
  import Spacing from './pages/Spacing.svelte'

  // Form Component Pages
  import ButtonPage from './pages/Button.svelte'
  import InputPage from './pages/Input.svelte'
  import SelectPage from './pages/Select.svelte'
  import TextareaPage from './pages/Textarea.svelte'
  import FormFieldPage from './pages/FormField.svelte'
  import SearchInputPage from './pages/SearchInput.svelte'
  import TogglePage from './pages/Toggle.svelte'
  import LabelPage from './pages/Label.svelte'
  import HelperTextPage from './pages/HelperText.svelte'
  import FormSectionTitlePage from './pages/FormSectionTitle.svelte'
  import ActionButtonPage from './pages/ActionButton.svelte'

  // Feedback Component Pages
  import AlertBoxPage from './pages/AlertBox.svelte'
  import SpinnerPage from './pages/Spinner.svelte'
  import ToastPage from './pages/Toast.svelte'
  import TooltipPage from './pages/Tooltip.svelte'
  import SkeletonPage from './pages/Skeleton.svelte'
  import ProgressPage from './pages/Progress.svelte'
  import LoadingStatePage from './pages/LoadingState.svelte'
  import ErrorStatePage from './pages/ErrorState.svelte'

  // Badge Component Pages
  import LozengePage from './pages/Lozenge.svelte'
  import BadgeCounterPage from './pages/BadgeCounter.svelte'
  import ColorDotPage from './pages/ColorDot.svelte'
  import BadgePage from './pages/Badge.svelte'
  import ChipPage from './pages/Chip.svelte'

  // Layout Component Pages
  import EmptyStatePage from './pages/EmptyState.svelte'
  import SectionHeaderPage from './pages/SectionHeader.svelte'
  import CardHeaderPage from './pages/CardHeader.svelte'
  import PageHeaderPage from './pages/PageHeader.svelte'

  // Sidebar Component Page
  import SidebarPage from './pages/Sidebar.svelte'

  // Navigation Component Pages
  import TabsPage from './pages/Tabs.svelte'
  import LinkPage from './pages/Link.svelte'
  import PaginationPage from './pages/Pagination.svelte'

  // Modal Component Pages
  import ModalPage from './pages/Modal.svelte'
  import FormDialogPage from './pages/FormDialog.svelte'

  // Menu Component Page
  import DropdownMenuPage from './pages/DropdownMenu.svelte'

  // Data Display Page
  import ItemKeyPage from './pages/ItemKey.svelte'

  // Picker Component Pages
  import ItemPickerPage from './pages/ItemPickerPage.svelte'
  import InlineSelectEditorPage from './pages/InlineSelectEditorPage.svelte'
  import IconSelectorPage from './pages/IconSelectorPage.svelte'
  import WorkspaceSelectorPage from './pages/WorkspaceSelectorPage.svelte'
  import ComboboxPickersPage from './pages/ComboboxPickersPage.svelte'
  import CategoryMultiSelectPage from './pages/CategoryMultiSelectPage.svelte'
  import FieldSelectorPage from './pages/FieldSelectorPage.svelte'
  import MentionPickerPage from './pages/MentionPickerPage.svelte'

  let currentPage = $state('colors')
  let colorMode = $state('light')

  function toggleColorMode() {
    colorMode = colorMode === 'light' ? 'dark' : 'light'
    document.documentElement.setAttribute('data-color-mode', colorMode)
  }

  const navGroups = [
    {
      title: 'Design Tokens',
      items: [
        { id: 'colors', label: 'Colors', icon: Palette },
        { id: 'typography', label: 'Typography', icon: Type },
        { id: 'spacing', label: 'Spacing', icon: Ruler },
      ]
    },
    {
      title: 'Form Components',
      items: [
        { id: 'button', label: 'Button', icon: Square },
        { id: 'input', label: 'Input', icon: TextCursor },
        { id: 'select', label: 'Select', icon: List },
        { id: 'textarea', label: 'Textarea', icon: AlignLeft },
        { id: 'formfield', label: 'FormField', icon: FormInput },
        { id: 'searchinput', label: 'SearchInput', icon: Search },
        { id: 'toggle', label: 'Toggle', icon: Sliders },
        { id: 'label', label: 'Label', icon: FileText },
        { id: 'helpertext', label: 'HelperText', icon: HelpCircle },
        { id: 'formsectiontitle', label: 'FormSectionTitle', icon: SectionIcon },
        { id: 'actionbutton', label: 'ActionButton', icon: MousePointer2 },
      ]
    },
    {
      title: 'Feedback',
      items: [
        { id: 'alertbox', label: 'AlertBox', icon: AlertCircle },
        { id: 'spinner', label: 'Spinner', icon: Loader },
        { id: 'toast', label: 'Toast', icon: MessageCircle },
        { id: 'tooltip', label: 'Tooltip', icon: MessageCircle },
        { id: 'skeleton', label: 'Skeleton', icon: Loader },
        { id: 'progress', label: 'Progress', icon: BarChart3 },
        { id: 'loadingstate', label: 'LoadingState', icon: RefreshCw },
        { id: 'errorstate', label: 'ErrorState', icon: XCircle },
      ]
    },
    {
      title: 'Badges',
      items: [
        { id: 'lozenge', label: 'Lozenge', icon: Tag },
        { id: 'badgecounter', label: 'BadgeCounter', icon: Circle },
        { id: 'colordot', label: 'ColorDot', icon: CircleDot },
        { id: 'badge', label: 'Badge', icon: BadgeCheck },
        { id: 'chip', label: 'Chip', icon: Disc },
      ]
    },
    {
      title: 'Layout',
      items: [
        { id: 'emptystate', label: 'EmptyState', icon: Inbox },
        { id: 'sectionheader', label: 'SectionHeader', icon: PanelTop },
        { id: 'cardheader', label: 'CardHeader', icon: LayoutPanelTop },
        { id: 'pageheader', label: 'PageHeader', icon: Columns3 },
      ]
    },
    {
      title: 'Sidebar',
      items: [
        { id: 'sidebar', label: 'Sidebar Components', icon: PanelLeft },
      ]
    },
    {
      title: 'Navigation',
      items: [
        { id: 'tabs', label: 'Tabs', icon: Navigation },
        { id: 'link', label: 'Link', icon: Link2 },
        { id: 'pagination', label: 'Pagination', icon: ChevronsLeftRight },
      ]
    },
    {
      title: 'Modals',
      items: [
        { id: 'modal', label: 'Modal', icon: Layers },
        { id: 'formdialog', label: 'FormDialog', icon: Layers },
      ]
    },
    {
      title: 'Menus',
      items: [
        { id: 'dropdownmenu', label: 'DropdownMenu', icon: ChevronDown },
      ]
    },
    {
      title: 'Data Display',
      items: [
        { id: 'itemkey', label: 'ItemKey', icon: Hash },
      ]
    },
    {
      title: 'Pickers & Comboboxes',
      items: [
        { id: 'itempicker', label: 'ItemPicker', icon: MousePointer },
        { id: 'inlineselecteditor', label: 'InlineSelectEditor', icon: Sliders },
        { id: 'iconselector', label: 'IconSelector', icon: Grid },
        { id: 'workspaceselector', label: 'WorkspaceSelector', icon: Building },
        { id: 'comboboxpickers', label: 'Combobox Pickers', icon: Users },
        { id: 'categorymultiselect', label: 'CategoryMultiSelect', icon: Tag },
        { id: 'fieldselector', label: 'FieldSelector', icon: List },
        { id: 'mentionpicker', label: 'MentionPicker', icon: AtSign },
      ]
    },
  ]
</script>

<div class="flex h-screen" style="background-color: var(--ds-surface);">
  <!-- Sidebar -->
  <aside
    class="w-64 flex-shrink-0 flex flex-col border-r"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Header -->
    <div class="p-4 border-b" style="border-color: var(--ds-border);">
      <h1 class="text-lg font-semibold" style="color: var(--ds-text);">
        Windshift Design System
      </h1>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 overflow-y-auto p-3">
      {#each navGroups as group}
        <div class="mb-4">
          <h2
            class="text-xs font-semibold uppercase tracking-wide px-3 py-2"
            style="color: var(--ds-text-subtle);"
          >
            {group.title}
          </h2>
          <ul class="space-y-1">
            {#each group.items as item}
              <li>
                <button
                  onclick={() => currentPage = item.id}
                  class="w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors"
                  style={currentPage === item.id
                    ? 'background-color: var(--ds-surface-selected); color: var(--ds-text);'
                    : 'color: var(--ds-text-subtle);'}
                >
                  <svelte:component this={item.icon} class="w-4 h-4" />
                  {item.label}
                </button>
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    </nav>

    <!-- Theme Toggle -->
    <div class="p-3 border-t" style="border-color: var(--ds-border);">
      <button
        onclick={toggleColorMode}
        class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm transition-colors"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text);"
      >
        {#if colorMode === 'light'}
          <Moon class="w-4 h-4" />
          Switch to Dark
        {:else}
          <Sun class="w-4 h-4" />
          Switch to Light
        {/if}
      </button>
    </div>
  </aside>

  <!-- Main Content -->
  <main class="flex-1 overflow-y-auto">
    {#if currentPage === 'colors'}
      <Colors />
    {:else if currentPage === 'typography'}
      <Typography />
    {:else if currentPage === 'spacing'}
      <Spacing />
    {:else if currentPage === 'button'}
      <ButtonPage />
    {:else if currentPage === 'input'}
      <InputPage />
    {:else if currentPage === 'select'}
      <SelectPage />
    {:else if currentPage === 'textarea'}
      <TextareaPage />
    {:else if currentPage === 'formfield'}
      <FormFieldPage />
    {:else if currentPage === 'searchinput'}
      <SearchInputPage />
    {:else if currentPage === 'toggle'}
      <TogglePage />
    {:else if currentPage === 'label'}
      <LabelPage />
    {:else if currentPage === 'helpertext'}
      <HelperTextPage />
    {:else if currentPage === 'formsectiontitle'}
      <FormSectionTitlePage />
    {:else if currentPage === 'actionbutton'}
      <ActionButtonPage />
    {:else if currentPage === 'alertbox'}
      <AlertBoxPage />
    {:else if currentPage === 'spinner'}
      <SpinnerPage />
    {:else if currentPage === 'toast'}
      <ToastPage />
    {:else if currentPage === 'tooltip'}
      <TooltipPage />
    {:else if currentPage === 'skeleton'}
      <SkeletonPage />
    {:else if currentPage === 'progress'}
      <ProgressPage />
    {:else if currentPage === 'loadingstate'}
      <LoadingStatePage />
    {:else if currentPage === 'errorstate'}
      <ErrorStatePage />
    {:else if currentPage === 'lozenge'}
      <LozengePage />
    {:else if currentPage === 'badgecounter'}
      <BadgeCounterPage />
    {:else if currentPage === 'colordot'}
      <ColorDotPage />
    {:else if currentPage === 'badge'}
      <BadgePage />
    {:else if currentPage === 'chip'}
      <ChipPage />
    {:else if currentPage === 'emptystate'}
      <EmptyStatePage />
    {:else if currentPage === 'sectionheader'}
      <SectionHeaderPage />
    {:else if currentPage === 'cardheader'}
      <CardHeaderPage />
    {:else if currentPage === 'pageheader'}
      <PageHeaderPage />
    {:else if currentPage === 'sidebar'}
      <SidebarPage />
    {:else if currentPage === 'tabs'}
      <TabsPage />
    {:else if currentPage === 'link'}
      <LinkPage />
    {:else if currentPage === 'pagination'}
      <PaginationPage />
    {:else if currentPage === 'modal'}
      <ModalPage />
    {:else if currentPage === 'formdialog'}
      <FormDialogPage />
    {:else if currentPage === 'dropdownmenu'}
      <DropdownMenuPage />
    {:else if currentPage === 'itemkey'}
      <ItemKeyPage />
    {:else if currentPage === 'itempicker'}
      <ItemPickerPage />
    {:else if currentPage === 'inlineselecteditor'}
      <InlineSelectEditorPage />
    {:else if currentPage === 'iconselector'}
      <IconSelectorPage />
    {:else if currentPage === 'workspaceselector'}
      <WorkspaceSelectorPage />
    {:else if currentPage === 'comboboxpickers'}
      <ComboboxPickersPage />
    {:else if currentPage === 'categorymultiselect'}
      <CategoryMultiSelectPage />
    {:else if currentPage === 'fieldselector'}
      <FieldSelectorPage />
    {:else if currentPage === 'mentionpicker'}
      <MentionPickerPage />
    {/if}
  </main>
</div>
