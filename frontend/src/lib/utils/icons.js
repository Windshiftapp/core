import * as icons from 'lucide-svelte';

/**
 * Central icon map for work item types.
 * Used across ConfigurationSetEntityPicker, ConfigurationSetItemTypes, and ItemTypeManager.
 */
export const itemTypeIconMap = {
  // Common item type icons
  FileText: icons.FileText,
  Bug: icons.Bug,
  Lightbulb: icons.Lightbulb,
  Rocket: icons.Rocket,
  CheckSquare: icons.CheckSquare,
  BookOpen: icons.BookOpen,
  Target: icons.Target,
  Zap: icons.Zap,
  Flag: icons.Flag,
  Star: icons.Star,
  Minus: icons.Minus,

  // Alert & Status icons
  AlertTriangle: icons.AlertTriangle,
  AlertCircle: icons.AlertCircle,
  Info: icons.Info,
  CheckCircle: icons.CheckCircle,
  XCircle: icons.XCircle,
  HelpCircle: icons.HelpCircle,
  Circle: icons.Circle,

  // Project & Organization icons
  Settings: icons.Settings,
  Package: icons.Package,
  Layers: icons.Layers,
  GitBranch: icons.GitBranch,
  Award: icons.Award,
  Briefcase: icons.Briefcase,
  Archive: icons.Archive,

  // Time & Calendar icons
  Calendar: icons.Calendar,
  Clock: icons.Clock,

  // Development icons
  Code: icons.Code,
  Terminal: icons.Terminal,
  Database: icons.Database,
  Server: icons.Server,

  // Editing icons
  Edit: icons.Edit,
  Eye: icons.Eye,
  PenTool: icons.PenTool,
  Copy: icons.Copy,
  Scissors: icons.Scissors,
  Trash: icons.Trash,

  // Navigation & Location icons
  Folder: icons.Folder,
  Home: icons.Home,
  MapPin: icons.MapPin,
  Map: icons.Map,
  Globe: icons.Globe,

  // Media icons
  Image: icons.Image,
  Video: icons.Video,
  Music: icons.Music,
  Play: icons.Play,

  // Communication icons
  Mail: icons.Mail,
  Phone: icons.Phone,
  MessageSquare: icons.MessageSquare,
  Send: icons.Send,

  // Security icons
  Key: icons.Key,
  Lock: icons.Lock,
  Shield: icons.Shield,

  // Link icons
  Link: icons.Link,
  ExternalLink: icons.ExternalLink,
  Paperclip: icons.Paperclip,

  // UI icons
  Search: icons.Search,
  Filter: icons.Filter,
  Plus: icons.Plus,
  Download: icons.Download,
  Upload: icons.Upload,
  List: icons.List,

  // Personal & Social icons
  User: icons.User,
  Users: icons.Users,
  Heart: icons.Heart,
  Smile: icons.Smile,

  // Misc icons
  Tag: icons.Tag,
  Bookmark: icons.Bookmark,
  Wifi: icons.Wifi,
  PieChart: icons.PieChart,
  ShoppingCart: icons.ShoppingCart
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
  AlertCircle: icons.AlertCircle,
  ArrowUp: icons.ArrowUp,
  ArrowDown: icons.ArrowDown,
  Minus: icons.Minus,

  // Common icons (same as itemTypeIconMap)
  Target: icons.Target,
  Zap: icons.Zap,
  BookOpen: icons.BookOpen,
  CheckSquare: icons.CheckSquare,
  Bug: icons.Bug,
  Star: icons.Star,
  Flag: icons.Flag,
  Lightbulb: icons.Lightbulb,
  Settings: icons.Settings,
  User: icons.User,
  Users: icons.Users,
  Calendar: icons.Calendar,
  Clock: icons.Clock,
  MapPin: icons.MapPin,
  Search: icons.Search,
  Filter: icons.Filter,
  Tag: icons.Tag,
  Bookmark: icons.Bookmark,
  Heart: icons.Heart,
  Shield: icons.Shield,
  Key: icons.Key,
  Lock: icons.Lock,
  Globe: icons.Globe,
  Wifi: icons.Wifi,
  Database: icons.Database,
  Server: icons.Server,
  Code: icons.Code,
  Terminal: icons.Terminal,
  FileText: icons.FileText,
  Folder: icons.Folder,
  Image: icons.Image,
  Video: icons.Video,
  Music: icons.Music,
  Download: icons.Download,
  Upload: icons.Upload,
  Send: icons.Send,
  Mail: icons.Mail,
  Phone: icons.Phone,
  MessageSquare: icons.MessageSquare,
  Info: icons.Info,
  CheckCircle: icons.CheckCircle,
  XCircle: icons.XCircle,
  HelpCircle: icons.HelpCircle,
  Archive: icons.Archive,
  Trash: icons.Trash,
  Edit: icons.Edit,
  Copy: icons.Copy,
  Scissors: icons.Scissors,
  Paperclip: icons.Paperclip,
  Link: icons.Link,
  ExternalLink: icons.ExternalLink
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
  Target: icons.Target,
  Zap: icons.Zap,
  BookOpen: icons.BookOpen,
  CheckSquare: icons.CheckSquare,
  Bug: icons.Bug,
  Minus: icons.Minus,
  Star: icons.Star,
  Flag: icons.Flag,
  Lightbulb: icons.Lightbulb,
  Settings: icons.Settings,
  User: icons.User,
  Users: icons.Users,
  Calendar: icons.Calendar,
  Clock: icons.Clock,
  MapPin: icons.MapPin,
  Search: icons.Search,
  Filter: icons.Filter,
  Tag: icons.Tag,
  Bookmark: icons.Bookmark,
  Heart: icons.Heart,
  Shield: icons.Shield,
  Key: icons.Key,
  Lock: icons.Lock,
  Globe: icons.Globe,
  Wifi: icons.Wifi,
  Database: icons.Database,
  Server: icons.Server,
  Code: icons.Code,
  Terminal: icons.Terminal,
  FileText: icons.FileText,
  Folder: icons.Folder,
  Image: icons.Image,
  Video: icons.Video,
  Music: icons.Music,
  Download: icons.Download,
  Upload: icons.Upload,
  Send: icons.Send,
  Mail: icons.Mail,
  Phone: icons.Phone,
  MessageSquare: icons.MessageSquare,
  AlertCircle: icons.AlertCircle,
  Info: icons.Info,
  CheckCircle: icons.CheckCircle,
  XCircle: icons.XCircle,
  HelpCircle: icons.HelpCircle,
  Archive: icons.Archive,
  Trash: icons.Trash,
  Edit: icons.Edit,
  Copy: icons.Copy,
  Scissors: icons.Scissors,
  Paperclip: icons.Paperclip,
  Link: icons.Link,
  ExternalLink: icons.ExternalLink,
  Package: icons.Package,
  Building: icons.Building,
  // Additional icons from IconSelector
  Rocket: icons.Rocket,
  Award: icons.Award,
  Bell: icons.Bell,
  Camera: icons.Camera,
  Coffee: icons.Coffee,
  Compass: icons.Compass,
  Feather: icons.Feather,
  Gift: icons.Gift,
  Home: icons.Home,
  Layers: icons.Layers,
  Map: icons.Map,
  Megaphone: icons.Megaphone,
  Monitor: icons.Monitor,
  Pen: icons.Pen,
  Printer: icons.Printer,
  RefreshCw: icons.RefreshCw,
  Save: icons.Save,
  Smile: icons.Smile,
  Wrench: icons.Wrench,
  Truck: icons.Truck,
  Volume2: icons.Volume2,
  Watch: icons.Watch,
  Briefcase: icons.Briefcase,
  Cloud: icons.Cloud,
  BarChart: icons.BarChart,
  Circle: icons.Circle
};

/**
 * Sorted list of workspace icon names for dropdown/select options.
 */
export const workspaceIconOptions = Object.keys(workspaceIconMap).sort();
