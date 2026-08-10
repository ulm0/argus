import type {
  AppStatus,
  APConfig,
  ArchiveConfigPublic,
  ArchiveStatus,
  APStatus,
  BluetoothDevice,
  BluetoothStatus,
  ChimeFilenamesResponse,
  ChimeGroup,
  ChimeGroupsResponse,
  ChimeListResponse,
  ChunkUploadResponse,
  CleanupPolicy,
  CleanupPreviewResponse,
  CleanupReport,
  CleanupSettingsResponse,
  CompleteAnalytics,
  ConfigResponse,
  FsckCheckResult,
  FsckHistoryResponse,
  FsckMode,
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
  StartupConfigPublic,
  StatusResponse,
  SystemMetrics,
  TelegramConfigPublic,
  TelegramStatus,
  UpdateStatus,
  UpdateConfigPublic,
  ViewerPrefsConfigPublic,
  VideoEvent,
  VideoEventsResponse,
  VideoListResponse,
  VideoSessionsResponse,
  VideoStats,
  WebConfigPublic,
  SavedWifiNetwork,
  WifiScanResponse,
  WifiStatus,
  WrapListResponse,
} from "./types";

// encodePath percent-encodes each segment of a multi-segment relative path
// while preserving the structural "/" separators. Filenames/directory names
// come from disk content (e.g. Tesla-written USB media), so a name containing
// "#", "?", "%", "&" or "=" must not be interpolated raw into a URL — it would
// otherwise truncate the path at a fragment, inject a query string, or smuggle
// extra query parameters into the request.
function encodePath(relativePath: string): string {
  return relativePath.split("/").map(encodeURIComponent).join("/");
}

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

// UNAUTHORIZED_EVENT is dispatched on window whenever the API returns 401 so the
// login gate can re-prompt without every caller having to handle it.
export const UNAUTHORIZED_EVENT = "argus:unauthorized";

function notifyUnauthorized() {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
  }
}

// Requests are bounded so a Pi that stops answering mid-request surfaces as an
// error instead of a spinner that never resolves. Uploads get a much longer
// budget — writing tens of MB to a USB-backed loop image on a Pi Zero is slow
// but legitimate.
const REQUEST_TIMEOUT_MS = 15_000;
const UPLOAD_TIMEOUT_MS = 120_000;
// SLOW_TIMEOUT_MS covers the handful of endpoints that do real work synchronously
// before answering — a mode switch unmounts, tears down loops and rebinds the USB
// gadget; cleanup deletes hundreds of directories; fsck and Samba restarts shell
// out. Timing those out at 15s would report "device not responding" for an
// operation that is still running and about to succeed.
const SLOW_TIMEOUT_MS = 180_000;

// UNREACHABLE_STATUS marks an ApiError that never reached the server (timeout,
// DNS failure, link dropped). Callers can tell it apart from a real HTTP error.
export const UNREACHABLE_STATUS = 0;

// toApiError normalises a fetch rejection. A timeout, an abort, or a TypeError
// (the shape fetch uses for every network-layer failure) all mean the same
// thing to the user: the device did not answer.
function toApiError(err: unknown): ApiError {
  if (err instanceof ApiError) return err;
  if (
    (err instanceof DOMException &&
      (err.name === "TimeoutError" || err.name === "AbortError")) ||
    err instanceof TypeError
  ) {
    return new ApiError(UNREACHABLE_STATUS, "Device not responding");
  }
  return new ApiError(UNREACHABLE_STATUS, err instanceof Error ? err.message : "Request failed");
}

async function request<T>(
  url: string,
  options?: RequestInit,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, {
      headers: { "Content-Type": "application/json", ...options?.headers },
      ...options,
      // After the spread so an explicit caller signal wins, but an absent one
      // still gets a deadline.
      signal: options?.signal ?? AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
  } catch (err) {
    throw toApiError(err);
  }

  if (!res.ok) {
    if (res.status === 401) notifyUnauthorized();
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

// postSlow is post() with the long deadline, for the endpoints that block on
// real device work rather than returning a job handle.
async function postSlow<T>(url: string, body?: unknown): Promise<T> {
  return request<T>(url, {
    method: "POST",
    body: body != null ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(SLOW_TIMEOUT_MS),
  });
}

async function postForm<T>(url: string, form: FormData, signal?: AbortSignal): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, {
      method: "POST",
      body: form,
      signal: signal ?? AbortSignal.timeout(UPLOAD_TIMEOUT_MS),
    });
  } catch (err) {
    throw toApiError(err);
  }
  if (!res.ok) {
    if (res.status === 401) notifyUnauthorized();
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
// Auth
// ──────────────────────────────────────────────

export interface AuthStatus {
  enabled: boolean;
  authenticated: boolean;
  using_default: boolean;
}

export function getAuthStatus(): Promise<AuthStatus> {
  return request<AuthStatus>("/api/auth/status");
}

export function login(username: string, password: string): Promise<{ status: string; using_default: boolean }> {
  return post("/api/login", { username, password });
}

export function logout(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/logout");
}

// ──────────────────────────────────────────────
// Mode
// ──────────────────────────────────────────────

export function getStatus(): Promise<AppStatus> {
  return request<AppStatus>("/api/status");
}

export function switchToPresent(): Promise<StatusResponse> {
  return postSlow<StatusResponse>("/api/present");
}

export function switchToEdit(): Promise<StatusResponse> {
  return postSlow<StatusResponse>("/api/edit");
}

export function getGadgetState(): Promise<GadgetState> {
  return request<GadgetState>("/api/gadget/state");
}

export function recoverGadget(): Promise<StatusResponse> {
  return postSlow<StatusResponse>("/api/gadget/recover");
}

export function getOperationStatus(): Promise<OperationStatus> {
  return request<OperationStatus>("/api/operation/status");
}

// ──────────────────────────────────────────────
// Videos
// ──────────────────────────────────────────────

export function getVideos(
  folder?: string,
  page = 0,
  perPage = 20,
  mode?: "sessions",
  // before is a YYYY-MM-DD date; the server drops events newer than it so the
  // list can jump straight to a day instead of scrolling through months.
  before?: string,
): Promise<VideoListResponse | VideoEventsResponse | VideoSessionsResponse> {
  const params = new URLSearchParams();
  if (folder) params.set("folder", folder);
  if (page) params.set("page", String(page));
  if (perPage !== 20) params.set("per_page", String(perPage));
  if (mode) params.set("mode", mode);
  if (before) params.set("before", before);
  return request(`/api/videos?${params}`);
}

export function getEvent(folder: string, event: string): Promise<VideoEvent> {
  return request<VideoEvent>(`/api/videos/${encodePath(folder)}/${encodeURIComponent(event)}`);
}

export function getSessionDetail(folder: string, session: string): Promise<VideoEvent> {
  return request<VideoEvent>(`/api/videos/session-detail/${encodePath(folder)}/${encodeURIComponent(session)}`);
}

export function streamURL(relativePath: string): string {
  return `/api/videos/stream/${encodePath(relativePath)}`;
}

export function downloadURL(relativePath: string): string {
  return `/api/videos/download/${encodePath(relativePath)}`;
}

export function seiURL(relativePath: string): string {
  return `/api/videos/sei/${encodePath(relativePath)}`;
}

export function telemetryURL(relativePath: string): string {
  return `/api/videos/telemetry/${encodePath(relativePath)}`;
}

export function downloadEventURL(folder: string, event: string): string {
  return `/api/videos/download-event/${encodePath(folder)}/${encodeURIComponent(event)}`;
}

export function thumbnailURL(folder: string, event: string, camera?: string, w?: number, h?: number): string {
  const params = new URLSearchParams();
  if (camera) params.set("camera", camera);
  if (w) params.set("w", String(w));
  if (h) params.set("h", String(h));
  const qs = params.toString();
  return `/api/videos/thumbnail/${encodePath(folder)}/${encodeURIComponent(event)}${qs ? `?${qs}` : ""}`;
}

export function deleteEvent(folder: string, event: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/videos/delete/${encodePath(folder)}/${encodeURIComponent(event)}`);
}

// keepEvent toggles the `.argus-keep` marker that excludes an event from cleanup.
export function keepEvent(folder: string, event: string): Promise<{ kept: boolean }> {
  return post<{ kept: boolean }>(`/api/videos/keep/${encodePath(folder)}/${encodeURIComponent(event)}`);
}

// exportURL trims a single camera file to [start, end] seconds server-side and
// returns it as a downloadable MP4 — the shareable form of "here's the moment".
export function exportURL(relativePath: string, start: number, end: number): string {
  const params = new URLSearchParams({ start: start.toFixed(2), end: end.toFixed(2) });
  return `/api/videos/export/${encodePath(relativePath)}?${params}`;
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

export function listSchedules(): Promise<{ schedules: Schedule[] | null }> {
  return request<{ schedules: Schedule[] | null }>("/api/chimes/schedules");
}

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
// Boombox (custom sounds, same partition/pipeline as chimes)
// ──────────────────────────────────────────────

export function getBoombox(): Promise<{ sounds: string[] | null; max_playable: number }> {
  return request<{ sounds: string[] | null; max_playable: number }>("/api/boombox");
}

export function uploadBoombox(file: File): Promise<StatusResponse> {
  const form = new FormData();
  form.append("file", file);
  return postForm<StatusResponse>("/api/boombox/upload", form);
}

export function deleteBoombox(filename: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/boombox/delete/${encodeURIComponent(filename)}`);
}

export function playBoomboxURL(filename: string): string {
  return `/api/boombox/play/${encodeURIComponent(filename)}`;
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
  return post<StatusResponse>(`/api/music/delete/${encodePath(relativePath)}`);
}

export function deleteDir(relativePath: string): Promise<StatusResponse> {
  return post<StatusResponse>(`/api/music/delete-dir/${encodePath(relativePath)}`);
}

export function moveFile(source: string, destination: string, newName: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/music/move", { source, destination, new_name: newName });
}

export function mkdir(path: string, name: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/music/mkdir", { path, name });
}

export function playURL(relativePath: string): string {
  return `/api/music/play/${encodePath(relativePath)}`;
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

export function getSystemMetrics(): Promise<SystemMetrics> {
  return request<SystemMetrics>("/api/analytics/system");
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
  return postSlow<CleanupReport>("/api/cleanup/execute", { dry_run: dryRun });
}

// ──────────────────────────────────────────────
// Fsck
// ──────────────────────────────────────────────

export function startFsck(partitions?: string[], mode?: FsckMode): Promise<StatusResponse> {
  const body: { partitions?: string[]; mode?: FsckMode } = {};
  if (partitions && partitions.length > 0) body.partitions = partitions;
  if (mode) body.mode = mode;
  return postSlow<StatusResponse>("/api/fsck/start", body);
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
  return postSlow<StatusResponse>("/api/ap/force", { mode });
}

export function configureAP(config: Partial<APConfig>): Promise<{ status: string; config: APConfig }> {
  return post("/api/ap/configure", config);
}

export function setAPShareInternet(enabled: boolean): Promise<StatusResponse> {
  return post<StatusResponse>("/api/ap/share-internet", { enabled });
}

// ──────────────────────────────────────────────
// Bluetooth tethering
// ──────────────────────────────────────────────

export function getBluetoothStatus(): Promise<BluetoothStatus> {
  return request<BluetoothStatus>("/api/bluetooth/status");
}

export function getBluetoothDevices(): Promise<{ devices: BluetoothDevice[] }> {
  return request<{ devices: BluetoothDevice[] }>("/api/bluetooth/devices");
}

export function scanBluetooth(): Promise<{ devices: BluetoothDevice[] }> {
  return request<{ devices: BluetoothDevice[] }>("/api/bluetooth/scan");
}

export function setBluetoothPower(enabled: boolean): Promise<StatusResponse> {
  return post<StatusResponse>("/api/bluetooth/power", { enabled });
}

export function setBluetoothDiscoverable(enabled: boolean): Promise<StatusResponse> {
  return post<StatusResponse>("/api/bluetooth/discoverable", { enabled });
}

export function bluetoothPair(mac: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/bluetooth/pair", { mac });
}

export function bluetoothRemove(mac: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/bluetooth/remove", { mac });
}

export function bluetoothConnect(mac: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/bluetooth/connect", { mac });
}

export function bluetoothDisconnect(mac: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/bluetooth/disconnect", { mac });
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
  return postSlow<StatusResponse>("/api/wifi/configure", { ssid, password });
}

export function dismissWifiStatus(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/wifi/dismiss-status");
}

export function getSavedWifi(): Promise<{ networks: SavedWifiNetwork[] }> {
  return request<{ networks: SavedWifiNetwork[] }>("/api/wifi/saved");
}

export function forgetWifi(uuid: string): Promise<StatusResponse> {
  return post<StatusResponse>("/api/wifi/forget", { uuid });
}

export function setWifiAutoConnect(uuid: string, autoconnect: boolean): Promise<StatusResponse> {
  return post<StatusResponse>("/api/wifi/autoconnect", { uuid, autoconnect });
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
  return postSlow<StatusResponse>("/api/telegram/test");
}

// ──────────────────────────────────────────────
// Archive
// ──────────────────────────────────────────────

export function getArchiveStatus(): Promise<ArchiveStatus> {
  return request<ArchiveStatus>("/api/archive/status");
}

export function runArchive(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/archive/run");
}

// ──────────────────────────────────────────────
// Update
// ──────────────────────────────────────────────
export function getUpdateStatus(): Promise<UpdateStatus> {
  return request<UpdateStatus>("/api/update/status");
}

// ──────────────────────────────────────────────
// System power
// ──────────────────────────────────────────────

export function rebootSystem(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/system/reboot");
}

export function powerOffSystem(): Promise<StatusResponse> {
  return post<StatusResponse>("/api/system/poweroff");
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

export function setSambaEnabled(enabled: boolean): Promise<{ enabled: boolean }> {
  return post<{ enabled: boolean }>("/api/samba/set-enabled", { enabled });
}

export function restartSamba(): Promise<StatusResponse> {
  return postSlow<StatusResponse>("/api/samba/restart");
}

export function regenerateSambaConfig(): Promise<StatusResponse> {
  return postSlow<StatusResponse>("/api/samba/regenerate");
}

export { ApiError };

// ──────────────────────────────────────────────
// Config
// ──────────────────────────────────────────────

export function getConfig(): Promise<ConfigResponse> {
  return request<ConfigResponse>("/api/config");
}

// The PATCH body is not the mirror of the GET response for the sections that
// hold secrets: the server never returns the Telegram bot token or the AP
// passphrase, but it does accept them. Spell those two out rather than deriving
// them from the read shapes.
type TelegramPatch = Omit<Partial<TelegramConfigPublic>, "bot_token_set"> & {
  bot_token?: string;
};

type OfflineAPPatch = Partial<OfflineAPPublic> & { passphrase?: string };

type AuthPatch = { enabled?: boolean; username?: string; password?: string };

type ConfigPatch = {
  archive?: Partial<ArchiveConfigPublic>;
  network?: Partial<NetworkConfigPublic>;
  offline_ap?: OfflineAPPatch;
  web?: Partial<WebConfigPublic>;
  telegram?: TelegramPatch;
  update?: Partial<UpdateConfigPublic>;
  startup?: Partial<StartupConfigPublic>;
  viewer_prefs?: Partial<ViewerPrefsConfigPublic>;
  auth?: AuthPatch;
  log_level?: string;
};

export function patchConfig(patch: ConfigPatch): Promise<StatusResponse> {
  return request<StatusResponse>("/api/config", {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

// ──────────────────────────────────────────────
// Webhook
// ──────────────────────────────────────────────

export interface WebhookStatus {
  enabled: boolean;
  url: string;
  has_secret: boolean;
}

export function getWebhookStatus(): Promise<WebhookStatus> {
  return request<WebhookStatus>("/api/webhook/status");
}

export function configureWebhook(data: {
  enabled: boolean;
  url: string;
  secret: string;
}): Promise<StatusResponse> {
  return post<StatusResponse>("/api/webhook/configure", data);
}

// ──────────────────────────────────────────────
// Sentry Events (SSE)
// ──────────────────────────────────────────────

export function sentryEventsURL(): string {
  return "/api/events/stream";
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
