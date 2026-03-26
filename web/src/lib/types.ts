// Application status & features

export interface Features {
  videos_available: boolean;
  analytics_available: boolean;
  chimes_available: boolean;
  shows_available: boolean;
  wraps_available: boolean;
  music_available: boolean;
}

export type ModeToken = "present" | "edit" | "unknown";

export interface ModeInfo {
  mode: ModeToken;
  mode_label: string;
  mode_class: string;
  share_paths?: Record<string, string>;
}

export interface AppStatus {
  mode: ModeToken;
  mode_label: string;
  hostname: string;
  features: Features;
}

// Videos

export interface VideoFolder {
  name: string;
  path: string;
  count: number;
}

export interface VideoEvent {
  name: string;
  datetime: string;
  city: string;
  reason: string;
  size_mb: number;
  has_thumbnail: boolean;
  camera_videos: Record<string, string>;
  encrypted_videos: Record<string, boolean>;
  clips?: string[];
}

export interface SessionGroup {
  session: string;
  cameras: string[];
  timestamp: string;
}

export interface VideoListResponse {
  folders: VideoFolder[];
  teslacam_path: string;
}

export interface VideoEventsResponse {
  events: VideoEvent[];
  page: number;
  per_page: number;
  has_next: boolean;
}

export interface VideoSessionsResponse {
  sessions: SessionGroup[];
  page: number;
  per_page: number;
  has_next: boolean;
}

// Chimes

export interface ChimeListResponse {
  chimes: string[];
  active: string;
  active_exists: boolean;
  random_mode: RandomConfig;
}

export interface ChimeFilenamesResponse {
  filenames: string[];
}

export type ScheduleType = "weekly" | "date" | "holiday" | "recurring";

export interface Schedule {
  id: string;
  chime_filename: string;
  time: string;
  type: ScheduleType;
  days?: number[];
  month?: number;
  day?: number;
  holiday?: string;
  interval?: string;
  name?: string;
  enabled: boolean;
  last_run?: string;
}

export interface ChimeGroup {
  id: string;
  name: string;
  description?: string;
  chimes: string[];
}

export interface RandomConfig {
  enabled: boolean;
  group_id: string;
  last_used?: string;
}

export interface ChimeGroupsResponse {
  groups: ChimeGroup[];
}

// Light Shows

export interface LightShow {
  base_name: string;
  fseq_file?: string;
  audio_file?: string;
  partition_key: string;
}

export interface LightShowListResponse {
  shows: LightShow[];
}

// Wraps

export interface WrapFile {
  filename: string;
  width: number;
  height: number;
  size: number;
  size_str: string;
  partition_key: string;
}

export interface WrapListResponse {
  wraps: WrapFile[];
  max_count: number;
  max_size: number;
}

// Music

export interface FileInfo {
  name: string;
  path: string;
  size: number;
}

export interface DirInfo {
  name: string;
  path: string;
}

export interface MusicListResult {
  current_path: string;
  dirs: DirInfo[];
  files: FileInfo[];
  used_bytes: number;
  free_bytes: number;
  total_bytes: number;
}

export interface ChunkUploadResponse {
  status: string;
  upload_id: string;
  complete: boolean;
}

// Analytics

export interface PartitionUsage {
  name: string;
  label: string;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  percent_used: number;
}

export interface VideoStats {
  folder: string;
  count: number;
  size_bytes: number;
}

export interface HealthStatus {
  status: "healthy" | "caution" | "warning" | "critical";
  score: number;
  alerts: string[];
  recommendations: string[];
}

export interface FolderBreakdown {
  name: string;
  count: number;
  size_mb: number;
  priority: "high" | "medium" | "low";
}

export interface CompleteAnalytics {
  partition_usage: PartitionUsage[];
  video_statistics: VideoStats[];
  storage_health: HealthStatus;
  recording_estimate: Record<string, number>;
  folder_breakdown: FolderBreakdown[];
  last_updated: string;
}

// Cleanup

export interface AgePolicy {
  enabled: boolean;
  max_days: number;
}

export interface SizePolicy {
  enabled: boolean;
  max_gb: number;
}

export interface CountPolicy {
  enabled: boolean;
  max_count: number;
}

export interface CleanupPolicy {
  enabled: boolean;
  boot_cleanup: boolean;
  age_based?: AgePolicy;
  size_based?: SizePolicy;
  count_based?: CountPolicy;
}

export interface FileToDelete {
  path: string;
  name: string;
  size: number;
  modified: string;
  reason: string;
}

export interface CleanupPlan {
  total_count: number;
  total_size: number;
  breakdown_by_folder: Record<string, FileToDelete[]>;
}

export interface CleanupReport {
  dry_run: boolean;
  deleted_count: number;
  deleted_size_gb: number;
  errors?: string[];
}

export interface CleanupSettingsResponse {
  policies: Record<string, CleanupPolicy>;
}

export interface CleanupPreviewResponse {
  plan: CleanupPlan;
  report: CleanupReport;
}

// Fsck

export type FsckStatusValue = "idle" | "running" | "done" | "failed";

export interface FsckCheckResult {
  partition: string;
  started_at: string;
  finished_at?: string;
  status: FsckStatusValue;
  output?: string;
  exit_code: number;
  error?: string;
}

export interface FsckStatus {
  running: boolean;
  partition?: string;
  started_at?: string;
  results?: FsckCheckResult[];
}

export interface FsckHistoryResponse {
  history: FsckCheckResult[];
}

// Access Point

export interface APStatus {
  enabled: boolean;
  active: boolean;
  ssid: string;
  interface: string;
  force_mode: string;
  channel: number;
  client_count: number;
}

export interface APConfig {
  ssid: string;
  passphrase: string;
  channel: number;
  interface: string;
  ipv4_cidr: string;
  dhcp_start: string;
  dhcp_end: string;
  check_interval: number;
  disconnect_grace: number;
}

// WiFi

export interface WifiConnection {
  connected: boolean;
  ssid?: string;
  signal?: number;
  frequency?: string;
  ip?: string;
}

export interface WifiNetwork {
  ssid: string;
  signal: number;
  frequency: string;
  security: string;
  in_use: boolean;
}

export interface WifiChangeStatus {
  pending: boolean;
  message?: string;
  timestamp?: string;
}

export interface WifiStatus {
  connection: WifiConnection;
  change_status: WifiChangeStatus;
}

export interface WifiScanResponse {
  networks: WifiNetwork[];
}

// Telegram

export interface TelegramStatus {
  enabled: boolean;
  queue_size: number;
  max_queue: number;
  online: boolean;
  bot_configured: boolean;
}

// Camera types

export type CameraName =
  | "front"
  | "back"
  | "left_repeater"
  | "right_repeater"
  | "left_pillar"
  | "right_pillar";

export const CAMERA_LABELS: Record<CameraName, string> = {
  front: "Front",
  back: "Back",
  left_repeater: "Left Repeater",
  right_repeater: "Right Repeater",
  left_pillar: "Left Pillar",
  right_pillar: "Right Pillar",
};

// Generic API responses

export interface StatusResponse {
  status: string;
  [key: string]: unknown;
}

export interface ErrorResponse {
  error: string;
}

// Gadget

export interface GadgetState {
  mode: ModeToken;
  gadget_present: boolean;
  lun0_file?: string;
}

export interface OperationStatus {
  in_progress: boolean;
  lock_age?: number;
  estimated_completion?: number;
}

// Samba

export interface SambaShare {
  name: string;
  label: string;
  path: string;
}

export interface SambaStatus {
  user: string;
  config_path: string;
  password_set: boolean;
  shares: SambaShare[];
}

// Config

export interface NetworkConfigPublic {
  web_port: number;
}

export interface OfflineAPPublic {
  enabled: boolean;
  ssid: string;
  passphrase: string;
  channel: number;
  ipv4_cidr: string;
  dhcp_start: string;
  dhcp_end: string;
  check_interval: number;
  disconnect_grace: number;
  min_rssi: number;
  stable_seconds: number;
  ping_target: string;
  retry_seconds: number;
  force_mode: string;
}

export interface WebConfigPublic {
  max_lock_chime_size: number;
  max_lock_chime_duration: number;
  min_lock_chime_duration: number;
  speed_range_min: number;
  speed_range_max: number;
  speed_step: number;
  max_upload_size_mb: number;
  max_upload_chunk_mb: number;
}

export interface TelegramConfigPublic {
  enabled: boolean;
  bot_token: string;
  chat_id: string;
  offline_mode: string;
  max_queue_size: number;
  video_quality: string;
}

export interface UpdateConfigPublic {
  auto_update: boolean;
  check_on_startup: boolean;
  channel: string;
}

export interface StorageInfo {
  cam_name: string;
  cam_label: string;
  part2_enabled: boolean;
  chimes_enabled: boolean;
  lightshow_enabled: boolean;
  wraps_enabled: boolean;
  music_enabled: boolean;
  music_fs: string;
  boot_fsck_enabled: boolean;
  install_dir: string;
  mount_dir: string;
  target_user: string;
}

export interface ConfigResponse {
  network: NetworkConfigPublic;
  offline_ap: OfflineAPPublic;
  web: WebConfigPublic;
  telegram: TelegramConfigPublic;
  update: UpdateConfigPublic;
  storage: StorageInfo;
}

// Logs

export type LogPriority = "error" | "warn" | "info" | "debug";

export interface LogLine {
  timestamp: string;
  priority: LogPriority;
  message: string;
}
