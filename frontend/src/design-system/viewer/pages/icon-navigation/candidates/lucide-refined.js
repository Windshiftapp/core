import Activity from '@lucide/svelte/icons/activity';
import BadgeCheck from '@lucide/svelte/icons/badge-check';
import Bell from '@lucide/svelte/icons/bell';
import Blocks from '@lucide/svelte/icons/blocks';
import Boxes from '@lucide/svelte/icons/boxes';
import Building2 from '@lucide/svelte/icons/building-2';
import CalendarRange from '@lucide/svelte/icons/calendar-range';
import ChartNoAxesGantt from '@lucide/svelte/icons/chart-no-axes-gantt';
import CircleDotDashed from '@lucide/svelte/icons/circle-dot-dashed';
import DatabaseZap from '@lucide/svelte/icons/database-zap';
import FileStack from '@lucide/svelte/icons/file-stack';
import GitPullRequestArrow from '@lucide/svelte/icons/git-pull-request-arrow';
import Goal from '@lucide/svelte/icons/goal';
import LibraryBig from '@lucide/svelte/icons/library-big';
import Link2 from '@lucide/svelte/icons/link-2';
import Mail from '@lucide/svelte/icons/mail';
import MessagesSquare from '@lucide/svelte/icons/messages-square';
import Network from '@lucide/svelte/icons/network';
import PackageOpen from '@lucide/svelte/icons/package-open';
import PanelsTopLeft from '@lucide/svelte/icons/panels-top-left';
import Paperclip from '@lucide/svelte/icons/paperclip';
import Plus from '@lucide/svelte/icons/plus';
import RadioTower from '@lucide/svelte/icons/radio-tower';
import Search from '@lucide/svelte/icons/search';
import Settings2 from '@lucide/svelte/icons/settings-2';
import Shapes from '@lucide/svelte/icons/shapes';
import ShieldCheck from '@lucide/svelte/icons/shield-check';
import SignalHigh from '@lucide/svelte/icons/signal-high';
import Sparkles from '@lucide/svelte/icons/sparkles';
import SwatchBook from '@lucide/svelte/icons/swatch-book';
import UsersRound from '@lucide/svelte/icons/users-round';
import Workflow from '@lucide/svelte/icons/workflow';

export default {
  id: 'lucide-refined',
  label: 'Lucide refined',
  license: 'ISC',
  mode: 'lucide-refined',
  refined: true,
  description:
    'Curated Lucide metaphors with optical sizing, lighter resting strokes, and stronger selected-state containers.',
  mainSize: 18,
  adminSize: 15,
  icons: {
    workspace: PanelsTopLeft,
    collections: Boxes,
    time: ChartNoAxesGantt,
    milestones: Goal,
    iterations: CalendarRange,
    knowledge: LibraryBig,
    assets: PackageOpen,
    channels: RadioTower,
    portal: MessagesSquare,
    organizations: Building2,
    teams: UsersRound,
    create: Plus,
    search: Search,
    settings: Settings2,
    notifications: Bell,
    customFields: DatabaseZap,
    screens: Shapes,
    hierarchy: Network,
    itemTypes: FileStack,
    priorities: SignalHigh,
    statuses: CircleDotDashed,
    workflows: Workflow,
    approvals: BadgeCheck,
    ai: Sparkles,
    scm: GitPullRequestArrow,
    integrations: Blocks,
    links: Link2,
    attachments: Paperclip,
    themes: SwatchBook,
    mail: Mail,
    security: ShieldCheck,
    activity: Activity,
  },
};
