import AlertCircle from '@lucide/svelte/icons/alert-circle';
// Alert & Status icons
import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
import Archive from '@lucide/svelte/icons/archive';
import ArrowDown from '@lucide/svelte/icons/arrow-down';
// Priority icons
import ArrowUp from '@lucide/svelte/icons/arrow-up';
import Award from '@lucide/svelte/icons/award';
import BarChart from '@lucide/svelte/icons/bar-chart';
// Workspace additional icons
import Bell from '@lucide/svelte/icons/bell';
import BookOpen from '@lucide/svelte/icons/book-open';
import Bookmark from '@lucide/svelte/icons/bookmark';
import Briefcase from '@lucide/svelte/icons/briefcase';
import Bug from '@lucide/svelte/icons/bug';
import Building from '@lucide/svelte/icons/building';
// Time & Calendar icons
import Calendar from '@lucide/svelte/icons/calendar';
import Camera from '@lucide/svelte/icons/camera';
import CheckCircle from '@lucide/svelte/icons/check-circle';
import Circle from '@lucide/svelte/icons/circle';
import Clock from '@lucide/svelte/icons/clock';
import Cloud from '@lucide/svelte/icons/cloud';
// Development icons
import Code from '@lucide/svelte/icons/code';
import Coffee from '@lucide/svelte/icons/coffee';
import Compass from '@lucide/svelte/icons/compass';
import Copy from '@lucide/svelte/icons/copy';
import Database from '@lucide/svelte/icons/database';
import Download from '@lucide/svelte/icons/download';
// Editing icons
import Edit from '@lucide/svelte/icons/edit';
import ExternalLink from '@lucide/svelte/icons/external-link';
import Eye from '@lucide/svelte/icons/eye';
import Feather from '@lucide/svelte/icons/feather';
// Common item type icons
import FileText from '@lucide/svelte/icons/file-text';
import Filter from '@lucide/svelte/icons/filter';
import Flag from '@lucide/svelte/icons/flag';
// Navigation & Location icons
import Folder from '@lucide/svelte/icons/folder';
import Gift from '@lucide/svelte/icons/gift';
import GitBranch from '@lucide/svelte/icons/git-branch';
import Globe from '@lucide/svelte/icons/globe';
import Heart from '@lucide/svelte/icons/heart';
import HelpCircle from '@lucide/svelte/icons/help-circle';
import Home from '@lucide/svelte/icons/home';
// Media icons
import Image from '@lucide/svelte/icons/image';
import Info from '@lucide/svelte/icons/info';
// Security icons
import Key from '@lucide/svelte/icons/key';
import Layers from '@lucide/svelte/icons/layers';
import Lightbulb from '@lucide/svelte/icons/lightbulb';
// Link icons
import Link from '@lucide/svelte/icons/link';
import List from '@lucide/svelte/icons/list';
import Lock from '@lucide/svelte/icons/lock';
// Communication icons
import Mail from '@lucide/svelte/icons/mail';
import MapIcon from '@lucide/svelte/icons/map';
import MapPin from '@lucide/svelte/icons/map-pin';
import Megaphone from '@lucide/svelte/icons/megaphone';
import MessageSquare from '@lucide/svelte/icons/message-square';
import Minus from '@lucide/svelte/icons/minus';
import Monitor from '@lucide/svelte/icons/monitor';
import Music from '@lucide/svelte/icons/music';
import Package from '@lucide/svelte/icons/package';
import Paperclip from '@lucide/svelte/icons/paperclip';
import Pen from '@lucide/svelte/icons/pen';
import PenTool from '@lucide/svelte/icons/pen-tool';
import Phone from '@lucide/svelte/icons/phone';
import PieChart from '@lucide/svelte/icons/pie-chart';
import Play from '@lucide/svelte/icons/play';
import Plus from '@lucide/svelte/icons/plus';
import Printer from '@lucide/svelte/icons/printer';
import RefreshCw from '@lucide/svelte/icons/refresh-cw';
import Rocket from '@lucide/svelte/icons/rocket';
import Save from '@lucide/svelte/icons/save';
import Scissors from '@lucide/svelte/icons/scissors';
// UI icons
import Search from '@lucide/svelte/icons/search';
import Send from '@lucide/svelte/icons/send';
import Server from '@lucide/svelte/icons/server';
// Project & Organization icons
import Settings from '@lucide/svelte/icons/settings';
import Shield from '@lucide/svelte/icons/shield';
import ShoppingCart from '@lucide/svelte/icons/shopping-cart';
import Smile from '@lucide/svelte/icons/smile';
import CheckSquare from '@lucide/svelte/icons/square-check';
import Star from '@lucide/svelte/icons/star';
// Misc icons
import Tag from '@lucide/svelte/icons/tag';
import Target from '@lucide/svelte/icons/target';
import Terminal from '@lucide/svelte/icons/terminal';
import TicketCheck from '@lucide/svelte/icons/ticket-check';
import Trash from '@lucide/svelte/icons/trash';
import Truck from '@lucide/svelte/icons/truck';
import Upload from '@lucide/svelte/icons/upload';
// Personal & Social icons
import User from '@lucide/svelte/icons/user';
import Users from '@lucide/svelte/icons/users';
import Video from '@lucide/svelte/icons/video';
import Volume2 from '@lucide/svelte/icons/volume-2';
import Watch from '@lucide/svelte/icons/watch';
import Wifi from '@lucide/svelte/icons/wifi';
import Wrench from '@lucide/svelte/icons/wrench';
import XCircle from '@lucide/svelte/icons/x-circle';
import Zap from '@lucide/svelte/icons/zap';

/**
 * Central icon map for work item types.
 * Used across ConfigurationSetEntityPicker, ConfigurationSetItemTypes, and ItemTypeManager.
 */
export const itemTypeIconMap = {
  // Common item type icons
  FileText,
  Bug,
  Lightbulb,
  Rocket,
  CheckSquare,
  BookOpen,
  Target,
  Zap,
  Flag,
  Star,
  Minus,

  // Alert & Status icons
  AlertTriangle,
  AlertCircle,
  Info,
  CheckCircle,
  XCircle,
  HelpCircle,
  Circle,

  // Project & Organization icons
  Settings,
  Package,
  Layers,
  GitBranch,
  Award,
  Briefcase,
  Archive,

  // Time & Calendar icons
  Calendar,
  Clock,

  // Development icons
  Code,
  Terminal,
  Database,
  Server,

  // Editing icons
  Edit,
  Eye,
  PenTool,
  Copy,
  Scissors,
  Trash,

  // Navigation & Location icons
  Folder,
  Home,
  MapPin,
  Map: MapIcon,
  Globe,

  // Media icons
  Image,
  Video,
  Music,
  Play,

  // Communication icons
  Mail,
  Phone,
  MessageSquare,
  Send,

  // Security icons
  Key,
  Lock,
  Shield,

  // Link icons
  Link,
  ExternalLink,
  Paperclip,

  // UI icons
  Search,
  Filter,
  Plus,
  Download,
  Upload,
  List,

  // Personal & Social icons
  User,
  Users,
  Heart,
  Smile,

  // Misc icons
  Tag,
  Bookmark,
  Wifi,
  PieChart,
  ShoppingCart,
};

/**
 * Sorted list of icon names for dropdown/select options.
 */
export const itemTypeIconOptions = Object.keys(itemTypeIconMap).sort();

/**
 * Central icon map for priorities.
 * Used across ConfigurationSetEntityPicker, ConfigurationSetPriorities, and PriorityManager.
 */
export const priorityIconMap = {
  // Priority-specific icons
  ArrowUp,
  ArrowDown,
  ...itemTypeIconMap,
};

/**
 * Sorted list of priority icon names for dropdown/select options.
 */
export const priorityIconOptions = Object.keys(priorityIconMap).sort();

/**
 * Central icon map for workspaces.
 * Used across MainApp, WorkspacePicker, IconSelector, CompactWorkspaceSelector, etc.
 */
export const workspaceIconMap = {
  ...itemTypeIconMap,
  // Workspace-specific additions
  Building,
  Bell,
  Camera,
  Coffee,
  Compass,
  Feather,
  Gift,
  Megaphone,
  Monitor,
  Pen,
  Printer,
  RefreshCw,
  Save,
  Search,
  Wrench,
  Truck,
  Volume2,
  Watch,
  Cloud,
  BarChart,
  TicketCheck,
};

/**
 * Sorted list of workspace icon names for dropdown/select options.
 */
export const workspaceIconOptions = Object.keys(workspaceIconMap).sort();
