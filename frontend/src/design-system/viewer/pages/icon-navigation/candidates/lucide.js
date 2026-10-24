import Activity from '@lucide/svelte/icons/activity';
import BadgeCheck from '@lucide/svelte/icons/badge-check';
import Bell from '@lucide/svelte/icons/bell';
import BookOpen from '@lucide/svelte/icons/book-open';
import BrainCircuit from '@lucide/svelte/icons/brain-circuit';
import CalendarDays from '@lucide/svelte/icons/calendar-days';
import Clock3 from '@lucide/svelte/icons/clock-3';
import Database from '@lucide/svelte/icons/database';
import FileText from '@lucide/svelte/icons/file-text';
import Flag from '@lucide/svelte/icons/flag';
import Folder from '@lucide/svelte/icons/folder';
import GitBranch from '@lucide/svelte/icons/git-branch';
import GitFork from '@lucide/svelte/icons/git-fork';
import GitMerge from '@lucide/svelte/icons/git-merge';
import Headphones from '@lucide/svelte/icons/headphones';
import LayoutGrid from '@lucide/svelte/icons/layout-grid';
import Link from '@lucide/svelte/icons/link';
import ListTree from '@lucide/svelte/icons/list-tree';
import Mail from '@lucide/svelte/icons/mail';
import MessageCircle from '@lucide/svelte/icons/message-circle';
import Monitor from '@lucide/svelte/icons/monitor';
import Package from '@lucide/svelte/icons/package';
import Paintbrush from '@lucide/svelte/icons/paintbrush';
import Paperclip from '@lucide/svelte/icons/paperclip';
import Phone from '@lucide/svelte/icons/phone';
import Plug from '@lucide/svelte/icons/plug';
import Plus from '@lucide/svelte/icons/plus';
import Search from '@lucide/svelte/icons/search';
import Settings from '@lucide/svelte/icons/settings';
import Shield from '@lucide/svelte/icons/shield';
import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
import Users from '@lucide/svelte/icons/users';

export default {
  id: 'lucide',
  label: 'Lucide baseline',
  license: 'ISC',
  mode: 'lucide',
  description:
    'Untuned Lucide glyphs at their default 2px stroke, providing a control for the comparison.',
  mainSize: 20,
  adminSize: 16,
  icons: {
    workspace: LayoutGrid,
    collections: Folder,
    time: Clock3,
    milestones: Flag,
    iterations: CalendarDays,
    knowledge: BookOpen,
    assets: Package,
    channels: Headphones,
    portal: MessageCircle,
    organizations: Users,
    teams: Phone,
    create: Plus,
    search: Search,
    settings: Settings,
    notifications: Bell,
    customFields: Database,
    screens: Monitor,
    hierarchy: ListTree,
    itemTypes: FileText,
    priorities: TriangleAlert,
    statuses: GitBranch,
    workflows: GitFork,
    approvals: BadgeCheck,
    ai: BrainCircuit,
    scm: GitMerge,
    integrations: Plug,
    links: Link,
    attachments: Paperclip,
    themes: Paintbrush,
    mail: Mail,
    security: Shield,
    activity: Activity,
  },
};
