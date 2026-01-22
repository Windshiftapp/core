import {
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
  Building,
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
  Map,
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
  // Priority icons
  ArrowUp,
  ArrowDown,
  // Workspace additional icons
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
  Wrench,
  Truck,
  Volume2,
  Watch,
  Cloud,
  BarChart
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
  Map,
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
  ShoppingCart
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
  ExternalLink
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
  Map,
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
  Circle
};

/**
 * Sorted list of workspace icon names for dropdown/select options.
 */
export const workspaceIconOptions = Object.keys(workspaceIconMap).sort();
