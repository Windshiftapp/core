import {
  AlertCircle,
  // Alert & Status icons
  AlertTriangle,
  Archive,
  ArrowDown,
  // Priority icons
  ArrowUp,
  Award,
  BarChart,
  // Workspace additional icons
  Bell,
  Bookmark,
  BookOpen,
  Briefcase,
  Bug,
  Building,
  // Time & Calendar icons
  Calendar,
  Camera,
  CheckCircle,
  CheckSquare,
  Circle,
  Clock,
  Cloud,
  // Development icons
  Code,
  Coffee,
  Compass,
  Copy,
  Database,
  Download,
  // Editing icons
  Edit,
  ExternalLink,
  Eye,
  Feather,
  // Common item type icons
  FileText,
  Filter,
  Flag,
  // Navigation & Location icons
  Folder,
  Gift,
  GitBranch,
  Globe,
  Heart,
  HelpCircle,
  Home,
  // Media icons
  Image,
  Info,
  // Security icons
  Key,
  Layers,
  Lightbulb,
  // Link icons
  Link,
  List,
  Lock,
  // Communication icons
  Mail,
  Map as MapIcon,
  MapPin,
  Megaphone,
  MessageSquare,
  Minus,
  Monitor,
  Music,
  Package,
  Paperclip,
  Pen,
  PenTool,
  Phone,
  PieChart,
  Play,
  Plus,
  Printer,
  RefreshCw,
  Rocket,
  Save,
  Scissors,
  // UI icons
  Search,
  Send,
  Server,
  // Project & Organization icons
  Settings,
  Shield,
  ShoppingCart,
  Smile,
  Star,
  // Misc icons
  Tag,
  Target,
  Terminal,
  Trash,
  Truck,
  Upload,
  // Personal & Social icons
  User,
  Users,
  Video,
  Volume2,
  Watch,
  Wifi,
  Wrench,
  XCircle,
  Zap,
} from 'lucide-svelte';

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
  AlertCircle,
  ArrowUp,
  ArrowDown,
  Minus,

  // Common icons (same as itemTypeIconMap)
  Target,
  Zap,
  BookOpen,
  CheckSquare,
  Bug,
  Star,
  Flag,
  Lightbulb,
  Settings,
  User,
  Users,
  Calendar,
  Clock,
  MapPin,
  Search,
  Filter,
  Tag,
  Bookmark,
  Heart,
  Shield,
  Key,
  Lock,
  Globe,
  Wifi,
  Database,
  Server,
  Code,
  Terminal,
  FileText,
  Folder,
  Image,
  Video,
  Music,
  Download,
  Upload,
  Send,
  Mail,
  Phone,
  MessageSquare,
  Info,
  CheckCircle,
  XCircle,
  HelpCircle,
  Archive,
  Trash,
  Edit,
  Copy,
  Scissors,
  Paperclip,
  Link,
  ExternalLink,
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
  // Core workspace icons
  Target,
  Zap,
  BookOpen,
  CheckSquare,
  Bug,
  Minus,
  Star,
  Flag,
  Lightbulb,
  Settings,
  User,
  Users,
  Calendar,
  Clock,
  MapPin,
  Search,
  Filter,
  Tag,
  Bookmark,
  Heart,
  Shield,
  Key,
  Lock,
  Globe,
  Wifi,
  Database,
  Server,
  Code,
  Terminal,
  FileText,
  Folder,
  Image,
  Video,
  Music,
  Download,
  Upload,
  Send,
  Mail,
  Phone,
  MessageSquare,
  AlertCircle,
  Info,
  CheckCircle,
  XCircle,
  HelpCircle,
  Archive,
  Trash,
  Edit,
  Copy,
  Scissors,
  Paperclip,
  Link,
  ExternalLink,
  Package,
  Building,
  // Additional icons from IconSelector
  Rocket,
  Award,
  Bell,
  Camera,
  Coffee,
  Compass,
  Feather,
  Gift,
  Home,
  Layers,
  Map: MapIcon,
  Megaphone,
  Monitor,
  Pen,
  Printer,
  RefreshCw,
  Save,
  Smile,
  Wrench,
  Truck,
  Volume2,
  Watch,
  Briefcase,
  Cloud,
  BarChart,
  Circle,
};

/**
 * Sorted list of workspace icon names for dropdown/select options.
 */
export const workspaceIconOptions = Object.keys(workspaceIconMap).sort();
