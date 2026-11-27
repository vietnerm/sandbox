"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  ChevronRight,
  Download,
  File as FileIcon,
  FileCode2,
  FileJson,
  FileText,
  FilePlus2,
  Folder,
  FolderOpen,
  FolderPlus,
  ListTree,
  Pencil,
  RefreshCw,
  RotateCcw,
  SquarePen,
  Trash2,
  Upload,
} from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import {
  type FsEntry,
  createFsEntry,
  deleteFsEntry,
  listFiles,
  moveFsEntry,
  readFile,
  resetFs,
  uploadFsFile,
  writeFile,
} from "@/console/api";
import { navigate as go } from "@/console/ui";
import { usePlatform } from "@/lib/platform-store";
import { cn } from "@/lib/utils";
import {
  assertSafeName,
  b64ToBytes,
  crumbsFromB64,
  decodeFsName,
  encodePathB64,
  joinPathB64,
  parentB64,
} from "@/lib/files-parse";
import { Mono, Panel, SectionTitle } from "../primitives";

/* FTP-style workspace manager per sandbox (/sandboxes/<id>/files).
   Browse the container filesystem, open text files in an editor and
   save straight back into the sandbox — plus create / rename / delete /
   upload / download. In demo mode this is the same in-browser workspace
   the terminal's demo shell operates on, so both stay in sync.

   #219 (D83): every path is handled in its base64-of-raw-bytes wire form
   (path_b64) — display strings are derivates and never used as paths, so
   filenames that are not valid UTF-8 stay openable/renamable/deletable. */

const btn =
  "inline-flex items-center justify-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40 disabled:pointer-events-none";

const ROOT_B64 = "Lw=="; // base64("/") — the listing boot path

/** ops path of an entry: the real bytes when present, else the (always
 *  valid) demo display path encoded on the fly. */
function entryB64(e: FsEntry): string {
  return e.path_b64 ?? encodePathB64(e.path);
}

/** display form of an entry name (valid → text; else placeholder + hex). */
function entryName(e: FsEntry): { display: string; valid: boolean; hex: string } {
  if (e.name_b64) return decodeFsName(e.name_b64);
  return { display: e.name, valid: true, hex: "" };
}

/** BYTE-level prefix test (#219): base64 strings have no prefix property,
 *  so `current` is under `dir` only if the decoded bytes say so. */
function b64IsPrefix(dirB64: string, pathB64: string): boolean {
  const d = b64ToBytes(dirB64);
  const p = b64ToBytes(pathB64);
  if (p.length < d.length) return false;
  for (let i = 0; i < d.length; i++) if (d[i] !== p[i]) return false;
  return d.length === p.length || d[d.length - 1] === 0x2f || p[d.length] === 0x2f;
}

function fmtBytes(n: number): string {
  if (!n) return "0 B";
  const u = ["B", "KiB", "MiB", "GiB"];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${u[i]}`;
}

function fmtMtime(ms: number): string {
  const d = new Date(ms);
  return `${d.toLocaleDateString("en", { month: "short", day: "2-digit" })} ${d.toLocaleTimeString("en", { hour: "2-digit", minute: "2-digit" })}`;
}

function modeRwx(mode: string, isDir: boolean): string {
  const bit = (c: string) => (c === "0" ? "---" : c === "4" ? "r--" : c === "5" ? "r-x" : c === "6" ? "rw-" : c === "7" ? "rwx" : "---");
  const [u, g, o] = (mode || "644").padStart(3, "0").split("");
  return `${isDir ? "d" : "-"}${bit(u)}${bit(g)}${bit(o)}`;
}

function FileGlyph({ entry, className }: { entry: FsEntry; className?: string }) {
  if (entry.type === "dir") return <Folder className={cn("size-4 shrink-0 text-info", className)} />;
  const n = entry.name.toLowerCase();
  const cls = cn("size-4 shrink-0 text-gray-400", className);
  if (/\.(ts|tsx|js|jsx|py|rs|go|sh|c|h|cpp|java|rb)$/.test(n)) return <FileCode2 className={cn(cls, "text-neon/80")} />;
  if (/\.(json|toml|yaml|yml|lock|env|gitignore|cfg|ini)$/.test(n) || /^\./.test(n)) return <FileJson className={cn(cls, "text-warn/80")} />;
  if (/\.(md|txt|log|csv|ipynb)$/.test(n)) return <FileText className={cn(cls, "text-info/80")} />;
  return <FileIcon className={cls} />;
}

/** parent chain for the breadcrumb — BYTE level (#219): the path travels
 *  as base64 and segments may be invalid UTF-8; labels are best-effort. */
function crumbs(pathB64: string, homeB64: string): Array<{ label: string; path: string }> {
  const all = crumbsFromB64(pathB64);
  const homeCrumbs = crumbsFromB64(homeB64);
  const underHome =
    homeCrumbs.length > 0 &&
    homeCrumbs.length <= all.length &&
    homeCrumbs.every((c, i) => all[i].b64 === c.b64);
  if (underHome) {
    // one crumb for the workspace root, then the segments under it
    const homeLabel = homeCrumbs[homeCrumbs.length - 1].label || homeCrumbs[homeCrumbs.length - 1].b64;
    return [
      { label: homeLabel === "/" ? "/" : homeLabel, path: homeB64 },
      ...all.slice(homeCrumbs.length).map((c) => ({ label: c.label, path: c.b64 })),
    ];
  }
  return all.map((c) => ({ label: c.label, path: c.b64 }));
}

/* ── lazy directory tree (left pane / mobile drawer) ───────────────── */

function TreeBranch({
  id,
  path,
  name,
  depth,
  current,
  expanded,
  kids,
  onToggle,
  onEnter,
}: {
  id: string;
  /** base64 of this branch path (#219) */
  path: string;
  /** best-effort display label */
  name: string;
  depth: number;
  current: string;
  expanded: Set<string>;
  kids: Record<string, FsEntry[]>;
  onToggle: (p: string) => void;
  onEnter: (p: string) => void;
}) {
  const isOpen = expanded.has(path);
  const children = kids[path];
  const active = b64IsPrefix(path, current);
  return (
    <div>
      <div
        className={cn(
          "group flex items-center gap-1 rounded-md pr-1 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
          active ? "bg-neon/10 text-neon" : "text-gray-300 hover:bg-fg/5",
        )}
        style={{ paddingLeft: depth * 14 + 4 }}
      >
        <button
          onClick={() => onToggle(path)}
          className="grid size-6 shrink-0 place-items-center rounded outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={isOpen ? `Collapse ${name}` : `Expand ${name}`}
          aria-expanded={isOpen}
        >
          <ChevronRight className={cn("size-3.5 text-gray-500 transition-transform", isOpen && "rotate-90")} />
        </button>
        <button onClick={() => onEnter(path)} className="flex min-w-0 flex-1 items-center gap-2 py-1.5 text-left">
          {isOpen ? <FolderOpen className="size-3.5 shrink-0 text-info" /> : <Folder className="size-3.5 shrink-0 text-info/80" />}
          <span className="truncate text-[13px]">{name}</span>
        </button>
      </div>
      {isOpen &&
        children &&
        children
          .filter((e) => e.type === "dir")
          .map((e) => (
            <TreeBranch
              key={entryB64(e)}
              id={id}
              path={entryB64(e)}
              name={entryName(e).display}
              depth={depth + 1}
              current={current}
              expanded={expanded}
              kids={kids}
              onToggle={onToggle}
              onEnter={onEnter}
            />
          ))}
      {isOpen && children && children.filter((e) => e.type === "dir").length === 0 && (
        <div style={{ paddingLeft: depth * 14 + 30 }} className="py-1 text-[11px] text-gray-600">
          empty
        </div>
      )}
      {isOpen && children === undefined && (
        <div style={{ paddingLeft: depth * 14 + 30 }} className="py-1 font-mono text-[11px] text-gray-600">
          loading…
        </div>
      )}
    </div>
  );
}

/* ── the page ───────────────────────────────────────────────────────── */

type SortKey = "name" | "size" | "mtime";

export function FileManagerView({ id }: { id: string }) {
  const sandbox = usePlatform((s) => s.sandboxes.find((b) => b.id === id));
  const me = usePlatform((s) => s.me);
  const canWrite = me?.role === "admin" || me?.role === "operator";

  // #219 (D83): the navigation state is base64-of-raw-bytes (source of
  // truth); the display strings are derivates kept only for the status bar.
  const [pathB64, setPathB64] = useState(ROOT_B64);
  const [path, setPath] = useState("/");
  const [homeB64, setHomeB64] = useState(ROOT_B64);
  const [home, setHome] = useState("/");
  const [entries, setEntries] = useState<FsEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [live, setLive] = useState(false);
  const [sort, setSort] = useState<SortKey>("name");
  const [showHidden, setShowHidden] = useState(false);
  const [treeOpen, setTreeOpen] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [kids, setKids] = useState<Record<string, FsEntry[]>>({});
  const [dragOver, setDragOver] = useState(false);

  // editor
  const [ed, setEd] = useState<{ pathB64: string; pathDisplay: string; content: string; orig: string; encoding: "utf8" | "base64"; size: number; mode: string; mtime: number } | null>(null);
  const [saving, setSaving] = useState(false);

  // prompts — rename carries the SOURCE path as base64 (#219 recovery)
  const [prompt, setPrompt] = useState<{ kind: "file" | "dir" | "rename"; value: string; fromB64?: string } | null>(null);

  const uploadRef = useRef<HTMLInputElement>(null);
  // Ignore out-of-order list responses when navigation/refresh is fast.
  const loadSeqRef = useRef(0);
  const readSeqRef = useRef(0);

  const sorted = useMemo(() => {
    const list = entries.filter((e) => showHidden || !(entryName(e).display.startsWith(".") || e.name.startsWith(".")));
    list.sort((a, b) => {
      if (a.type !== b.type) return a.type === "dir" ? -1 : 1;
      if (sort === "size") return b.size - a.size;
      if (sort === "mtime") return b.mtime - a.mtime;
      return entryName(a).display.localeCompare(entryName(b).display);
    });
    return list;
  }, [entries, sort, showHidden]);

  const load = useCallback(
    async (p: string) => {
      const seq = ++loadSeqRef.current;
      readSeqRef.current += 1;
      setLoading(true);
      setError("");
      try {
        const r = await listFiles(id, p);
        if (seq !== loadSeqRef.current) return;
        setEntries(r.entries);
        setPathB64(r.path_b64 ?? encodePathB64(r.path));
        setPath(r.path);
        if (r.home) {
          setHome(r.home);
          setHomeB64(r.home_b64 ?? encodePathB64(r.home));
        }
      } catch (e) {
        if (seq === loadSeqRef.current) setError(e instanceof Error ? e.message : String(e));
      } finally {
        if (seq === loadSeqRef.current) setLoading(false);
      }
    },
    [id],
  );

  /* boot: list / to learn `home`, then jump into the workspace */
  useEffect(() => {
    let dead = false;
    (async () => {
      try {
        const r = await listFiles(id, ROOT_B64);
        if (dead) return;
        setHome(r.home);
        setHomeB64(r.home_b64 ?? encodePathB64(r.home));
        await load(r.home_b64 ?? encodePathB64(r.home));
      } catch {
        if (!dead) await load(ROOT_B64);
      }
    })();
    return () => {
      dead = true;
    };
  }, [id, load]);

  /* live re-poll of the current dir */
  useEffect(() => {
    if (!live) return;
    const pollSeq = loadSeqRef.current;
    const t = setInterval(() => {
      listFiles(id, pathB64)
        .then((r) => {
          if (pollSeq !== loadSeqRef.current) return;
          setEntries(r.entries);
          setPathB64(r.path_b64 ?? encodePathB64(r.path));
          setPath(r.path);
        })
        .catch(() => {});
    }, 4000);
    return () => clearInterval(t);
  }, [live, id, pathB64]);

  /* tree expansion loads children */
  const toggleTree = useCallback(
    (p: string) => {
      setExpanded((xs) => {
        const nx = new Set(xs);
        if (nx.has(p)) nx.delete(p);
        else nx.add(p);
        return nx;
      });
      if (!kids[p]) {
        listFiles(id, p)
          .then((r) => setKids((k) => ({ ...k, [p]: r.entries })))
          .catch(() => setKids((k) => ({ ...k, [p]: [] })));
      }
    },
    [id, kids],
  );

  const enter = useCallback(
    (p: string) => {
      void load(p);
      setTreeOpen(false);
    },
    [load],
  );

  const openFile = useCallback(
    async (e: FsEntry) => {
      const seq = ++readSeqRef.current;
      try {
        const r = await readFile(id, entryB64(e));
        if (seq !== readSeqRef.current) return;
        setEd({
          pathB64: r.path_b64 ?? entryB64(e),
          pathDisplay: r.path,
          content: r.content,
          orig: r.content,
          encoding: r.encoding,
          size: r.size,
          mode: r.mode,
          mtime: r.mtime,
        });
      } catch (err) {
        if (seq === readSeqRef.current) setError(err instanceof Error ? err.message : String(err));
      }
    },
    [id],
  );

  const saveFile = useCallback(async () => {
    if (!ed || ed.encoding === "base64") return;
    setSaving(true);
    try {
      await writeFile(id, ed.pathB64, ed.content);
      setEd({ ...ed, orig: ed.content, mtime: Date.now() });
      await load(pathB64);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }, [ed, id, load, pathB64]);

  /* Ctrl+S in the editor */
  useEffect(() => {
    if (!ed) return;
    const on = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        if (ed.content !== ed.orig) void saveFile();
      }
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, [ed, saveFile]);

  const download = useCallback(
    async (e: FsEntry) => {
      if (e.type === "dir") return;
      try {
        const r = await readFile(id, entryB64(e));
        let blob: Blob;
        if (r.encoding === "base64") {
          const bin = atob(r.content);
          const arr = new Uint8Array(bin.length);
          for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
          blob = new Blob([arr], { type: "application/octet-stream" });
        } else {
          blob = new Blob([r.content], { type: "text/plain;charset=utf-8" });
        }
        const a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        const n = entryName(e);
        // display name for valid names; a safe ASCII fallback otherwise
        a.download = n.valid && n.display ? n.display : "invalid-utf8.bin";
        a.click();
        URL.revokeObjectURL(a.href);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      }
    },
    [id],
  );

  const remove = useCallback(
    async (e: FsEntry) => {
      const n = entryName(e);
      if (!confirm(`Delete ${e.type === "dir" ? "folder" : "file"} ${n.valid ? n.display : n.hex || "(invalid UTF-8 name)"}? (recursively — the demo workspace can be reseeded)`)) return;
      try {
        await deleteFsEntry(id, entryB64(e));
        await load(pathB64);
        setKids((k) => {
          const nk: Record<string, FsEntry[]> = {};
          // byte-level subtree prune (#219): b64 strings are not prefix-ordered
          const gone = b64ToBytes(entryB64(e));
          for (const [p, v] of Object.entries(k)) {
            const pb = b64ToBytes(p);
            const under = pb.length > gone.length && gone.every((b, i) => pb[i] === b) && pb[gone.length] === 0x2f;
            if (!under) nk[p] = v;
          }
          return nk;
        });
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      }
    },
    [id, load, pathB64],
  );

  /* keep the tree cache of `p` in sync */
  const refreshKids = useCallback(
    (p: string) => {
      listFiles(id, p)
        .then((r) => setKids((k) => ({ ...k, [p]: r.entries })))
        .catch(() => {});
    },
    [id],
  );

  const submitPrompt = useCallback(async () => {
    if (!prompt) return;
    const name = prompt.value.trim();
    if (!name) return;
    try {
      if (prompt.kind === "rename" && prompt.fromB64) {
        // #219: source = raw bytes (path_b64); target = safe user name joined
        // at byte level. Recovery of an invalid-UTF-8 name is exactly this flow.
        assertSafeName(name);
        await moveFsEntry(id, prompt.fromB64, joinPathB64(pathB64, name));
        void refreshKids(pathB64);
      } else if (prompt.kind === "dir") {
        await createFsEntry(id, joinPathB64(pathB64, name), "dir");
        void refreshKids(pathB64);
      } else {
        await createFsEntry(id, joinPathB64(pathB64, name), "file");
      }
      setPrompt(null);
      await load(pathB64);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [prompt, id, pathB64, load, refreshKids]);

  const uploadFiles = useCallback(
    async (files: FileList | File[]) => {
      for (const f of Array.from(files)) {
        try {
          // text-like → utf8, everything else → base64
          const isText = /\.(txt|md|log|csv|tsv|json|yaml|yml|toml|ini|cfg|env|ts|tsx|js|jsx|py|rs|go|sh|html|css|xml|gitignore|lock|ipynb)$/i.test(f.name) || f.type.startsWith("text/") || f.type === "application/json";
          if (isText && f.size < 2_000_000) {
            const content = await f.text();
            await uploadFsFile(id, pathB64, f.name, content, "utf8");
          } else {
            const b64 = await new Promise<string>((res, rej) => {
              const r = new FileReader();
              r.onload = () => res(String(r.result).split(",")[1] ?? "");
              r.onerror = () => rej(new Error(`cannot read ${f.name}`));
              r.readAsDataURL(f);
            });
            await uploadFsFile(id, pathB64, f.name, b64, "base64");
          }
        } catch (err) {
          setError(`upload ${f.name}: ${err instanceof Error ? err.message : String(err)}`);
        }
      }
      await load(pathB64);
    },
    [id, pathB64, load],
  );

  const resetWorkspace = useCallback(async () => {
    if (!confirm("Reset the workspace to the image seed? All changes in this demo workspace are lost.")) return;
    try {
      await resetFs(id);
      setExpanded(new Set());
      setKids({});
      await load(homeB64);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [id, homeB64, load]);

  const totalSize = entries.reduce((a, e) => a + (e.type === "file" ? e.size : 0), 0);
  const dirty = !!ed && ed.content !== ed.orig;

  return (
    <div className="flex flex-col gap-4">
      {/* ── header ───────────────────────────────────────────────── */}
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex min-w-0 flex-wrap items-center gap-2.5">
          <button
            onClick={() => go(`/sandboxes/${id}`)}
            className="grid size-9 shrink-0 place-items-center rounded-lg border border-line text-gray-400 outline-none transition-colors hover:bg-fg/5 hover:text-gray-200 focus-visible:ring-2 focus-visible:ring-ring"
            aria-label="Back to sandbox"
          >
            <span className="text-sm">←</span>
          </button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate text-[24px] font-bold tracking-tight text-ink">
                <FolderOpen className="mr-2 inline size-5 text-info" />
                Files
              </h1>
              <Mono className="rounded-md border border-line bg-surface px-2 py-1 text-xs text-info">{sandbox?.name ?? id}</Mono>
            </div>
            <Mono className="mt-1 block text-[11px] text-gray-500">
              workspace FTP · {sandbox?.image ?? "—"} · /sandboxes/{id}/files
            </Mono>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setLive((v) => !v)}
            className={cn(btn, live ? "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20" : "border-line bg-surface text-gray-300 hover:bg-fg/5")}
            aria-pressed={live}
            title="Auto-refresh the listing every 4s"
          >
            <span className={cn("size-1.5 rounded-full", live ? "bg-neon dot-live" : "bg-gray-500")} /> Live
          </button>
          <button onClick={() => void load(pathB64)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")} disabled={loading} title="Refresh">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> <span className="hidden sm:inline">Refresh</span>
          </button>
          {canWrite && (
            <>
              <button onClick={() => setPrompt({ kind: "dir", value: "" })} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
                <FolderPlus className="size-3.5" /> <span className="hidden sm:inline">Folder</span>
              </button>
              <button onClick={() => setPrompt({ kind: "file", value: "" })} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
                <FilePlus2 className="size-3.5" /> <span className="hidden sm:inline">File</span>
              </button>
              <button onClick={() => uploadRef.current?.click()} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
                <Upload className="size-3.5" /> <span className="hidden sm:inline">Upload</span>
              </button>
              <button onClick={() => void resetWorkspace()} className={cn(btn, "border-line bg-surface text-warn hover:bg-warn/10")} title="Reset workspace to image seed">
                <RotateCcw className="size-3.5" />
              </button>
            </>
          )}
          <input ref={uploadRef} type="file" multiple hidden onChange={(e) => e.target.files && void uploadFiles(e.target.files)} />
        </div>
      </div>

      {error && (
        <Panel className="border-danger/40 p-3 text-sm text-danger">
          {error} <button onClick={() => setError("")} className="ml-2 text-xs text-gray-400 hover:text-gray-200">dismiss</button>
        </Panel>
      )}

      {/* ── browser ──────────────────────────────────────────────── */}
      <Panel className={cn("flex overflow-hidden", "min-h-[540px]", "flex-col lg:flex-row")}>
        {/* tree — desktop */}
        <aside className="hidden w-[240px] shrink-0 overflow-y-auto border-r border-line bg-navy/40 py-3 lg:block">
          <div className="px-3 pb-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-gray-600">Workspace tree</div>
          <TreeBranch
            id={id}
            path={homeB64}
            name={crumbsFromB64(homeB64).at(-1)?.label ?? home}
            depth={0}
            current={pathB64}
            expanded={expanded}
            kids={kids}
            onToggle={toggleTree}
            onEnter={enter}
          />
        </aside>

        {/* main listing */}
        <div className="flex min-w-0 flex-1 flex-col">
          {/* breadcrumb / path bar */}
          <div className="flex flex-wrap items-center gap-1.5 border-b border-line px-3 py-2">
            <button
              onClick={() => setTreeOpen(true)}
              className="grid size-8 place-items-center rounded-md border border-line text-gray-400 outline-none transition-colors hover:text-gray-200 focus-visible:ring-2 focus-visible:ring-ring lg:hidden"
              aria-label="Open workspace tree"
            >
              <ListTree className="size-4" />
            </button>
            <button
              onClick={() => enter(parentB64(pathB64))}
              className="grid size-8 place-items-center rounded-md border border-line text-gray-400 outline-none transition-colors hover:text-gray-200 focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Parent directory"
              disabled={pathB64 === ROOT_B64}
            >
              <ChevronRight className="size-4 -rotate-90" />
            </button>
            <nav className="flex min-w-0 flex-1 basis-full flex-wrap items-center gap-0.5 font-mono text-xs sm:basis-auto" aria-label="Breadcrumb">
              {pathB64 === homeB64 && <FolderOpen className="mr-1 size-3.5 text-info" />}
              {crumbs(pathB64, homeB64).map((c, i, arr) => (
                <span key={c.path} className="flex items-center">
                  <button
                    onClick={() => enter(c.path)}
                    className={cn(
                      "max-w-[180px] truncate rounded px-1.5 py-1 outline-none transition-colors hover:bg-fg/5 focus-visible:ring-2 focus-visible:ring-ring",
                      i === arr.length - 1 ? "text-neon" : "text-gray-400 hover:text-gray-200",
                    )}
                  >
                    {c.label}
                  </button>
                  {i < arr.length - 1 && <span className="text-gray-600">/</span>}
                </span>
              ))}
            </nav>
            <select
              value={sort}
              onChange={(e) => setSort(e.target.value as SortKey)}
              className="ml-auto h-8 rounded-md border border-line bg-navy/60 px-2 text-xs text-gray-300 outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Sort files"
            >
              <option value="name">Name</option>
              <option value="size">Size</option>
              <option value="mtime">Modified</option>
            </select>
            <label className="flex cursor-pointer items-center gap-1.5 text-xs text-gray-400" title="Show dotfiles">
              <input type="checkbox" checked={showHidden} onChange={(e) => setShowHidden(e.target.checked)} className="accent-[var(--neon)]" />
              hidden
            </label>
          </div>

          {/* table */}
          <div
            className={cn("min-h-0 flex-1 overflow-y-auto transition-colors", dragOver && "bg-neon/5")}
            onDragOver={(e) => {
              e.preventDefault();
              if (canWrite) setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDragOver(false);
              if (canWrite && e.dataTransfer.files.length) void uploadFiles(e.dataTransfer.files);
            }}
          >
            <table className="w-full min-w-[560px] text-sm">
              <thead className="sticky top-0 z-10 bg-surface">
                <tr className="border-b border-line text-left text-[11px] uppercase tracking-wider text-gray-500">
                  <th className="px-4 py-2.5 font-medium">Name</th>
                  <th className="hidden px-3 py-2.5 font-medium md:table-cell">Size</th>
                  <th className="hidden px-3 py-2.5 font-medium lg:table-cell">Mode</th>
                  <th className="hidden px-3 py-2.5 font-medium md:table-cell">Modified</th>
                  <th className="px-3 py-2.5 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {loading &&
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={i} className="animate-pulse border-b border-line-soft">
                      <td className="px-4 py-3">
                        <div className="h-4 w-48 rounded bg-line" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-4 w-12 rounded bg-line" />
                      </td>
                      <td colSpan={3} />
                    </tr>
                  ))}
                {!loading &&
                  sorted.map((e) => {
                    const n = entryName(e);
                    return (
                      <tr
                        key={entryB64(e)}
                        onClick={() => (e.type === "dir" ? enter(entryB64(e)) : void openFile(e))}
                        className="group cursor-pointer border-b border-line-soft last:border-0 transition-colors hover:bg-line/40"
                      >
                        <td className="px-4 py-2.5">
                          <div className="flex items-center gap-2.5">
                            <FileGlyph entry={e} />
                            <span className={cn("max-w-[320px] truncate text-[13px]", e.type === "dir" ? "font-medium text-gray-100" : "text-gray-200")} title={n.valid ? e.name : n.hex}>
                              {n.display}
                            </span>
                            {!n.valid && (
                              <span
                                className="rounded border border-warn/40 bg-warn/10 px-1 font-mono text-[9px] text-warn"
                                title={n.hex}
                              >
                                Invalid UTF-8
                              </span>
                            )}
                            {n.valid && e.name.startsWith(".") && <span className="rounded border border-line px-1 font-mono text-[9px] text-gray-600">dot</span>}
                          </div>
                        </td>
                        <td className="hidden px-3 py-2.5 font-mono text-xs text-gray-500 md:table-cell">{e.type === "dir" ? "—" : fmtBytes(e.size)}</td>
                        <td className="hidden px-3 py-2.5 font-mono text-xs text-gray-500 lg:table-cell">{modeRwx(e.mode, e.type === "dir")}</td>
                        <td className="hidden px-3 py-2.5 font-mono text-xs text-gray-500 md:table-cell">{fmtMtime(e.mtime)}</td>
                        <td className="px-3 py-2.5" onClick={(ev) => ev.stopPropagation()}>
                          <div className="flex items-center justify-end gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100 max-md:opacity-100">
                            {e.type === "file" && (
                              <>
                                <button onClick={() => void openFile(e)} className="rounded-md p-1.5 text-gray-500 outline-none transition-colors hover:bg-fg/5 hover:text-neon focus-visible:ring-2 focus-visible:ring-ring" title="View & edit">
                                  <Pencil className="size-3.5" />
                                </button>
                                <button onClick={() => void download(e)} className="rounded-md p-1.5 text-gray-500 outline-none transition-colors hover:bg-fg/5 hover:text-info focus-visible:ring-2 focus-visible:ring-ring" title="Download">
                                  <Download className="size-3.5" />
                                </button>
                              </>
                            )}
                            {canWrite && (
                              <>
                                {!n.valid && (
                                  <button
                                    onClick={() => setPrompt({ kind: "rename", value: "text-recovered.txt", fromB64: entryB64(e) })}
                                    className="rounded-md border border-warn/40 bg-warn/10 px-1.5 py-0.5 text-[10px] font-medium text-warn outline-none transition-colors hover:bg-warn/20 focus-visible:ring-2 focus-visible:ring-ring"
                                    title={`Đổi tên an toàn từ bytes gốc (${n.hex})`}
                                  >
                                    Đổi tên an toàn
                                  </button>
                                )}
                                <button
                                  onClick={() => setPrompt({ kind: "rename", value: n.valid ? e.name : "text-recovered.txt", fromB64: entryB64(e) })}
                                  className="rounded-md p-1.5 text-gray-500 outline-none transition-colors hover:bg-fg/5 hover:text-warn focus-visible:ring-2 focus-visible:ring-ring"
                                  title="Rename"
                                >
                                  <SquarePen className="size-3.5" />
                                </button>
                                <button onClick={() => void remove(e)} className="rounded-md p-1.5 text-gray-500 outline-none transition-colors hover:bg-fg/5 hover:text-danger focus-visible:ring-2 focus-visible:ring-ring" title="Delete">
                                  <Trash2 className="size-3.5" />
                                </button>
                              </>
                            )}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                {!loading && sorted.length === 0 && (
                  <tr>
                    <td colSpan={5} className="px-4 py-14 text-center">
                      <FolderOpen className="mx-auto size-7 text-gray-600" />
                      <p className="mt-2 text-sm text-gray-500">Empty directory{showHidden ? "" : " — turn on “hidden” for dotfiles"}.</p>
                      {canWrite && (
                        <p className="mt-1 text-xs text-gray-600">
                          drag & drop files here, or use <span className="text-gray-400">Upload</span>
                        </p>
                      )}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
            {canWrite && dragOver && (
              <div className="pointer-events-none sticky bottom-4 mx-auto w-fit rounded-lg border border-neon/40 bg-navy/90 px-4 py-2 text-xs text-neon shadow-[0_0_20px_var(--glow-soft)]">
                drop files to upload into {path}
              </div>
            )}
          </div>

          {/* status bar */}
          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line px-4 py-2 font-mono text-[10px] text-gray-600">
            <span>
              {entries.length} entries · {fmtBytes(totalSize)} in files · {path}
            </span>
            <span className="hidden sm:block">execv2-backed · paths là operand base64 bytes (#219)</span>
          </div>
        </div>
      </Panel>

      {/* mobile tree drawer */}
      <Sheet open={treeOpen} onOpenChange={setTreeOpen}>
        <SheetContent side="left" className="w-[280px] overflow-y-auto bg-navy p-0">
          <SheetHeader className="px-4 pt-4">
            <SheetTitle className="text-sm text-gray-300">Workspace tree</SheetTitle>
          </SheetHeader>
          <div className="px-2 pb-4">
            <TreeBranch
              id={id}
              path={homeB64}
              name={crumbsFromB64(homeB64).at(-1)?.label ?? home}
              depth={0}
              current={pathB64}
              expanded={expanded}
              kids={kids}
              onToggle={toggleTree}
              onEnter={enter}
            />
          </div>
        </SheetContent>
      </Sheet>

      {/* ── editor dialog ─────────────────────────────────────────── */}
      <Dialog open={!!ed} onOpenChange={(o) => !o && setEd(null)}>
        <DialogContent className="flex h-[92vh] max-w-[900px] flex-col gap-0 overflow-hidden p-0 sm:h-[85vh]">
          {ed && (
            <>
              <DialogHeader className="border-b border-line px-5 py-3.5">
                <DialogTitle className="flex flex-wrap items-center gap-2 font-mono text-sm text-ink">
                  <FileGlyph entry={{ name: ed.pathDisplay, path: ed.pathDisplay, type: "file", size: ed.size, mode: ed.mode, mtime: ed.mtime, owner: "sandbox" }} />
                  <span className="break-all">{ed.pathDisplay}</span>
                  {dirty && <span className="rounded border border-warn/40 bg-warn/10 px-1.5 py-0.5 text-[10px] font-medium text-warn">unsaved</span>}
                </DialogTitle>
                <DialogDescription className="font-mono text-[11px] text-gray-500">
                  {fmtBytes(ed.size)} · {modeRwx(ed.mode, false)} · edited {fmtMtime(ed.mtime)} · {ed.encoding === "base64" ? "binary" : "utf-8"}
                </DialogDescription>
              </DialogHeader>
              {ed.encoding === "base64" ? (
                <div className="grid flex-1 place-items-center px-6 text-center">
                  <div>
                    <Download className="mx-auto size-8 text-gray-500" />
                    <p className="mt-3 text-sm text-gray-300">Binary file — editing is disabled.</p>
                    <p className="mt-1 text-xs text-gray-500">Tải về để xem — file nhị phân không sửa trực tiếp được.</p>
                    <button onClick={() => void download({ name: ed.pathDisplay.split("/").pop() ?? "file", path: ed.pathDisplay, path_b64: ed.pathB64, type: "file", size: ed.size, mode: ed.mode, mtime: ed.mtime, owner: "sandbox" })} className={cn(btn, "mt-4 border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}>
                      <Download className="size-3.5" /> Download
                    </button>
                  </div>
                </div>
              ) : (
                <textarea
                  value={ed.content}
                  onChange={(e) => setEd({ ...ed, content: e.target.value })}
                  spellCheck={false}
                  className="min-h-0 flex-1 resize-none bg-sunken px-4 py-3 font-mono text-[12.5px] leading-relaxed text-gray-200 outline-none focus-visible:ring-inset focus-visible:ring-1 focus-visible:ring-ring"
                  aria-label={`Edit ${ed.pathDisplay}`}
                />
              )}
              <DialogFooter className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-5 py-3">
                <Mono className="text-[10px] text-gray-600">Ctrl+S saves into the sandbox · demo shell sees it instantly</Mono>
                <div className="flex items-center gap-2">
                  <button onClick={() => setEd(null)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
                    {ed.encoding === "base64" ? "Close" : "Cancel"}
                  </button>
                  {ed.encoding === "utf8" && (
                    <button
                      onClick={() => void saveFile()}
                      disabled={!dirty || saving}
                      className="inline-flex items-center gap-2 rounded-lg bg-neon px-4 py-2 text-sm font-semibold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_20px_var(--glow-mid)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40"
                    >
                      {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Download className="size-3.5 rotate-180" />} Save
                    </button>
                  )}
                </div>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* ── new file / folder / rename prompt ─────────────────────── */}
      <Dialog open={!!prompt} onOpenChange={(o) => !o && setPrompt(null)}>
        <DialogContent className="sm:max-w-[420px]">
          {prompt && (
            <>
              <DialogHeader>
                <DialogTitle className="text-ink">
                  {prompt.kind === "rename" ? (prompt.value === "text-recovered.txt" ? "Đổi tên an toàn (recovery)" : "Rename") : prompt.kind === "dir" ? "New folder" : "New file"}
                </DialogTitle>
                <DialogDescription>
                  {prompt.kind === "rename"
                    ? `Renaming the raw-bytes path (${prompt.fromB64}) — enter the new name (valid UTF-8, one segment).`
                    : `Created inside ${path}.`}
                </DialogDescription>
              </DialogHeader>
              <input
                autoFocus
                value={prompt.value}
                onChange={(e) => setPrompt({ ...prompt, value: e.target.value })}
                onKeyDown={(e) => e.key === "Enter" && void submitPrompt()}
                placeholder={prompt.kind === "dir" ? "assets" : prompt.kind === "file" ? "notes.md" : prompt.value}
                className="h-10 w-full rounded-lg border border-line bg-navy/60 px-3 font-mono text-sm text-gray-100 placeholder:text-gray-600 outline-none focus-visible:ring-2 focus-visible:ring-ring"
                aria-label="Name"
              />
              <DialogFooter>
                <button onClick={() => setPrompt(null)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
                  Cancel
                </button>
                <button
                  onClick={() => void submitPrompt()}
                  className="inline-flex items-center gap-2 rounded-lg bg-neon px-4 py-2 text-sm font-semibold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_20px_var(--glow-mid)]"
                >
                  {prompt.kind === "rename" ? "Rename" : "Create"}
                </button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
