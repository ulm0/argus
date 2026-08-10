"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";

interface Toast {
  id: number;
  msg: string;
  ok: boolean;
}

interface ToastApi {
  /** Show a transient message. Success toasts self-dismiss; errors stay until dismissed. */
  showToast: (msg: string, ok?: boolean) => void;
}

const ToastContext = createContext<ToastApi>({ showToast: () => {} });

/** useToast returns the app-wide toast dispatcher. Safe to call outside a provider (no-op). */
export function useToast(): ToastApi {
  return useContext(ToastContext);
}

const SUCCESS_MS = 3000;

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);
  // Timer ids are tracked so dismissing early — or unmounting — cannot leave a
  // setState scheduled against a gone component.
  const timers = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map());

  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timers.current.delete(id);
    }
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const showToast = useCallback(
    (msg: string, ok = true) => {
      const id = nextId.current++;
      setToasts((prev) => [...prev, { id, msg, ok }]);
      // Errors persist: on a phone in the sun a 3s failure message is the same
      // as no message at all, and failures are the ones worth reading.
      if (ok) {
        timers.current.set(
          id,
          setTimeout(() => {
            timers.current.delete(id);
            setToasts((prev) => prev.filter((t) => t.id !== id));
          }, SUCCESS_MS),
        );
      }
    },
    [],
  );

  useEffect(() => {
    const pending = timers.current;
    return () => {
      pending.forEach(clearTimeout);
      pending.clear();
    };
  }, []);

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      <div
        role="status"
        aria-live="polite"
        className="pointer-events-none fixed bottom-[max(1.5rem,env(safe-area-inset-bottom))] left-1/2 z-50 flex w-[min(24rem,calc(100vw-2rem))] -translate-x-1/2 flex-col items-center gap-2"
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            role={t.ok ? undefined : "alert"}
            className={`pointer-events-auto flex w-full items-start gap-3 rounded-sm px-4 py-3 text-sm font-medium shadow-lg ${
              t.ok
                ? "bg-[var(--color-success)] text-white"
                : "bg-[var(--color-danger)] text-white"
            }`}
          >
            <span className="min-w-0 flex-1 break-words">{t.msg}</span>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              aria-label="Dismiss"
              className="-m-1 shrink-0 p-1 text-white/70 transition-colors hover:text-white"
            >
              <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
                <line x1={18} y1={6} x2={6} y2={18} />
                <line x1={6} y1={6} x2={18} y2={18} />
              </svg>
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
