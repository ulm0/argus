"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import * as api from "@/lib/api";
import type { ChimeGroup, Schedule } from "@/lib/types";
import { useFeatureGuard } from "@/hooks/useFeatureGuard";

const LUFS_PRESETS = [
  { label: "Quiet (-20)", value: -20 },
  { label: "Normal (-14)", value: -14 },
  { label: "Loud (-9)", value: -9 },
];

export default function ChimesPage() {
  const { available, loading: featureLoading } = useFeatureGuard("chimes_available");
  const [chimes, setChimes] = useState<string[]>([]);
  const [active, setActive] = useState("");
  const [activeExists, setActiveExists] = useState(false);
  const [randomMode, setRandomMode] = useState({ enabled: false, group_id: "" });

  const [groups, setGroups] = useState<ChimeGroup[]>([]);
  const [schedules, setSchedules] = useState<Schedule[]>([]);

  const [normalize, setNormalize] = useState(false);
  const [targetLUFS, setTargetLUFS] = useState(-14);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [playing, setPlaying] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  const [newGroupName, setNewGroupName] = useState("");
  const [newGroupDesc, setNewGroupDesc] = useState("");
  const [showGroupForm, setShowGroupForm] = useState(false);
  const [editingGroup, setEditingGroup] = useState<string | null>(null);

  const [showScheduleForm, setShowScheduleForm] = useState(false);
  const [schedName, setSchedName] = useState("");
  const [schedChime, setSchedChime] = useState("");
  const [schedTime, setSchedTime] = useState("08:00");
  const [schedEnabled, setSchedEnabled] = useState(true);

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadAll = useCallback(async () => {
    try {
      const [chimeData, groupData] = await Promise.all([
        api.getChimes(),
        api.listGroups(),
      ]);
      setChimes(chimeData.chimes);
      setActive(chimeData.active);
      setActiveExists(chimeData.active_exists);
      setRandomMode(chimeData.random_mode);
      setGroups(groupData.groups);
    } catch {
      showToast("Failed to load chimes", false);
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadAll();
  }, [loadAll]);

  const handlePlay = (filename: string | null) => {
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current = null;
    }
    if (!filename || playing === filename) {
      setPlaying(null);
      return;
    }
    const url =
      filename === "__active__"
        ? api.playActiveChimeURL()
        : api.playChimeURL(filename);
    const audio = new Audio(url);
    audio.onended = () => setPlaying(null);
    audio.play();
    audioRef.current = audio;
    setPlaying(filename);
  };

  const handleUpload = async (files: FileList | File[]) => {
    if (!files.length) return;
    setUploading(true);
    try {
      for (const file of Array.from(files)) {
        await api.uploadChime(file, normalize, targetLUFS);
      }
      showToast(`Uploaded ${files.length} file(s)`);
      await loadAll();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Upload failed", false);
    } finally {
      setUploading(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    handleUpload(e.dataTransfer.files);
  };

  const handleSetActive = async (filename: string) => {
    try {
      await api.setActiveChime(filename);
      showToast(`Active chime set to ${filename}`);
      await loadAll();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleDelete = async (filename: string) => {
    if (!confirm(`Delete "${filename}"?`)) return;
    try {
      await api.deleteChime(filename);
      showToast("Chime deleted");
      await loadAll();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Delete failed", false);
    }
  };

  const handleCreateGroup = async () => {
    if (!newGroupName.trim()) return;
    try {
      await api.createGroup(newGroupName, newGroupDesc, []);
      showToast("Group created");
      setNewGroupName("");
      setNewGroupDesc("");
      setShowGroupForm(false);
      const res = await api.listGroups();
      setGroups(res.groups);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleDeleteGroup = async (id: string) => {
    if (!confirm("Delete this group?")) return;
    try {
      await api.deleteGroup(id);
      showToast("Group deleted");
      const res = await api.listGroups();
      setGroups(res.groups);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleToggleChimeInGroup = async (
    groupId: string,
    filename: string,
    isInGroup: boolean,
  ) => {
    try {
      if (isInGroup) {
        await api.removeChimeFromGroup(groupId, filename);
      } else {
        await api.addChimeToGroup(groupId, filename);
      }
      const res = await api.listGroups();
      setGroups(res.groups);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleRandomToggle = async () => {
    try {
      await api.setRandomMode(!randomMode.enabled, randomMode.group_id);
      const res = await api.getChimes();
      setRandomMode(res.random_mode);
      showToast(
        res.random_mode.enabled ? "Random mode enabled" : "Random mode disabled",
      );
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleAddSchedule = async () => {
    if (!schedName || !schedChime) return;
    try {
      await api.addSchedule({
        chime_filename: schedChime,
        time: schedTime,
        type: "weekly",
        enabled: schedEnabled,
        name: schedName,
      });
      showToast("Schedule created");
      setShowScheduleForm(false);
      setSchedName("");
      setSchedChime("");
      setSchedTime("08:00");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleToggleSchedule = async (id: string) => {
    try {
      await api.toggleSchedule(id);
      setSchedules((prev) =>
        prev.map((s) => (s.id === id ? { ...s, enabled: !s.enabled } : s)),
      );
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleDeleteSchedule = async (id: string) => {
    if (!confirm("Delete this schedule?")) return;
    try {
      await api.deleteSchedule(id);
      setSchedules((prev) => prev.filter((s) => s.id !== id));
      showToast("Schedule deleted");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  if (!featureLoading && !available) {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 p-8 text-center">
        <svg className="h-12 w-12 text-[var(--color-text-muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
        </svg>
        <h1 className="text-xl font-bold text-[var(--color-text-primary)]">Chimes Disabled</h1>
        <p className="max-w-sm text-sm text-[var(--color-text-muted)]">
          Chimes are not enabled. Set <code className="rounded bg-[var(--color-bg-tertiary)] px-1">chimes_enabled: true</code> (and <code className="rounded bg-[var(--color-bg-tertiary)] px-1">part2_enabled: true</code>) in your <code className="rounded bg-[var(--color-bg-tertiary)] px-1">config.yaml</code> and restart Argus.
        </p>
      </div>
    );
  }

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      {toast && (
        <div
          className={`fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-sm px-4 py-3 text-sm font-medium shadow-lg ${
            toast.ok ? "bg-[var(--color-success)] text-white" : "bg-[var(--color-danger)] text-white"
          }`}
        >
          {toast.msg}
        </div>
      )}

      <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">
        Lock Chimes
      </h1>

      {/* Active Chime */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
          Active Chime
        </h2>
        <div className="mt-3 flex items-center gap-3">
          <span className="text-sm text-[var(--color-text-secondary)]">
            {activeExists ? active || "LockChime.wav" : "No active chime set"}
          </span>
          {activeExists && (
            <button
              onClick={() => handlePlay("__active__")}
              className={`rounded-sm px-3 py-1.5 text-sm font-medium transition-colors ${
                playing === "__active__"
                  ? "bg-[var(--color-accent)] text-white"
                  : "border border-[var(--color-border)] text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]"
              }`}
            >
              {playing === "__active__" ? "Stop" : "Play"}
            </button>
          )}
        </div>
        <div className="mt-4 flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
            <input
              type="checkbox"
              checked={randomMode.enabled}
              onChange={handleRandomToggle}
              className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]"
            />
            Random mode
          </label>
        </div>
      </section>

      {/* Upload */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
          Upload Chimes
        </h2>
        <div
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={handleDrop}
          className={`mt-4 flex cursor-pointer flex-col items-center rounded-sm border-2 border-dashed p-8 transition-colors ${
            dragOver
              ? "border-[var(--color-accent)] bg-[var(--color-accent-subtle)]"
              : "border-[var(--color-border)] hover:border-[var(--color-text-muted)]"
          }`}
          onClick={() => fileInputRef.current?.click()}
        >
          <svg className="h-8 w-8 text-[var(--color-text-muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
          </svg>
          <p className="mt-2 text-sm font-medium text-[var(--color-text-secondary)]">
            {uploading ? "Uploading..." : "Drop files here or click to browse"}
          </p>
          <p className="text-xs text-[var(--color-text-muted)]">WAV, MP3, or OGG</p>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            accept=".wav,.mp3,.ogg"
            className="hidden"
            onChange={(e) => e.target.files && handleUpload(e.target.files)}
          />
        </div>
        <div className="mt-4 space-y-3">
          <label className="flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
            <input
              type="checkbox"
              checked={normalize}
              onChange={(e) => setNormalize(e.target.checked)}
              className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]"
            />
            Normalize volume
          </label>
          {normalize && (
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Target LUFS
              </label>
              <div className="mt-1 flex gap-2">
                {LUFS_PRESETS.map((p) => (
                  <button
                    key={p.value}
                    onClick={() => setTargetLUFS(p.value)}
                    className={`rounded px-3 py-1.5 text-xs font-medium transition-colors ${
                      targetLUFS === p.value
                        ? "bg-[var(--color-accent)] text-white"
                        : "border border-[var(--color-border)] text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]"
                    }`}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
              <input
                type="range"
                min={-30}
                max={-5}
                step={1}
                value={targetLUFS}
                onChange={(e) => setTargetLUFS(Number(e.target.value))}
                className="mt-2 w-full accent-[var(--color-accent)]"
              />
              <span className="text-xs text-[var(--color-text-muted)]">{targetLUFS} LUFS</span>
            </div>
          )}
        </div>
      </section>

      {/* Chime Library */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
          Library ({chimes.length})
        </h2>
        {chimes.length === 0 ? (
          <p className="mt-4 text-sm text-[var(--color-text-muted)]">No chimes uploaded yet.</p>
        ) : (
          <div className="mt-4 divide-y divide-[var(--color-border)]">
            {chimes.map((c) => (
              <div key={c} className="flex items-center justify-between py-3">
                <div className="flex items-center gap-3 overflow-hidden">
                  {c === active && (
                    <span className="shrink-0 rounded bg-[var(--color-success-bg)] px-2 py-0.5 text-xs font-medium text-[var(--color-success)]">
                      Active
                    </span>
                  )}
                  <span className="truncate text-sm text-[var(--color-text-primary)]">
                    {c}
                  </span>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <button
                    onClick={() => handlePlay(c)}
                    className={`rounded-sm p-1.5 text-xs transition-colors ${
                      playing === c
                        ? "bg-[var(--color-accent-subtle)] text-[var(--color-accent)]"
                        : "text-[var(--color-text-muted)] hover:bg-[var(--color-bg-tertiary)]"
                    }`}
                  >
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      {playing === c ? (
                        <path strokeLinecap="round" strokeLinejoin="round" d="M6 4h4v16H6zM14 4h4v16h-4z" />
                      ) : (
                        <path strokeLinecap="round" strokeLinejoin="round" d="M5 3l14 9-14 9V3z" />
                      )}
                    </svg>
                  </button>
                  <button onClick={() => handleSetActive(c)} disabled={c === active} className="rounded-sm p-1.5 text-xs text-[var(--color-text-muted)] hover:bg-[var(--color-bg-tertiary)] disabled:opacity-30">
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  </button>
                  <a href={`/api/chimes/download/${encodeURIComponent(c)}`} className="rounded-sm p-1.5 text-xs text-[var(--color-text-muted)] transition-colors hover:bg-[var(--color-bg-tertiary)]">
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                    </svg>
                  </a>
                  <button onClick={() => handleDelete(c)} className="rounded-sm p-1.5 text-xs text-[var(--color-danger)] transition-colors hover:bg-[var(--color-danger-bg)]">
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Groups */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Groups</h2>
          <button onClick={() => setShowGroupForm(!showGroupForm)} className="rounded-sm border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)]">
            {showGroupForm ? "Cancel" : "New Group"}
          </button>
        </div>
        {showGroupForm && (
          <div className="mt-4 space-y-3 rounded-sm bg-[var(--color-bg-card-nested)] p-4">
            <input type="text" placeholder="Group name" value={newGroupName} onChange={(e) => setNewGroupName(e.target.value)} className="block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-2 text-sm text-[var(--color-text-primary)]" />
            <input type="text" placeholder="Description" value={newGroupDesc} onChange={(e) => setNewGroupDesc(e.target.value)} className="block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-2 text-sm text-[var(--color-text-primary)]" />
            <button onClick={handleCreateGroup} className="rounded bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--color-accent-hover)]">Create</button>
          </div>
        )}
        {groups.length === 0 ? (
          <p className="mt-4 text-sm text-[var(--color-text-muted)]">No groups yet.</p>
        ) : (
          <div className="mt-4 space-y-4">
            {groups.map((g) => (
              <div key={g.id} className="rounded-sm border border-[var(--color-border)] p-4">
                <div className="flex items-start justify-between">
                  <div>
                    <h3 className="font-semibold text-[var(--color-text-primary)]">{g.name}</h3>
                    {g.description && <p className="text-xs text-[var(--color-text-muted)]">{g.description}</p>}
                    <p className="mt-1 text-xs text-[var(--color-text-muted)]">{g.chimes?.length || 0} chimes</p>
                  </div>
                  <div className="flex gap-1">
                    <button onClick={() => setEditingGroup(editingGroup === g.id ? null : g.id)} className="rounded-sm p-1.5 text-[var(--color-text-muted)] hover:bg-[var(--color-bg-tertiary)]">
                      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
                    </button>
                    <button onClick={() => handleDeleteGroup(g.id)} className="rounded-sm p-1.5 text-[var(--color-danger)] hover:bg-[var(--color-danger-bg)]">
                      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                    </button>
                  </div>
                </div>
                {editingGroup === g.id && (
                  <div className="mt-3 space-y-1">
                    <p className="text-xs font-medium text-[var(--color-text-secondary)]">Toggle chimes in this group:</p>
                    <div className="flex flex-wrap gap-1">
                      {chimes.map((c) => {
                        const inGroup = g.chimes?.includes(c) ?? false;
                        return (
                          <button key={c} onClick={() => handleToggleChimeInGroup(g.id, c, inGroup)} className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${inGroup ? "bg-[var(--color-accent-subtle)] text-[var(--color-accent)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)] hover:bg-[var(--color-border)]"}`}>
                            {c}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Schedules */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Schedules</h2>
          <button onClick={() => setShowScheduleForm(!showScheduleForm)} className="rounded-sm border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)]">
            {showScheduleForm ? "Cancel" : "Add Schedule"}
          </button>
        </div>
        {showScheduleForm && (
          <div className="mt-4 space-y-3 rounded-sm bg-[var(--color-bg-card-nested)] p-4">
            <input type="text" placeholder="Schedule name" value={schedName} onChange={(e) => setSchedName(e.target.value)} className="block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-2 text-sm text-[var(--color-text-primary)]" />
            <select value={schedChime} onChange={(e) => setSchedChime(e.target.value)} className="block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-2 text-sm text-[var(--color-text-primary)]">
              <option value="">Select chime...</option>
              {chimes.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
            <input type="time" value={schedTime} onChange={(e) => setSchedTime(e.target.value)} className="block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-2 text-sm text-[var(--color-text-primary)]" />
            <label className="flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
              <input type="checkbox" checked={schedEnabled} onChange={(e) => setSchedEnabled(e.target.checked)} className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]" />
              Enabled
            </label>
            <button onClick={handleAddSchedule} className="rounded bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--color-accent-hover)]">Create Schedule</button>
          </div>
        )}
        {schedules.length === 0 ? (
          <p className="mt-4 text-sm text-[var(--color-text-muted)]">No schedules configured.</p>
        ) : (
          <div className="mt-4 divide-y divide-[var(--color-border)]">
            {schedules.map((s) => (
              <div key={s.id} className="flex items-center justify-between py-3">
                <div>
                  <p className="text-sm font-medium text-[var(--color-text-primary)]">{s.name}</p>
                  <p className="text-xs text-[var(--color-text-muted)]">{s.chime_filename} &middot; {s.time}</p>
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => handleToggleSchedule(s.id)} className={`rounded px-2.5 py-1 text-xs font-medium ${s.enabled ? "bg-[var(--color-success-bg)] text-[var(--color-success)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"}`}>
                    {s.enabled ? "On" : "Off"}
                  </button>
                  <button onClick={() => handleDeleteSchedule(s.id)} className="rounded-sm p-1.5 text-[var(--color-danger)] hover:bg-[var(--color-danger-bg)]">
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
