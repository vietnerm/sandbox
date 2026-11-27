"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowLeft,
  Clipboard,
  Download,
  Eraser,
  FileText,
  Maximize2,
  Minimize2,
  Play,
  RotateCw,
  SquareTerminal,
  Terminal as TerminalIcon,
  X,
} from "lucide-react";
import "@xterm/xterm/css/xterm.css";
import { get, post, sse, type Sandbox as ApiSandbox } from "@/console/api";
import { getToken } from "@/console/minictd";
import { navigate as go } from "@/console/ui";
import { DemoShell } from "@/lib/demo-shell";
import { ExecShell } from "@/lib/exec-shell";
import { isDemo } from "@/console/api";
import { usePlatform } from "@/lib/platform-store";
import { cn } from "@/lib/utils";
import { Mono, Panel, SectionTitle, Segmented, StatusPill } from "../primitives";

/* Realtime terminal page per sandbox (/sandboxes/<id>/terminal).
   Tries the sandboxd PTY WebSocket first; when no live node answers
   (Next.js dev / demo), it falls back to an interactive local shell that
   shares the demo workspace with the Files manager. A Logs tab streams
   the container log via the same SSE contract. */

type Conn = "connecting" | "live" | "demo" | "closed";

const DEMO_QUICK_CMDS = ["ls -la", "tree", "df -h", "ps aux", "env", "git status", "grep -n error logs/app.log", "help"];
const EXEC_QUICK_CMDS = ["ls -la", "df -h", "ps", "env", "cat /etc/os-release", "uname -a", "free"];

const btn =
  "inline-flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40 disabled:pointer-events-none";

/* ── theme → xterm palette (resolved per connect, works in light/dark) ── */

function cssVar(name: string, fallback: string): string {
  if (typeof window === "undefined") return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

function hexA(hex: string, alpha: number): string {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex);
  if (!m) return hex;
  const n = parseInt(m[1], 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}

function termTheme() {
  const bg = cssVar("--sunken", "#0d1322");
  const neon = cssVar("--neon", "#00ff9d");
  return {
    background: bg,
    foreground: cssVar("--ink", "#e5e7eb"),
    cursor: neon,
    cursorAccent: bg,
    selectionBackground: hexA(neon, 0.25),
    black: bg,
    red: cssVar("--danger", "#ef4444"),
    green: cssVar("--neon", "#00ff9d"),
    yellow: cssVar("--warn", "#f59e0b"),
    blue: cssVar("--info", "#22d3ee"),
    magenta: "#c084fc",
    cyan: cssVar("--info", "#22d3ee"),
    white: "#e5e7eb",
    brightBlack: cssVar("--g500", "#6b7280"),
    brightRed: "#f87171",
    brightGreen: "#4ade80",
    brightYellow: "#fbbf24",
    brightBlue: "#7dd3fc",
    brightMagenta: "#d8b4fe",
    brightCyan: "#67e8f9",
    brightWhite: "#f9fafb",
  };
}

const ANSI_RE = /\x1b\[[0-9;?]*[A-Za-z]|\x1b\][^\x07]*\x07|\x1b(B)/g;

/* ── follow-along log stream (Logs tab) ─────────────────────────────── */

function lineClass(l: string): string {
  if (/\b(error|fail|failed|denied|blocked|refused|fatal|panic)\b/i.test(l)) return "text-danger";
  if (/\b(warn|warning|deprecated|retry|pending)\b/i.test(l)) return "text-warn";
  if (/^[$]/.test(l) || /sandboxd deploy/.test(l)) return "text-neon";
  if (/\b(ok|ready|started|listening|allowed|success|completed|running|live)\b/i.test(l) || l.trim().endsWith("ok")) return "text-neon/90";
  return "text-gray-300";
}

function LogsPanel({ id, running }: { id: string; running: boolean }) {
  const [lines, setLines] = useState<string[]>([]);
  const [follow, setFollow] = useState(true);
  const [err, setErr] = useState("");
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let live = true;
    setLines([]);
    setErr("");
    get<string>("/v1/sandboxes/" + id + "/logs?tail=200")
      .then((t) => live && setLines(typeof t === "string" && t ? t.replace(/\n$/, "").split("\n") : []))
      .catch((e) => live && setErr(String(e)));
    return () => {
      live = false;
    };
  }, [id]);

  useEffect(() => {
    if (!follow || !running) return;
    return sse(`/v1/sandboxes/${id}/logs?tail=0&follow=true`, (l) =>
      setLines((xs) => (xs.length > 500 ? [...xs.slice(-400), l] : [...xs, l])),
    );
  }, [id, follow, running]);

  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [lines, follow]);

  return (
    <Panel className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line px-4 py-2.5">
        <SectionTitle title="Container logs" vi="Nhật ký container — stream realtime" />
        <label className="flex cursor-pointer items-center gap-2 text-xs text-gray-400">
          <input type="checkbox" checked={follow} disabled={!running} onChange={(e) => setFollow(e.target.checked)} className="accent-[var(--neon)]" />
          Follow {follow && running ? <span className="size-1.5 rounded-full bg-neon dot-live" /> : null}
        </label>
      </div>
      <div ref={box} className="min-h-0 flex-1 overflow-y-auto bg-sunken px-4 py-3 font-mono text-[12px] leading-relaxed">
        {err && <p className="text-danger text-xs">{err}</p>}
        {!err && lines.length === 0 && <p className="text-gray-500 text-xs">No output yet…</p>}
        {lines.map((l, i) => (
          <div key={i} className={cn("whitespace-pre-wrap break-all", lineClass(l))}>
            {l}
          </div>
        ))}
        {follow && running && <span className="text-neon">▌</span>}
      </div>
    </Panel>
  );
}

/* ── the page ───────────────────────────────────────────────────────── */

export function TerminalView({ id }: { id: string }) {
  const sandbox = usePlatform((s) => s.sandboxes.find((b) => b.id === id));
  const setSandboxStatus = usePlatform((s) => s.setSandboxStatus);
  const me = usePlatform((s) => s.me);
  const canWrite = me?.role === "admin" || me?.role === "operator";

  const [tab, setTab] = useState<"shell" | "logs">("shell");
  const [conn, setConn] = useState<Conn>("connecting");
  const [full, setFull] = useState(false);
  const [fontSize, setFontSize] = useState(13);
  const [dims, setDims] = useState({ cols: 80, rows: 24 });
  const [tick, setTick] = useState(0); // +1 = restart session
  const [info, setInfo] = useState<ApiSandbox | null>(null);

  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<import("@xterm/xterm").Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const shellRef = useRef<DemoShell | ExecShell | null>(null);
  const fitRef = useRef<import("@xterm/addon-fit").FitAddon | null>(null);
  const transcriptRef = useRef<string>("");
  const liveWsRef = useRef(false);
  const ptySidRef = useRef<string | null>(null); // GĐ3 mục 2: PTY session id
  const ptyCleanupRef = useRef<(() => void) | null>(null);
  // POST input requests must be serialized; concurrent key chunks can arrive
  // out of order, which corrupts UTF-8/IME text in the PTY.
  const ptyInputQueueRef = useRef<Promise<void>>(Promise.resolve());
  // #218 (D84): minictd WebSocket PTY — binary stdin/stdout frames, one
  // connection per session; SSE+POST stays as the automatic fallback.
  const ptyWsRef = useRef<WebSocket | null>(null);

  const running = (sandbox?.status ?? "Running") === "Running";

  useEffect(() => {
    let live = true;
    get<ApiSandbox>(`/v1/sandboxes/${id}`)
      .then((d) => live && setInfo(d))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [id, sandbox?.status]);

  useEffect(() => {
    if (!full) return;
    const on = (e: KeyboardEvent) => e.key === "Escape" && setFull(false);
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, [full]);

  /* write helper shared by PTY + demo shell — keeps the transcript */
  const termWrite = useCallback((data: string) => {
    termRef.current?.write(data);
    if (transcriptRef.current.length > 2_000_000) transcriptRef.current = transcriptRef.current.slice(-1_000_000);
    transcriptRef.current += data.replace(ANSI_RE, "");
  }, []);

  /* send a line into whichever channel is active */
  const send = useCallback(
    (text: string, submit = true) => {
      const payload = submit ? text + "\r" : text;
      if (liveWsRef.current && wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(payload);
      else shellRef.current?.handleData(payload);
    },
    [],
  );

  /* ── xterm session boot (per id / tick / tab switch back to shell) ── */
  useEffect(() => {
    if (tab !== "shell") return;
    let disposed = false;
    let ro: ResizeObserver | null = null;
    let remeasure: ReturnType<typeof setTimeout> | undefined;
    setConn("connecting");
    transcriptRef.current = "";

    (async () => {
      const [{ Terminal }, { FitAddon }] = await Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit")]);
      if (disposed || !containerRef.current) return;

      /* xterm measures cell size from the current font; make sure the mono
         web font is loaded first or the scroll area desyncs from the rows */
      try {
        if (typeof document !== "undefined" && (document as Document).fonts) {
          await Promise.all([
            document.fonts.load('13px "JetBrains Mono Variable"'),
            document.fonts.load('600 13px "JetBrains Mono Variable"'),
          ]);
          await document.fonts.ready;
        }
      } catch {
        /* fonts API unavailable — proceed with fallback metrics */
      }
      if (disposed || !containerRef.current) return;

      const term = new Terminal({
        fontFamily: '"JetBrains Mono Variable", ui-monospace, SFMono-Regular, Menlo, monospace',
        fontSize,
        lineHeight: 1.25,
        cursorBlink: true,
        scrollback: 5000,
        convertEol: false,
        theme: termTheme(),
      });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.open(containerRef.current);
      try {
        fit.fit();
      } catch {
        /* container not measured yet */
      }
      termRef.current = term;
      fitRef.current = fit;
      setDims({ cols: term.cols, rows: term.rows });
      // debug hook (also handy for users' browser consoles)
      (window as unknown as Record<string, unknown>).__sbxTerm = term;

      term.onData((d) => {
        if (liveWsRef.current && wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(d);
        else if (ptyWsRef.current?.readyState === WebSocket.OPEN) {
          // #218: keystrokes/IME/paste go as BINARY frames — raw UTF-8 bytes,
          // one frame per chunk, zero HTTP round-trips (SSE+POST fallback
          // below keeps the #217 serial queue).
          try {
            ptyWsRef.current.send(new TextEncoder().encode(d));
          } catch {
            /* closing — fallback routing happens on close */
          }
        } else if (ptySidRef.current) {
          // GĐ3 mục 2: PTY mode — serialize raw input POSTs so IME/UTF-8
          // chunks cannot overtake one another on the network.
          const sid = ptySidRef.current;
          ptyInputQueueRef.current = ptyInputQueueRef.current
            .catch(() => {})
            .then(async () => {
              if (ptySidRef.current === sid) await post(`/v1/sandboxes/${id}/terminal/${sid}/input`, d);
            })
            .catch(() => {});
        } else shellRef.current?.handleData(d);
      });
      term.onResize(({ cols, rows }) => {
        setDims({ cols, rows });
        const ws = wsRef.current;
        if (liveWsRef.current && ws?.readyState === WebSocket.OPEN) {
          try {
            ws.send(JSON.stringify({ type: "resize", cols, rows }));
          } catch {
            /* ignore */
          }
        } else if (ptyWsRef.current?.readyState === WebSocket.OPEN) {
          try {
            ptyWsRef.current.send(JSON.stringify({ type: "resize", rows, cols }));
          } catch {
            /* ignore */
          }
        } else if (ptySidRef.current) {
          // GĐ3 mục 2: PTY resize
          post(`/v1/sandboxes/${id}/terminal/${ptySidRef.current}/resize`, { rows, cols }).catch(() => {});
        }
      });

      ro = new ResizeObserver(() => {
        requestAnimationFrame(() => {
          try {
            fitRef.current?.fit();
          } catch {
            /* hidden */
          }
        });
      });
      ro.observe(containerRef.current);

      /* late font swaps / first-layout rounding — re-measure once more */
      remeasure = setTimeout(() => {
        if (disposed || termRef.current !== term) return;
        try {
          fitRef.current?.fit();
          term.refresh(0, term.rows - 1);
        } catch {
          /* ignore */
        }
      }, 400);

      const startDemo = () => {
        if (disposed) return;
        liveWsRef.current = false;
        const project = sandbox?.name || "workspace";
        const shell = new DemoShell({
          sandboxId: id,
          project,
          image: sandbox?.image ?? info?.image ?? "4aicoding/base:node22",
          egressMode: info?.egress_mode,
          memLimitMb: sandbox?.memGb ? sandbox.memGb * 1024 : undefined,
          write: termWrite,
          onExit: () => setConn("closed"),
        });
        shellRef.current = shell;
        shell.banner();
        setConn("demo");
      };

      if (!running) {
        setConn("closed");
        termWrite(`\x1b[90m— sandbox is ${sandbox?.status ?? "stopped"} — start it to open a shell —\x1b[0m\r\n`);
        return;
      }

      /* minictd: try PTY terminal endpoint first (GĐ3 mục 2, issue #189);
         fall back to line-mode ExecShell if the endpoint is unavailable.
         #206 (d) two-layer fallback: a 200 from POST /terminal is no longer
         trusted on its own — if the stream errors or closes BEFORE the first
         frame (session died between start and stream, the owner-reported
         bug), switch to line-mode with a visible notice instead of leaving
         a blank terminal. */
      /* #218 (D84): attach the minictd WebSocket PTY — binary frames both
         directions, auth token as the FIRST text frame (browsers cannot set
         Authorization on a WS handshake). Resolves true only after the
         server's {"type":"ready"}; any failure before that falls back. */
      const attachPtyWs = (term: import("@xterm/xterm").Terminal): Promise<boolean> =>
        new Promise((resolve) => {
          let settled = false;
          let ready = false;
          let opened = false;
          let ws: WebSocket | null = null;
          const proto = location.protocol === "https:" ? "wss" : "ws";
          try {
            ws = new WebSocket(`${proto}://${location.host}/v1/sandboxes/${id}/terminal/ws`);
          } catch {
            resolve(false);
            return;
          }
          ws.binaryType = "arraybuffer";
          const bail = (notice: string) => {
            if (settled) return;
            settled = true;
            if (notice) termWrite(notice);
            try {
              ws?.close();
            } catch {
              /* ignore */
            }
            ptyWsRef.current = null;
            resolve(false);
          };
          const openTimer = window.setTimeout(() => bail("\x1b— WS unavailable (open timeout 2s) — fallback to SSE —\x1b\r\n"), 2000);
          ws.onopen = () => {
            opened = true;
            window.clearTimeout(openTimer);
            try {
              ws?.send(JSON.stringify({ type: "auth", token: getToken() }));
            } catch {
              bail("");
            }
          };
          ws.onmessage = (ev) => {
            if (typeof ev.data === "string") {
              let msg: { type?: string; code?: number; msg?: string };
              try {
                msg = JSON.parse(ev.data);
              } catch {
                return;
              }
              if (msg.type === "ready") {
                ready = true;
                settled = true;
                ptyWsRef.current = ws;
                termWrite(`\x1b— PTY attached (WebSocket binary, ${term.cols}×${term.rows}) —\x1b\r\n`);
                try {
                  ws?.send(JSON.stringify({ type: "resize", rows: term.rows, cols: term.cols }));
                } catch {
                  /* initial resize is cosmetic; onResize covers the rest */
                }
                resolve(true);
                return;
              }
              if (msg.type === "exit") {
                if (ptyWsRef.current === ws) ptyWsRef.current = null;
                termWrite(`\r\n\x1b— PTY exited (code ${msg.code ?? "?"}) — click Restart session —\x1b\r\n`);
                setConn("closed");
                try {
                  ws?.close();
                } catch {
                  /* ignore */
                }
                return;
              }
              if (msg.type === "error") {
                if (!ready) bail(`\x1b— WS PTY error: ${msg.msg ?? "?"} — fallback to SSE —\x1b\r\n`);
                else termWrite(`\r\n\x1b— PTY error: ${msg.msg ?? "?"} —\x1b\r\n`);
                return;
              }
              return;
            }
            // binary frame = raw PTY output bytes — written as bytes, never
            // decoded to string (UTF-8 boundary-safe across chunks)
            term.write(new Uint8Array(ev.data as ArrayBuffer));
          };
          ws.onclose = () => {
            window.clearTimeout(openTimer);
            if (!ready) {
              bail(opened ? "\x1b— WS closed before ready — fallback to SSE —\x1b\r\n" : "");
              return;
            }
            if (ptyWsRef.current === ws) ptyWsRef.current = null;
            termWrite(`\r\n\x1b— PTY disconnected — click Restart session —\x1b\r\n`);
            setConn("closed");
          };
          ws.onerror = () => {
            if (!ready) bail("");
          };
        });

      if (!isDemo()) {
        // ── #218 (D84): try the WebSocket PTY first ─────────────────────
        // Open ≤ 2 s + auth frame + ready text; ANY failure before `ready`
        // (open timeout, 4401, close) falls through to the SSE+POST path
        // below with a visible notice. After `ready` the WS owns the
        // session until close/exit.
        const wsOk = await attachPtyWs(term);
        if (wsOk) {
          setConn("live");
          return;
        }
        try {
          const { session_id: sid } = await post<{ session_id: string }>(`/v1/sandboxes/${id}/terminal`);
          if (sid) {
            ptySidRef.current = sid;
            ptyInputQueueRef.current = Promise.resolve();
            setConn("live");
            let gotFirstFrame = false;
            const stop = sse(
              `/v1/sandboxes/${id}/terminal/${sid}/stream`,
              (data) => { gotFirstFrame = true; termWrite(data); },
              () => {
                if (disposed) return;
                // session is gone either way — stop routing input to it
                ptyCleanupRef.current = null;
                ptySidRef.current = null;
                if (!gotFirstFrame) {
                  termWrite(`\x1b[33m— PTY unavailable (session died before output) — switched to line-mode —\x1b[0m\r\n`);
                  const shell = new ExecShell({ sandboxId: id, write: termWrite, onExit: () => setConn("closed") });
                  shellRef.current = shell;
                  shell.banner();
                  setConn("live");
                  return;
                }
                setConn("closed");
              },
            );
            ptyCleanupRef.current = stop;
            // initial resize so the PTY knows the terminal size
            post(`/v1/sandboxes/${id}/terminal/${sid}/resize`, { rows: term.rows, cols: term.cols }).catch(() => {});
            return;
          }
        } catch {
          /* PTY endpoint unavailable — fall through to line-mode */
        }
        // Fall back to line-mode (ExecShell)
        const shell = new ExecShell({ sandboxId: id, write: termWrite, onExit: () => setConn("closed") });
        shellRef.current = shell;
        shell.banner();
        setConn("live");
        return;
      }

      /* 1) try the real PTY WebSocket on the sandboxd origin */
      let opened = false;
      let ws: WebSocket | null = null;
      try {
        const proto = location.protocol === "https:" ? "wss" : "ws";
        ws = new WebSocket(`${proto}://${location.host}/v1/sandboxes/${id}/pty`);
        wsRef.current = ws;
        const guard = setTimeout(() => {
          if (!opened && wsRef.current) {
            try {
              wsRef.current.close();
            } catch {
              /* already down */
            }
          }
        }, 1200);
        ws.onopen = () => {
          opened = true;
          clearTimeout(guard);
          if (disposed) return;
          liveWsRef.current = true;
          setConn("live");
          termWrite(`\x1b[90m— PTY attached (${term.cols}×${term.rows}) —\x1b[0m\r\n`);
          try {
            ws?.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
          } catch {
            /* ignore */
          }
        };
        ws.onmessage = (ev) => {
          const d = ev.data;
          if (typeof d === "string") termWrite(d);
          else if (d instanceof Blob) d.text().then(termWrite);
          else if (d instanceof ArrayBuffer) termWrite(new TextDecoder().decode(d));
        };
        ws.onclose = () => {
          if (!opened) {
            startDemo(); // handshake never completed → demo session
            return;
          }
          liveWsRef.current = false;
          setConn("closed");
          termWrite(`\r\n\x1b[90m— PTY disconnected — click Restart session —\x1b[0m\r\n`);
        };
        ws.onerror = () => {
          if (!opened) {
            try {
              ws?.close();
            } catch {
              /* ignore */
            }
          }
        };
      } catch {
        startDemo();
      }
    })();

    return () => {
      disposed = true;
      ro?.disconnect();
      clearTimeout(remeasure);
      liveWsRef.current = false;
      try {
        wsRef.current?.close();
      } catch {
        /* ignore */
      }
      wsRef.current = null;
      // GĐ3 mục 2: cleanup PTY session (abort SSE stream)
      ptyCleanupRef.current?.();
      ptyCleanupRef.current = null;
      ptySidRef.current = null;
      // #218: close the WS PTY — {"type":"close"} text then TCP close; the
      // daemon SIGHUPs + reaps the PTY and removes the env-file either way.
      const pws = ptyWsRef.current;
      ptyWsRef.current = null;
      if (pws) {
        try {
          if (pws.readyState === WebSocket.OPEN) pws.send(JSON.stringify({ type: "close" }));
        } catch {
          /* ignore */
        }
        try {
          pws.close();
        } catch {
          /* ignore */
        }
      }
      shellRef.current = null;
      termRef.current?.dispose();
      termRef.current = null;
    };
    // fontSize intentionally excluded — applied separately below
  }, [id, tick, tab, running]);

  /* font size changes re-fit without a new session */
  useEffect(() => {
    const t = termRef.current;
    if (!t) return;
    t.options.fontSize = fontSize;
    requestAnimationFrame(() => {
      try {
        fitRef.current?.fit();
      } catch {
        /* ignore */
      }
    });
  }, [fontSize]);

  const restartSession = () => setTick((t) => t + 1);

  const clearTerminal = () => {
    termRef.current?.clear();
    termRef.current?.focus();
  };

  const downloadTranscript = () => {
    const blob = new Blob([transcriptRef.current], { type: "text/plain;charset=utf-8" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `terminal-${id}.log`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  const startSandbox = () => {
    setSandboxStatus(id, "Starting");
    setTimeout(() => setSandboxStatus(id, "Running"), 1500);
  };

  const name = sandbox?.name ?? info?.external?.project_id ?? id;
  const connMeta: Record<Conn, { label: string; cls: string; dot: string }> = {
    connecting: { label: "connecting", cls: "text-warn", dot: "bg-warn" },
    live: { label: isDemo() ? "PTY attached" : "exec attached", cls: "text-neon", dot: "bg-neon dot-live" },
    demo: { label: "demo session", cls: "text-info", dot: "bg-info dot-live" },
    closed: { label: "session closed", cls: "text-gray-500", dot: "bg-gray-500" },
  };
  const cm = connMeta[conn];

  return (
    <div className={cn("flex flex-col gap-4", full && "fixed inset-0 z-[70] bg-navy p-3 md:p-5")}>
      {/* ── header ───────────────────────────────────────────────── */}
      <div className={cn("flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between", full && "gap-2")}>
        <div className="flex min-w-0 flex-wrap items-center gap-2.5">
          {!full && (
            <button
              onClick={() => go(`/sandboxes/${id}`)}
              className="grid size-9 shrink-0 place-items-center rounded-lg border border-line text-gray-400 outline-none transition-colors hover:bg-fg/5 hover:text-gray-200 focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Back to sandbox"
            >
              <span className="text-sm">←</span>
            </button>
          )}
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className={cn("truncate font-bold tracking-tight text-ink", full ? "text-lg" : "text-[24px]")}>
                <SquareTerminal className="mr-2 inline size-5 text-neon" />
                Terminal
              </h1>
              <Mono className="rounded-md border border-line bg-surface px-2 py-1 text-xs text-info">{name}</Mono>
              <span className="rounded-md border border-line bg-surface px-2.5 py-1.5">
                <StatusPill status={sandbox?.status ?? "Running"} />
              </span>
            </div>
            {!full && (
              <Mono className="mt-1 block text-[11px] text-gray-500">
                {sandbox?.image ?? "—"} · node {sandbox?.host ?? "local"} · /sandboxes/{id}/terminal
              </Mono>
            )}
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Segmented
            size="sm"
            options={[
              { value: "shell", label: "Shell" },
              { value: "logs", label: "Logs" },
            ]}
            value={tab}
            onChange={setTab}
          />
          <span className={cn("inline-flex items-center gap-1.5 rounded-md border border-line bg-surface px-2.5 py-1.5 font-mono text-[11px]", cm.cls)}>
            <span className={cn("size-1.5 rounded-full", cm.dot)} /> {cm.label}
          </span>
          {tab === "shell" && (
            <>
              <span className="hidden items-center gap-1 md:inline-flex">
                <button onClick={() => setFontSize((f) => Math.max(10, f - 1))} className={cn(btn, "border-line bg-surface px-2 py-1.5 text-gray-400 hover:bg-fg/5")} aria-label="Smaller font">
                  A-
                </button>
                <button onClick={() => setFontSize((f) => Math.min(20, f + 1))} className={cn(btn, "border-line bg-surface px-2 py-1.5 text-gray-400 hover:bg-fg/5")} aria-label="Bigger font">
                  A+
                </button>
              </span>
              <button onClick={clearTerminal} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")} title="Clear scrollback">
                <Eraser className="size-3.5" /> <span className="hidden sm:inline">Clear</span>
              </button>
              <button onClick={downloadTranscript} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")} title="Download transcript">
                <Download className="size-3.5" /> <span className="hidden sm:inline">Transcript</span>
              </button>
            </>
          )}
          <button onClick={restartSession} className={cn(btn, "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")} disabled={conn === "connecting"}>
            <RotateCw className={cn("size-3.5", conn === "connecting" && "animate-spin")} /> <span className="hidden sm:inline">Restart</span>
          </button>
          <button onClick={() => setFull((f) => !f)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")} aria-label="Toggle fullscreen">
            {full ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
          </button>
          {full && (
            <button onClick={() => go(`/sandboxes/${id}`)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
              <X className="size-3.5" /> Exit
            </button>
          )}
        </div>
      </div>

      {/* ── body ─────────────────────────────────────────────────── */}
      {tab === "shell" ? (
        <Panel className={cn("flex min-h-0 flex-col overflow-hidden", full ? "flex-1" : "")}>
          {/* quick commands — also the mobile keyboard workaround */}
          <div className="flex flex-wrap items-center gap-1.5 border-b border-line px-3 py-2">
            <Mono className="mr-1 hidden text-[10px] text-gray-600 sm:block">quick:</Mono>
            {(isDemo() ? DEMO_QUICK_CMDS : EXEC_QUICK_CMDS).map((c) => (
              <button
                key={c}
                onClick={() => {
                  send(c);
                  termRef.current?.focus();
                }}
                disabled={conn !== "demo" && conn !== "live"}
                className="rounded-md border border-line bg-navy/60 px-2 py-1 font-mono text-[11px] text-gray-400 outline-none transition-colors hover:border-neon/40 hover:text-neon focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40"
              >
                {c}
              </button>
            ))}
            <button
              onClick={() => {
                send("", true);
                navigator.clipboard
                  ?.readText()
                  .then((t) => {
                    if (t) send(t, true);
                  })
                  .catch(() => {});
                termRef.current?.focus();
              }}
              disabled={conn !== "demo" && conn !== "live"}
              className="ml-auto inline-flex items-center gap-1.5 rounded-md border border-line bg-navy/60 px-2 py-1 font-mono text-[11px] text-gray-400 outline-none transition-colors hover:border-info/40 hover:text-info focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40"
              title="Paste from clipboard and run"
            >
              <Clipboard className="size-3" /> paste run
            </button>
          </div>

          {/* the terminal surface */}
          <div className={cn("relative bg-sunken", full ? "min-h-0 flex-1" : "h-[420px] md:h-[520px]")}>
            <div ref={containerRef} className="absolute inset-0 px-2 py-1.5" onClick={() => termRef.current?.focus()} />
            {conn === "connecting" && (
              <div className="pointer-events-none absolute inset-0 grid place-items-center">
                <div className="flex items-center gap-2 font-mono text-xs text-gray-500">
                  <RotateCw className="size-3.5 animate-spin" /> attaching PTY…
                </div>
              </div>
            )}
            {!running && (
              <div className="absolute inset-0 grid place-items-center bg-navy/70 backdrop-blur-sm">
                <div className="max-w-sm text-center">
                  <TerminalIcon className="mx-auto size-8 text-gray-500" />
                  <p className="mt-3 text-sm text-gray-300">
                    Sandbox is <span className="text-warn">{sandbox?.status ?? "stopped"}</span> — the shell needs a running container.
                  </p>
                  <p className="mt-1 text-xs text-gray-500">Môi trường đang tắt — khởi động để mở terminal.</p>
                  {canWrite && (
                    <button
                      onClick={startSandbox}
                      className="mt-4 inline-flex items-center gap-2 rounded-lg bg-neon px-4 py-2.5 text-sm font-semibold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_20px_var(--glow-mid)]"
                    >
                      <Play className="size-4" /> Start sandbox
                    </button>
                  )}
                </div>
              </div>
            )}
            {conn === "closed" && running && (
              <div className="pointer-events-none absolute bottom-3 right-4 rounded-md border border-line bg-surface/90 px-3 py-1.5 font-mono text-[11px] text-gray-400">
                session closed · press Restart to reconnect
              </div>
            )}
          </div>

          {/* status bar */}
          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line px-4 py-2 font-mono text-[10px] text-gray-600">
            <span>
              {conn === "live" ? (isDemo() ? "pty" : "exec") : conn === "demo" ? "demo-shell" : conn} · {dims.cols}×{dims.rows} · scrollback 5000
            </span>
            <span className="hidden sm:block">
              {isDemo() ? (<>↑/↓ history · Tab completes · Ctrl+C interrupt · Ctrl+L clear — <span className="text-gray-500">files shared with the Files tab</span></>) : (<>↑/↓ history · Ctrl+L clear · timeout 60s / lệnh</>)}
            </span>
          </div>
        </Panel>
      ) : (
        <div className={cn("flex min-h-0 flex-col", full ? "flex-1" : "h-[460px] md:h-[560px]")}>
          <LogsPanel id={id} running={running} />
        </div>
      )}

      {!full && isDemo() && (
        <p className="flex items-center gap-2 text-xs text-gray-500">
          <FileText className="size-3.5" />
          Workspace files live in the <button onClick={() => go(`/sandboxes/${id}/files`)} className="text-info hover:underline">Files manager</button> — edits appear here instantly.
        </p>
      )}
    </div>
  );
}
