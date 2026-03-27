import type {
  AppStatus,
  APConfig,
  APStatus,
  ChimeFilenamesResponse,
  ChimeGroup,
  ChimeGroupsResponse,
  ChimeListResponse,
  ChunkUploadResponse,
  CleanupPlan,
  CleanupPolicy,
  CleanupPreviewResponse,
  CleanupReport,
  CleanupSettingsResponse,
  CompleteAnalytics,
  ConfigResponse,
  FsckCheckResult,
  FsckHistoryResponse,
  FsckStatus,
  GadgetState,
  HealthStatus,
  LightShowListResponse,
  LogLine,
  MusicListResult,
  NetworkConfigPublic,
  OfflineAPPublic,
  OperationStatus,
  PartitionUsage,
  SambaStatus,
  Schedule,
  StatusResponse,
  TelegramConfigPublic,
  TelegramStatus,
  UpdateConfigPublic,
  VideoEvent,
  VideoEventsResponse,
  VideoListResponse,
  VideoSessionsResponse,
  VideoStats,
  WebConfigPublic,
  WifiScanResponse,
  WifiStatus,
  WrapListResponse,
} from "./types";

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(
  url: string,
  options?: RequestInit,
): Promise<T> {
  const res = await fetch(url, {
    headers: { "Content-Type": "application/json", ...options?.headers },
    ...options,
  });

  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = await res.json();
      if (body.error) msg = body.error;
    } catch {
      /* use statusText */
    }
    throw new ApiError(res.status, msg);
  }

  return res.json() as Promise<T>;
}

async function post<T>(url: string, body?: unknown): Promise<T> {
  return request<T>(url, {
    method: "POST",
    body: body != null ? JSON.stringify(body) : undefined,
  });
}

async function postForm<T>(url: string, form: FormData): Promise<T> {
  const res = await fetch(url, { method: "POST", body: form });
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = await res.json();
      if (body.error) msg = body.error;
    } catch {
      /* use statusText */
    }
    throw new ApiError(res.status, msg);
  }
  return res.json() as Promise<T>;
}

// ──────────────────────────────────────────────
// Mode
// ──────────────────────────────────────────────

export function getStatus(): Promise<AppStatus> {
  return request<AppStatus>("/api/status");
}

export function switchToPresent(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/present");
}

export function switchToEdit(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/edit");
}

export function getGadgetState(): Promise<GadgetState> {
  return request<GadgetState>("/api/gadget/state");
}

export function recoverGadget(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/gadget/recover");
}

export function getOperationStatus(): Promise<OperationStatus> {
  return request<OperationStatus>("/api/operation/status");
}

// ──────────────────────────────────────────────
// Videos
// ──────────────────────────────────────────────

export function getVideos(folder?: string, page = 0, perPage = 20, mode?: "sessions"): Promise<VideoListResponse | VideoEventsResponse | VideoSessionsResponse> {
  const params = new URLSearchParams();
  if (folder) params.set("folder", folder);
  if (page) params.set("page", String(page));
  if (perPage !== 20) params.set("per_page", String(perPage));
  if (mode) params.set("mode", mode);
  return request(`/api/videos?${params}`);
}

export function getEvent(folder: string, event: string): Promise<VideoEvent> {
  return request<VideoEvent>(`/api/videos/${encodeURIComponent(folder)}/${encodeURIComponent(event)}`);
}

export function streamURL(relativePath: string): string {
  return `/api/videos/stream/${relativePath}`;
}

export function downloadURL(relativePath: string): string {
  return `/api/videos/download/${relativePath}`;
}

export function seiURL(relativePath: string): string {
  return `/api/videos/sei/${relativePath}`;
}

export function downloadEventURL(folder: string, event: string): string {
  return `/api/videos/download-event/${encodeURIComponent(folder)}/${encodeURIComponent(event)}`;
}

export function thumbnailURL(folder: string, event: string, camera?: string, w?: number, h?: number): string {
  const params = new URLSearchParams();
  if (camera) params.set("camera", camera);
  if (w) params.set("w", String(w));
  if (h) params.set("h", String(h));
  const qs = params.toString();
  return `/api/videos/thumbnail/${encodeURIComponent(folder)}/${encodeURIComponent(event)}${qs ? `?${qs}` : ""}`;
}

export function deleteEvent(folder: string, event: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/videos/delete/${encodeURIComponent(folder)}/${encodeURIComponent(event)}`);
}

// ──────────────────────────────────────────────
// Chimes
// ──────────────────────────────────────────────

export function getChimes(): Promise<ChimeListResponse> {
  return request<ChimeListResponse>("/api/chimes");
}

export function uploadChime(file: File, normalize = false, targetLufs = -14): Promise<StatusResponse> {
  const form = new FormData();
  form.append("file", file);
  form.append("normalize", String(normalize));
  form.append("target_lufs", String(targetLufs));
  return postForm<StatusResponse>("/api/chimes/upload", form);
}

export function setActiveChime(filename: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/set/${encodeURIComponent(filename)}`);
}

export function deleteChime(filename: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/delete/${encodeURIComponent(filename)}`);
}

export function renameChime(oldName: string, newName: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/rename/${encodeURIComponent(oldName)}/${encodeURIComponent(newName)}`);
}

export function getFilenames(): Promise<ChimeFilenamesResponse> {
  return request<ChimeFilenamesResponse>("/api/chimes/filenames");
}

export function playChimeURL(filename: string): string {
  return `/api/chimes/play/${encodeURIComponent(filename)}`;
}

export function playActiveChimeURL(): string {
  return "/api/chimes/play/active";
}

// ──────────────────────────────────────────────
// Chime Scheduler
// ──────────────────────────────────────────────

export function addSchedule(schedule: Omit<Schedule, "id">): Promise<StatusResponse> {
  return post<StatusResponse>("/api/chimes/schedule/add", schedule);
}

export function toggleSchedule(id: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/schedule/${encodeURIComponent(id)}/toggle`);
}

export function deleteSchedule(id: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/schedule/${encodeURIComponent(id)}/delete`);
}

export function getSchedule(id: string): Promise<Schedule> {
  return request<Schedule>(`/api/chimes/schedule/${encodeURIComponent(id)}`);
}

export function editSchedule(id: string, updates: Partial<Schedule>): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/schedule/${encodeURIComponent(id)}/edit`, updates);
}

// ──────────────────────────────────────────────
// Chime Groups
// ──────────────────────────────────────────────

export function listGroups(): Promise<ChimeGroupsResponse> {
  return request<ChimeGroupsResponse>("/api/chimes/groups");
}

export function createGroup(name: string, description: string, chimes: string[]): Promise<StatusResponse> {
  return post<StatusResponse>("/api/chimes/groups/create", { name, description, chimes });
}

export function updateGroup(id: string, name: string, description: string, chimes: string[]): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/groups/${encodeURIComponent(id)}/update`, { name, description, chimes });
}

export function deleteGroup(id: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/groups/${encodeURIComponent(id)}/delete`);
}

export function addChimeToGroup(groupId: string, filename: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/groups/${encodeURIComponent(groupId)}/add-chime`, { filename });
}

export function removeChimeFromGroup(groupId: string, filename: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/chimes/groups/${encodeURIComponent(groupId)}/remove-chime`, { filename });
}

export function setRandomMode(enabled: boolean, groupId: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/chimes/groups/random-mode", { enabled, group_id: groupId });
}

// ──────────────────────────────────────────────
// Light Shows
// ──────────────────────────────────────────────

export function getShows(): Promise<LightShowListResponse> {
  return request<LightShowListResponse>("/api/lightshows");
}

export function uploadShow(file: File): Promise<StatusResponse> {
  const form = new FormData();
  form.append("file", file);
  return postForm<StatusResponse>("/api/lightshows/upload", form);
}

export function deleteShow(partition: string, baseName: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/lightshows/delete/${encodeURIComponent(partition)}/${encodeURIComponent(baseName)}`);
}

export function downloadShowURL(partition: string, baseName: string): string {
  return `/api/lightshows/download/${encodeURIComponent(partition)}/${encodeURIComponent(baseName)}`;
}

export function playShowURL(partition: string, filename: string): string {
  return `/api/lightshows/play/${encodeURIComponent(partition)}/${encodeURIComponent(filename)}`;
}

// ──────────────────────────────────────────────
// Wraps
// ──────────────────────────────────────────────

export function getWraps(): Promise<WrapListResponse> {
  return request<WrapListResponse>("/api/wraps");
}

export function uploadWrap(file: File): Promise<StatusResponse> {
  const form = new FormData();
  form.append("file", file);
  return postForm<StatusResponse>("/api/wraps/upload", form);
}

export function deleteWrap(partition: string, filename: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/wraps/delete/${encodeURIComponent(partition)}/${encodeURIComponent(filename)}`);
}

export function wrapThumbnailURL(partition: string, filename: string): string {
  return `/api/wraps/thumbnail/${encodeURIComponent(partition)}/${encodeURIComponent(filename)}`;
}

export function downloadWrapURL(partition: string, filename: string): string {
  return `/api/wraps/download/${encodeURIComponent(partition)}/${encodeURIComponent(filename)}`;
}

// ──────────────────────────────────────────────
// Music
// ──────────────────────────────────────────────

export function listFiles(path = ""): Promise<MusicListResult> {
  const params = path ? `?path=${encodeURIComponent(path)}` : "";
  return request<MusicListResult>(`/api/music${params}`);
}

export function uploadChunk(
  chunk: Blob,
  uploadId: string,
  filename: string,
  chunkIndex: number,
  totalChunks: number,
  path = "",
): Promise<ChunkUploadResponse> {
  const form = new FormData();
  form.append("chunk", chunk);
  form.append("upload_id", uploadId);
  form.append("filename", filename);
  form.append("chunk_index", String(chunkIndex));
  form.append("total_chunks", String(totalChunks));
  form.append("path", path);
  return postForm<ChunkUploadResponse>("/api/music/upload-chunk", form);
}

export function deleteFile(relativePath: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/music/delete/${relativePath}`);
}

export function deleteDir(relativePath: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/music/delete-dir/${relativePath}`);
}

export function moveFile(source: string, destination: string, newName: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/music/move", { source, destination, new_name: newName });
}

export function mkdir(path: string, name: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/music/mkdir", { path, name });
}

export function playURL(relativePath: string): string {
  return `/api/music/play/${relativePath}`;
}

// ──────────────────────────────────────────────
// Analytics
// ──────────────────────────────────────────────

export function getDashboard(): Promise<CompleteAnalytics> {
  return request<CompleteAnalytics>("/api/analytics");
}

export function getPartitionUsage(): Promise<{ partitions: PartitionUsage[] }> {
  return request("/api/analytics/partition-usage");
}

export function getVideoStats(): Promise<{ statistics: VideoStats[] }> {
  return request("/api/analytics/video-stats");
}

export function getHealth(): Promise<HealthStatus> {
  return request<HealthStatus>("/api/analytics/health");
}

// ──────────────────────────────────────────────
// Cleanup
// ──────────────────────────────────────────────

export function getSettings(): Promise<CleanupSettingsResponse> {
  return request<CleanupSettingsResponse>("/api/cleanup/settings");
}

export function saveSettings(policies: Record<string, CleanupPolicy>): Promise<StatusResponse> {
  return post<StatusResponse>("/api/cleanup/settings", { policies });
}

export function getPreview(): Promise<CleanupPreviewResponse> {
  return request<CleanupPreviewResponse>("/api/cleanup/preview");
}

export function executeCleanup(dryRun = false): Promise<CleanupReport> {
  return post<CleanupReport>("/api/cleanup/execute", { dry_run: dryRun });
}

export function calculateCleanup(): Promise<CleanupPlan> {
  return post<CleanupPlan>("/api/cleanup/calculate");
}

// ──────────────────────────────────────────────
// Fsck
// ──────────────────────────────────────────────

export function startFsck(partitions?: string[]): Promise<StatusResponse> {
  return post<StatusResponse>("/api/fsck/start", partitions ? { partitions } : {});
}

export function getFsckStatus(): Promise<FsckStatus> {
  return request<FsckStatus>("/api/fsck/status");
}

export function cancelFsck(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/fsck/cancel");
}

export function getFsckHistory(): Promise<FsckHistoryResponse> {
  return request<FsckHistoryResponse>("/api/fsck/history");
}

export function getLastCheck(partition: string): Promise<FsckCheckResult> {
  return request<FsckCheckResult>(`/api/fsck/last-check/${encodeURIComponent(partition)}`);
}

// ──────────────────────────────────────────────
// Access Point
// ──────────────────────────────────────────────

export function getAPStatus(): Promise<APStatus> {
  return request<APStatus>("/api/ap/status");
}

export function forceAP(mode: "auto" | "on" | "off"): Promise<StatusResponse> {
  return post<StatusResponse>("/api/ap/force", { mode });
}

export function configureAP(config: Partial<APConfig>): Promise<{ status: string; config: APConfig }> {
  return post("/api/ap/configure", config);
}

// ──────────────────────────────────────────────
// WiFi
// ──────────────────────────────────────────────

export function getWifiStatus(): Promise<WifiStatus> {
  return request<WifiStatus>("/api/wifi/status");
}

export function scanWifi(): Promise<WifiScanResponse> {
  return request<WifiScanResponse>("/api/wifi/scan");
}

export function configureWifi(ssid: string, password: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/wifi/configure", { ssid, password });
}

export function dismissWifiStatus(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/wifi/dismiss-status");
}

// ──────────────────────────────────────────────
// Telegram
// ──────────────────────────────────────────────

export function getTelegramStatus(): Promise<TelegramStatus> {
  return request<TelegramStatus>("/api/telegram/status");
}

export function configureTelegram(
  botToken: string,
  chatId: string,
  offlineMode?: string,
  videoQuality?: string,
): Promise<StatusResponse> {
  return post<StatusResponse>("/api/telegram/configure", {
    bot_token: botToken,
    chat_id: chatId,
    offline_mode: offlineMode,
    video_quality: videoQuality,
  });
}

export function testTelegram(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/telegram/test");
}

// ──────────────────────────────────────────────
// Samba
// ──────────────────────────────────────────────

export function getSambaStatus(): Promise<SambaStatus> {
  return request<SambaStatus>("/api/samba/status");
}

export function setSambaPassword(password: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/samba/set-password", { password });
}

export function restartSamba(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/samba/restart");
}

export function regenerateSambaConfig(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/samba/regenerate");
}

export { ApiError };

// ──────────────────────────────────────────────
// Config
// ──────────────────────────────────────────────

export function getConfig(): Promise<ConfigResponse> {
  return request<ConfigResponse>("/api/config");
}

type ConfigPatch = {
  network?: Partial<NetworkConfigPublic>;
  offline_ap?: Partial<OfflineAPPublic>;
  web?: Partial<WebConfigPublic>;
  telegram?: Partial<TelegramConfigPublic>;
  update?: Partial<UpdateConfigPublic>;
  log_level?: string;
};

export function patchConfig(patch: ConfigPatch): Promise<StatusResponse> {
  return request<StatusResponse>("/api/config", {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

// ──────────────────────────────────────────────
// Logs
// ──────────────────────────────────────────────

export function openLogsStream(
  opts: { unit?: string; n?: number; follow?: boolean } = {},
): EventSource {
  const params = new URLSearchParams();
  if (opts.unit) params.set("unit", opts.unit);
  if (opts.n !== undefined) params.set("n", String(opts.n));
  if (opts.follow === false) params.set("follow", "false");
  return new EventSource(`/api/logs?${params}`);
}

export type { LogLine };
