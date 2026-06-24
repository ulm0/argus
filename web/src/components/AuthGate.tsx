"use client";

import { useCallback, useEffect, useState } from "react";
import * as api from "@/lib/api";
import { UNAUTHORIZED_EVENT } from "@/lib/api";

type Phase = "loading" | "authed" | "login";

export default function AuthGate({ children }: { children: React.ReactNode }) {
  const [phase, setPhase] = useState<Phase>("loading");
  const [usingDefault, setUsingDefault] = useState(false);

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api
      .getAuthStatus()
      .then((s) => {
        if (cancelled) return;
        setUsingDefault(s.using_default);
        setPhase(!s.enabled || s.authenticated ? "authed" : "login");
      })
      // If the (open) status endpoint is unreachable, optimistically render the
      // app; any subsequent 401 will flip us to the login screen.
      .catch(() => !cancelled && setPhase("authed"));
    return () => {
      cancelled = true;
    };
  }, []);

  // Any API 401 anywhere re-prompts for login.
  useEffect(() => {
    const onUnauthorized = () => setPhase("login");
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
  }, []);

  const onSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setSubmitting(true);
      setError(null);
      try {
        const res = await api.login(username, password);
        setUsingDefault(res.using_default);
        setPassword("");
        setPhase("authed");
      } catch {
        setError("Invalid username or password");
      } finally {
        setSubmitting(false);
      }
    },
    [username, password],
  );

  if (phase === "loading") {
    return (
      <div className="flex h-screen w-full items-center justify-center text-[var(--color-text-muted)]">
        Loading…
      </div>
    );
  }

  if (phase === "login") {
    return (
      <div className="flex h-screen w-full items-center justify-center p-6">
        <form
          onSubmit={onSubmit}
          className="w-full max-w-sm rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-primary)] p-6 shadow-sm"
        >
          <h1 className="mb-1 text-xl font-semibold text-[var(--color-text-primary)]">Argus</h1>
          <p className="mb-5 text-sm text-[var(--color-text-muted)]">Sign in to continue</p>

          <label className="mb-3 block">
            <span className="mb-1 block text-xs font-medium text-[var(--color-text-secondary)]">Username</span>
            <input
              type="text"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-secondary)] px-3 py-2 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--color-accent)]"
              autoFocus
            />
          </label>

          <label className="mb-4 block">
            <span className="mb-1 block text-xs font-medium text-[var(--color-text-secondary)]">Password</span>
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg-secondary)] px-3 py-2 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--color-accent)]"
            />
          </label>

          {error && <p className="mb-3 text-sm text-red-500">{error}</p>}

          <button
            type="submit"
            disabled={submitting}
            className="w-full rounded-md bg-[var(--color-accent)] px-3 py-2 text-sm font-medium text-[var(--color-accent-text)] transition-opacity hover:opacity-90 disabled:opacity-50"
          >
            {submitting ? "Signing in…" : "Sign in"}
          </button>

          {usingDefault && (
            <p className="mt-4 text-center text-xs text-[var(--color-text-muted)]">
              Default credentials are <code>admin</code> / <code>argus</code>. Change them in Settings.
            </p>
          )}
        </form>
      </div>
    );
  }

  return <>{children}</>;
}
