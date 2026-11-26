"use client";

import { useEffect, useMemo, useState } from "react";
import {
  BadgeCheck,
  Bell,
  CheckCircle2,
  FileClock,
  Globe,
  KeyRound,
  LogOut,
  MonitorSmartphone,
  Palette,
  RefreshCw,
  Save,
  ScrollText,
  ShieldCheck,
  Stamp,
  TriangleAlert,
  UserCircle,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";
import { get as apiGet, put as apiPut, post as apiPost, del as apiDel, type ApiKey, type AuditEntry, type DomainInfo, type Policy } from "@/console/api";
import { usePlatform } from "@/lib/platform-store";
import { useTheme } from "@/lib/theme-store";
import { cn } from "@/lib/utils";
import { FancySelect } from "../controls";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Mono, PageHeader, Panel, SectionTitle, UserAvatar } from "../primitives";

/* ══ Settings — the full settings hub of the console ══════════════════
   Replaces the old placeholder: a 12-entry sub-menu (left rail) with the
   same SBX design language as the rest of the platform views. Sections
   backed by sandboxd: API keys, policy, approvals, audit, domain.
   Client-persisted sections (localStorage): general, appearance,
   profile, security, notifications, sessions, danger zone. */

type SectionId =
  | "general"
  | "appearance"
  | "profile"
  | "security"
  | "notifications"
  | "api-keys"
  | "policy"
  | "audit"
  | "domain"
  | "sessions"
  | "danger";

const SECTIONS: Array<{ id: SectionId; label: string; vi: string; icon: typeof Bell; group: "workspace" | "account" | "system" }> = [
  { id: "general",       label: "General",        vi: "Chung",           icon: UserCircle,         group: "workspace" },
  { id: "appearance",    label: "Appearance",     vi: "Giao diện",       icon: Palette,            group: "workspace" },
  { id: "notifications", label: "Notifications",  vi: "Thông báo",       icon: Bell,               group: "workspace" },
  { id: "profile",       label: "Profile",        vi: "Hồ sơ",           icon: BadgeCheck,         group: "account" },
  { id: "security",      label: "Security",       vi: "Bảo mật",         icon: ShieldCheck,        group: "account" },
  { id: "sessions",      label: "Sessions",       vi: "Phiên đăng nhập", icon: MonitorSmartphone,  group: "account" },
  { id: "api-keys",      label: "API Keys",       vi: "Khóa API",        icon: KeyRound,           group: "system" },
  { id: "policy",        label: "Access & Policy", vi: "Truy cập & chính sách", icon: Stamp,       group: "system" },
  { id: "audit",         label: "Audit Log",      vi: "Nhật ký",         icon: ScrollText,         group: "system" },
  { id: "domain",        label: "Domain & TLS",   vi: "Tên miền",        icon: Globe,              group: "system" },
  { id: "danger",        label: "Danger Zone",    vi: "Vùng nguy hiểm",  icon: TriangleAlert,      group: "system" },
];

/* Approvals outgrew settings — the gated-action queue is a first-class
   page now (/approvals, with a pending badge in the sidebar). */

const GROUP_LABEL: Record<string, [string, string]> = {
  workspace: ["Workspace", "Không gian làm việc"],
  account: ["Account", "Tài khoản"],
  system: ["System administration", "Quản trị hệ thống"],
};

const DEFAULT_SECTION: SectionId = "general";

function isSection(v: string | undefined): v is SectionId {
  return !!v && SECTIONS.some((s) => s.id === v);
}

/** section id from a URL: /settings/<id>, or the old /admin/<tab> console */
export function settingsSectionFromPath(pathname: string): SectionId | null {
  const seg = pathname.split("/").filter(Boolean);
  if (seg[0] === "settings") return isSection(seg[1]) ? seg[1] : null;
  if (seg[0] === "admin") {
    const map: Record<string, SectionId> = { policy: "policy", audit: "audit", keys: "api-keys" };
    return map[seg[1] ?? ""] ?? null;
  }
  return null;
}

/* ── tiny localStorage state helper ─────────────────────────────────── */
function useLocalState<T>(key: string, initial: T): [T, (v: T) => void] {
  const [value, setValue] = useState<T>(initial);
  useEffect(() => {
    try {
      const raw = localStorage.getItem(key);
      if (raw) setValue(JSON.parse(raw) as T);
    } catch {
      /* private mode */
    }
     
  }, [key]);
  const set = (v: T) => {
    setValue(v);
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {
      /* private mode */
    }
  };
  return [value, set];
}

/* ── shared bits ────────────────────────────────────────────────────── */

function Field({ label, vi, children, hint }: { label: string; vi?: string; children: React.ReactNode; hint?: string }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-gray-300">
        {label}
        {vi && <span className="ml-1.5 text-[11px] text-gray-600">· {vi}</span>}
      </span>
      {children}
      {hint && <span className="text-[11px] text-gray-600">{hint}</span>}
    </label>
  );
}

const textInput =
  "h-10 w-full rounded-lg border border-line bg-navy/60 px-3 text-sm text-gray-100 placeholder:text-gray-600 outline-none focus-visible:ring-2 focus-visible:ring-ring";

const btnNeon =
  "inline-flex items-center gap-2 rounded-lg border border-neon/30 bg-neon/10 px-4 py-2 text-sm text-neon transition-colors hover:bg-neon/20 disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const btnGhost =
  "inline-flex items-center gap-2 rounded-lg border border-line px-4 py-2 text-sm text-gray-300 transition-colors hover:bg-fg/5 disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const btnDanger =
  "inline-flex items-center gap-2 rounded-lg border border-danger/40 bg-danger/10 px-4 py-2 text-sm text-danger transition-colors hover:bg-danger/20 disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

function ToggleRow({ title, vi, desc, on, onChange, disabled }: { title: string; vi?: string; desc?: string; on: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-line-soft py-3.5 last:border-0">
      <div className="min-w-0">
        <div className="text-sm text-gray-200">
          {title}
          {vi && <span className="ml-1.5 text-[11px] text-gray-600">· {vi}</span>}
        </div>
        {desc && <p className="mt-0.5 text-xs leading-relaxed text-gray-500">{desc}</p>}
      </div>
      <Switch checked={on} onCheckedChange={onChange} disabled={disabled} aria-label={title} />
    </div>
  );
}

/* ══ 1 · General ═══════════════════════════════════════════════════════ */

function GeneralSection() {
  const [general, setGeneral] = useLocalState("sbx-set-general", {
    instanceName: "4AiCoding Sandbox",
    description: "Shared dev sandboxes for the platform team",
    timezone: "Asia/Ho_Chi_Minh",
    language: "en",
  });
  const [savedTick, setSavedTick] = useState(0);
  useEffect(() => {
    if (!savedTick) return;
    const t = setTimeout(() => setSavedTick(0), 2000);
    return () => clearTimeout(t);
  }, [savedTick]);

  return (
    <Panel className="p-6">
      <SectionTitle title="Instance preferences" vi="Cấu hình chung" />
      <div className="mt-5 grid grid-cols-1 gap-5 md:grid-cols-2">
        <Field label="Instance name" vi="Tên phiên bản">
          <input className={textInput} value={general.instanceName} onChange={(e) => setGeneral({ ...general, instanceName: e.target.value })} />
        </Field>
        <Field label="Description" vi="Mô tả">
          <input className={textInput} value={general.description} onChange={(e) => setGeneral({ ...general, description: e.target.value })} />
        </Field>
        <Field label="Timezone" vi="Múi giờ" hint="Used for audit timestamps and reports.">
          <FancySelect
            ariaLabel="Timezone"
            value={general.timezone}
            onChange={(v) => setGeneral({ ...general, timezone: v })}
            options={["Asia/Ho_Chi_Minh", "Asia/Singapore", "Asia/Tokyo", "Europe/Berlin", "UTC"].map((tz) => ({ value: tz, label: tz }))}
          />
        </Field>
        <Field label="Console language" vi="Ngôn ngữ bảng điều khiển" hint="Vietnamese labels stay visible as secondary hints.">
          <FancySelect
            ariaLabel="Console language"
            value={general.language}
            onChange={(v) => setGeneral({ ...general, language: v })}
            options={[
              { value: "en", label: "English" },
              { value: "vi", label: "Tiếng Việt" },
            ]}
          />
        </Field>
      </div>
      <div className="mt-6 flex items-center gap-3">
        <button
          className={btnNeon}
          onClick={() => {
            setSavedTick(Date.now());
            toast.success("General settings saved · Đã lưu cài đặt chung");
          }}
        >
          <Save className="size-4" /> Save changes
        </button>
        {savedTick > 0 && (
          <Mono className="text-xs text-neon">
            <span className="dot-live mr-1.5 inline-block size-1.5 rounded-full bg-neon align-middle" />
            saved
          </Mono>
        )}
      </div>
    </Panel>
  );
}

/* ══ 2 · Appearance ════════════════════════════════════════════════════ */

function AppearanceSection() {
  const theme = useTheme((s) => s.theme);
  const setTheme = useTheme((s) => s.setTheme);
  const [density, setDensity] = useLocalState<"comfortable" | "compact">("sbx-set-density", "comfortable");

  return (
    <Panel className="p-6">
      <SectionTitle title="Theme" vi="Chủ đề" />
      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
        {(["dark", "light"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTheme(t)}
            aria-pressed={theme === t}
            className={cn(
              "rounded-xl border p-4 text-left transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              theme === t ? "border-neon/50 bg-neon/5 shadow-[inset_0_0_0_1px_var(--glow-mid)]" : "border-line hover:border-gray-500",
            )}
          >
            <div className={cn("mb-3 h-20 rounded-lg border", t === "dark" ? "border-line bg-[#0a0f1d]" : "border-gray-300 bg-[#f4f6fb]")}>
              <div className="flex h-full items-center justify-center gap-1.5 px-4">
                <div className={cn("h-6 flex-1 rounded", t === "dark" ? "bg-[#111827]" : "bg-white")} />
                <div className="h-6 w-8 rounded" style={{ background: t === "dark" ? "#00ff9d" : "#047857", opacity: 0.85 }} />
              </div>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-gray-200">
                {t === "dark" ? "Dark · brand default" : "Light · daylight enterprise"}
              </span>
              {theme === t && <BadgeCheck className="size-4 text-neon" />}
            </div>
          </button>
        ))}
      </div>
      <div className="mt-6 border-t border-line-soft pt-5">
        <SectionTitle title="Density" vi="Mật độ hiển thị" />
        <div className="mt-3 flex items-center gap-2">
          {(["comfortable", "compact"] as const).map((d) => (
            <button
              key={d}
              onClick={() => setDensity(d)}
              aria-pressed={density === d}
              className={cn(
                "rounded-lg border px-3.5 py-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                density === d ? "border-neon/40 bg-neon/10 text-neon" : "border-line text-gray-400 hover:bg-fg/5",
              )}
            >
              {d === "comfortable" ? "Comfortable · Thoải mái" : "Compact · Gọn"}
            </button>
          ))}
        </div>
      </div>
    </Panel>
  );
}

/* ══ 3 · Notifications ═════════════════════════════════════════════════ */

function NotificationsSection() {
  const [prefs, setPrefs] = useLocalState("sbx-set-notify", {
    sandboxFailure: true,
    approvals: true,
    egressBlocked: true,
    newNode: false,
    weeklyDigest: true,
    sound: false,
  });

  return (
    <Panel className="p-6">
      <SectionTitle title="Event notifications" vi="Thông báo sự kiện" />
      <div className="mt-2">
        <ToggleRow title="Sandbox failures" vi="Lỗi sandbox" desc="A sandbox enters the Error state or crashes." on={prefs.sandboxFailure} onChange={(v) => setPrefs({ ...prefs, sandboxFailure: v })} />
        <ToggleRow title="Approval requests" vi="Yêu cầu phê duyệt" desc="Someone requests a gated action that needs an admin decision." on={prefs.approvals} onChange={(v) => setPrefs({ ...prefs, approvals: v })} />
        <ToggleRow title="Egress blocked" vi="Chặn kết nối ra" desc="Outbound requests denied by the firewall allowlist." on={prefs.egressBlocked} onChange={(v) => setPrefs({ ...prefs, egressBlocked: v })} />
        <ToggleRow title="Node joins / leaves fleet" vi="Node tham gia / rời" desc="A worker registers or drops its heartbeat." on={prefs.newNode} onChange={(v) => setPrefs({ ...prefs, newNode: v })} />
      </div>
      <div className="mt-6 border-t border-line-soft pt-5">
        <SectionTitle title="Delivery" vi="Kênh nhận" />
        <div className="mt-2">
          <ToggleRow title="Weekly digest email" vi="Email tổng hợp tuần" desc="Every Monday 08:00 — fleet KPIs, audit highlights, capacity." on={prefs.weeklyDigest} onChange={(v) => setPrefs({ ...prefs, weeklyDigest: v })} />
          <ToggleRow title="In-app sound" vi="Âm thanh trong app" desc="Play a chime for critical alerts." on={prefs.sound} onChange={(v) => setPrefs({ ...prefs, sound: v })} />
        </div>
      </div>
    </Panel>
  );
}

/* ══ 4 · Profile ═══════════════════════════════════════════════════════ */

function ProfileSection() {
  const me = usePlatform((s) => s.me);
  const [profile, setProfile] = useLocalState("sbx-set-profile", { displayName: "", email: "", colorIdx: 0 });
  const name = me?.username ?? "operator";
  const displayName = profile.displayName || name;

  return (
    <div className="space-y-5">
      <Panel className="p-6">
        <SectionTitle title="Identity" vi="Danh tính" />
        <div className="mt-5 flex flex-col gap-6 sm:flex-row sm:items-start">
          <div className="flex flex-col items-center gap-3">
            <UserAvatar name={displayName} colorIdx={profile.colorIdx} size="lg" />
            <div className="flex flex-wrap justify-center gap-1.5">
              {Array.from({ length: 11 }, (_, i) => (
                <button
                  key={i}
                  onClick={() => setProfile({ ...profile, colorIdx: i })}
                  aria-label={`Avatar color ${i + 1}`}
                  className={cn("rounded-full p-0.5 transition-transform", profile.colorIdx === i ? "scale-110 ring-2 ring-neon/60" : "hover:scale-105")}
                >
                  <UserAvatar name={name} colorIdx={i} size="sm" />
                </button>
              ))}
            </div>
          </div>
          <div className="grid flex-1 grid-cols-1 gap-5 md:grid-cols-2">
            <Field label="Display name" vi="Tên hiển thị" hint="Shown in the sidebar footer and avatar menu.">
              <input className={textInput} placeholder={name} value={profile.displayName} onChange={(e) => setProfile({ ...profile, displayName: e.target.value })} />
            </Field>
            <Field label="Email" vi="Email" hint="Used for notification delivery.">
              <input className={textInput} type="email" placeholder="you@4aicoding.dev" value={profile.email} onChange={(e) => setProfile({ ...profile, email: e.target.value })} />
            </Field>
            <div className="md:col-span-2 flex flex-wrap items-center gap-x-8 gap-y-3 rounded-lg border border-line bg-navy/40 px-4 py-3">
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-wider text-gray-600">Signed in as</div>
                <Mono className="text-sm text-gray-200">{me?.username ?? "—"}</Mono>
              </div>
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-wider text-gray-600">Role · Vai trò</div>
                <Mono className="text-sm text-admin">{me?.role ?? "—"}</Mono>
              </div>
            </div>
          </div>
        </div>
        <div className="mt-6">
          <button className={btnNeon} onClick={() => toast.success("Profile saved · Đã lưu hồ sơ")}>
            <Save className="size-4" /> Save profile
          </button>
        </div>
      </Panel>
    </div>
  );
}

/* ══ 5 · Security ══════════════════════════════════════════════════════ */

function SecuritySection() {
  const [pw, setPw] = useState({ current: "", next: "", confirm: "" });
  const [sec, setSec] = useLocalState("sbx-set-security", { twoFactor: false, sessionTtl: "24h", requireStrong: true });

  const changePw = () => {
    if (!pw.current || !pw.next) {
      toast.error("Fill in both password fields · Nhập đủ cả hai mật khẩu");
      return;
    }
    if (pw.next.length < 8) {
      toast.error("New password must be at least 8 characters · Mật khẩu mới tối thiểu 8 ký tự");
      return;
    }
    if (pw.next !== pw.confirm) {
      toast.error("Passwords do not match · Mật khẩu không khớp");
      return;
    }
    setPw({ current: "", next: "", confirm: "" });
    toast.success("Password changed · Đã đổi mật khẩu");
  };

  return (
    <div className="space-y-5">
      <Panel className="p-6">
        <SectionTitle title="Change password" vi="Đổi mật khẩu" />
        <div className="mt-5 grid grid-cols-1 gap-5 md:grid-cols-3">
          <Field label="Current password" vi="Mật khẩu hiện tại">
            <input className={textInput} type="password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} autoComplete="current-password" />
          </Field>
          <Field label="New password" vi="Mật khẩu mới">
            <input className={textInput} type="password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} autoComplete="new-password" />
          </Field>
          <Field label="Confirm new password" vi="Xác nhận mật khẩu mới">
            <input className={textInput} type="password" value={pw.confirm} onChange={(e) => setPw({ ...pw, confirm: e.target.value })} autoComplete="new-password" />
          </Field>
        </div>
        <div className="mt-6">
          <button className={btnNeon} onClick={changePw}>
            <ShieldCheck className="size-4" /> Update password
          </button>
        </div>
      </Panel>

      <Panel className="p-6">
        <SectionTitle title="Session & 2FA" vi="Phiên & xác thực hai lớp" />
        <div className="mt-2">
          <ToggleRow
            title="Two-factor authentication (TOTP)"
            vi="Xác thực hai lớp"
            desc="Require a 6-digit authenticator code after the password."
            on={sec.twoFactor}
            onChange={(v) => {
              setSec({ ...sec, twoFactor: v });
              toast(v ? "2FA enabled · Đã bật 2FA" : "2FA disabled · Đã tắt 2FA");
            }}
          />
          <ToggleRow
            title="Require strong passwords"
            vi="Yêu cầu mật khẩu mạnh"
            desc="12+ chars with upper, lower, digit and symbol for every account."
            on={sec.requireStrong}
            onChange={(v) => setSec({ ...sec, requireStrong: v })}
          />
        </div>
        <div className="mt-5 max-w-xs">
          <Field label="Session lifetime" vi="Thời hạn phiên" hint="Idle sessions are revoked after this window.">
            <FancySelect
              ariaLabel="Session lifetime"
              value={sec.sessionTtl}
              onChange={(v) => setSec({ ...sec, sessionTtl: v })}
              options={["8h", "24h", "7d", "30d"].map((t) => ({ value: t, label: t }))}
            />
          </Field>
        </div>
      </Panel>
    </div>
  );
}

/* ══ 6 · Sessions ══════════════════════════════════════════════════════ */

interface SessionRow {
  id: string;
  device: string;
  browser: string;
  ip: string;
  lastActive: string;
  current: boolean;
}

function SessionsSection() {
  const me = usePlatform((s) => s.me);
  const [sessions, setSessions] = useLocalState<SessionRow[]>("sbx-set-sessions", [
    { id: "ss-1", device: "This browser", browser: "Chrome · macOS", ip: "current session", lastActive: "now", current: true },
    { id: "ss-2", device: "MacBook Pro", browser: "Safari · macOS", ip: "113.161.72.10", lastActive: "2h ago", current: false },
    { id: "ss-3", device: "iPhone 15", browser: "Safari · iOS", ip: "42.115.89.201", lastActive: "1d ago", current: false },
    { id: "ss-4", device: "vscode-bridge", browser: "API key · operator", ip: "token session", lastActive: "4h ago", current: false },
  ]);

  const revoke = (id: string) => {
    setSessions(sessions.filter((s) => s.id !== id));
    toast.success("Session revoked · Đã thu hồi phiên");
  };

  return (
    <Panel className="p-6">
      <SectionTitle title="Active sessions" vi="Phiên đang hoạt động" right={<Mono className="text-[10px] text-gray-500">{sessions.length} active</Mono>} />
      <div className="mt-3 divide-y divide-line-soft">
        {sessions.map((s) => (
          <div key={s.id} className="flex items-center gap-4 py-3.5">
            <MonitorSmartphone className={cn("size-4 shrink-0", s.current ? "text-neon" : "text-gray-500")} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 text-sm text-gray-200">
                {s.device}
                {s.current && <span className="rounded border border-neon/30 bg-neon/10 px-1.5 py-0.5 font-mono text-[10px] text-neon">this device</span>}
              </div>
              <Mono className="text-[11px] text-gray-500">{s.browser} · {s.ip} · {s.lastActive}</Mono>
            </div>
            {!s.current && (
              <button className={btnDanger} onClick={() => revoke(s.id)}>
                Revoke
              </button>
            )}
          </div>
        ))}
        {sessions.length === 1 && <p className="py-3 text-sm text-gray-500">Only this session is active · Chỉ còn phiên hiện tại.</p>}
      </div>
      <p className="mt-4 text-xs text-gray-600">
        Sessions for {me?.username ?? "your account"} · API-key sessions appear with their key role.
      </p>
    </Panel>
  );
}

/* ══ 7 · API Keys ══════════════════════════════════════════════════════ */

const SCOPES = ["health", "sandboxes:read", "sandboxes:write", "sandboxes:exec", "snapshots", "sandboxes:egress", "sandboxes:files"];

function ApiKeysSection() {
  const [keys, setKeys] = useState<ApiKey[] | null>(null);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({ name: "", role: "operator", scopes: ["sandboxes:read"] as string[], ttl: "" });
  const [minted, setMinted] = useState("");

  const load = () => {
    apiGet<{ keys: ApiKey[] }>("/v1/api-keys")
      .then((d) => setKeys(d.keys))
      .catch((e) => toast.error(e instanceof Error ? e.message : "Could not load API keys"));
  };
  useEffect(load, []);

  const create = async () => {
    if (!form.name.trim()) {
      toast.error("Key name is required · Cần tên khóa");
      return;
    }
    try {
      const r = await apiPost<{ key: string }>("/v1/api-keys", {
        name: form.name.trim(),
        role: form.role,
        scopes: form.scopes,
        ...(form.ttl ? { expires_at: new Date(Date.now() + Number(form.ttl) * 86400e3).toISOString() } : {}),
      });
      setMinted(r.key);
      setCreating(false);
      setForm({ name: "", role: "operator", scopes: ["sandboxes:read"], ttl: "" });
      toast.success("API key created · Đã tạo khóa API");
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not create the key");
    }
  };

  const revoke = async (k: ApiKey) => {
    try {
      await apiDel(`/v1/api-keys/${k.id}`);
      toast.success(`Revoked ${k.name} · Đã thu hồi`);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not revoke");
    }
  };

  return (
    <div className="space-y-5">
      <Panel className="p-6">
        <SectionTitle
          title="API keys"
          vi="Khóa API"
          right={
            <button className={btnNeon} onClick={() => setCreating((v) => !v)}>
              <KeyRound className="size-4" /> New key
            </button>
          }
        />
        {creating && (
          <div className="mt-4 rounded-xl border border-line bg-navy/50 p-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <Field label="Name" vi="Tên">
                <input className={textInput} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="ci-runner" />
              </Field>
              <Field label="Role" vi="Vai trò">
                <FancySelect
                  ariaLabel="API key role"
                  value={form.role}
                  onChange={(v) => setForm({ ...form, role: v })}
                  options={["admin", "operator", "viewer"].map((r) => ({ value: r, label: r }))}
                />
              </Field>
              <Field label="Expires in (days)" vi="Hạn (ngày)" hint="Empty = never expires.">
                <input className={textInput} type="number" min="1" value={form.ttl} onChange={(e) => setForm({ ...form, ttl: e.target.value })} placeholder="90" />
              </Field>
            </div>
            <div className="mt-4">
              <div className="mb-2 text-xs font-medium text-gray-300">Scopes · Phạm vi</div>
              <div className="flex flex-wrap gap-2">
                {SCOPES.map((sc) => {
                  const on = form.scopes.includes(sc);
                  return (
                    <button
                      key={sc}
                      onClick={() => setForm({ ...form, scopes: on ? form.scopes.filter((x) => x !== sc) : [...form.scopes, sc] })}
                      aria-pressed={on}
                      className={cn(
                        "rounded-full border px-3 py-1 font-mono text-[11px] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                        on ? "border-neon/40 bg-neon/10 text-neon" : "border-line text-gray-500 hover:text-gray-300",
                      )}
                    >
                      {sc}
                    </button>
                  );
                })}
              </div>
            </div>
            <div className="mt-4 flex gap-2">
              <button className={btnNeon} onClick={create}>
                <KeyRound className="size-4" /> Create key
              </button>
              <button className={btnGhost} onClick={() => setCreating(false)}>
                Cancel
              </button>
            </div>
          </div>
        )}
        {minted && (
          <div className="mt-4 rounded-xl border border-warn/40 bg-warn/5 p-4">
            <div className="text-xs font-medium text-warn">Copy this secret now — it is shown only once · Sao chép ngay — chỉ hiện một lần</div>
            <Mono className="mt-2 block break-all rounded-lg bg-navy px-3 py-2 text-xs text-gray-200">{minted}</Mono>
          </div>
        )}
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[10px] uppercase tracking-wider text-gray-600">
                <th className="pb-2 pr-4 font-semibold">Name</th>
                <th className="pb-2 pr-4 font-semibold">Prefix</th>
                <th className="pb-2 pr-4 font-semibold">Role</th>
                <th className="pb-2 pr-4 font-semibold">Scopes</th>
                <th className="pb-2 pr-4 font-semibold">Last used</th>
                <th className="pb-2 font-semibold"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {(keys ?? []).map((k) => (
                <tr key={k.id} className="text-gray-200">
                  <td className="py-3 pr-4 font-medium">{k.name}</td>
                  <td className="py-3 pr-4"><Mono className="text-xs text-info">{k.prefix}…</Mono></td>
                  <td className="py-3 pr-4"><Mono className="text-xs text-admin">{k.role}</Mono></td>
                  <td className="py-3 pr-4">
                    <div className="flex max-w-64 flex-wrap gap-1">
                      {(k.scopes ?? []).slice(0, 3).map((s) => (
                        <span key={s} className="rounded border border-line px-1.5 py-0.5 font-mono text-[10px] text-gray-400">{s}</span>
                      ))}
                      {(k.scopes?.length ?? 0) > 3 && <span className="font-mono text-[10px] text-gray-600">+{(k.scopes?.length ?? 0) - 3}</span>}
                    </div>
                  </td>
                  <td className="py-3 pr-4 text-xs text-gray-500">{k.last_used_at ? new Date(k.last_used_at).toLocaleString() : "never"}</td>
                  <td className="py-3 text-right">
                    <button className="rounded-lg border border-danger/40 px-3 py-1.5 text-xs text-danger transition-colors hover:bg-danger/10" onClick={() => revoke(k)}>
                      Revoke
                    </button>
                  </td>
                </tr>
              ))}
              {!keys && (
                <tr>
                  <td colSpan={6} className="py-4 text-sm text-gray-500">Loading keys…</td>
                </tr>
              )}
              {keys && keys.length === 0 && (
                <tr>
                  <td colSpan={6} className="py-4 text-sm text-gray-500">No API keys yet · Chưa có khóa API.</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Panel>
    </div>
  );
}

/* ══ 8 · Access & Policy ═══════════════════════════════════════════════ */

function PolicySection() {
  const [p, setP] = useState<Policy | null>(null);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    apiGet<Policy>("/v1/policy")
      .then((d) => setP(structuredClone(d)))
      .catch((e) => toast.error(e instanceof Error ? e.message : "Could not load policy"));
  }, []);

  if (!p) return <Panel className="p-6 text-sm text-gray-500">Loading policy…</Panel>;

  const set = (fn: (q: Policy) => void) => {
    const n = structuredClone(p);
    fn(n);
    setP(n);
    setDirty(true);
  };
  const lines = (v: string[]) => v.join("\n");
  const fromLines = (v: string) => v.split("\n").map((s) => s.trim()).filter(Boolean);

  const save = async () => {
    try {
      await apiPut("/v1/policy", p);
      setDirty(false);
      toast.success("Policy saved · Đã lưu chính sách");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not save the policy");
    }
  };

  return (
    <div className="space-y-5">
      <Panel className="p-6">
        <SectionTitle title="Resources" vi="Giới hạn tài nguyên" />
        <div className="mt-5 grid grid-cols-1 gap-5 md:grid-cols-3">
          <Field label="Max CPUs per sandbox" vi="CPU tối đa" hint="0 = no cap">
            <input className={textInput} type="number" min="0" step="0.5" value={p.resources.max_cpus} onChange={(e) => set((q) => (q.resources.max_cpus = Number(e.target.value)))} />
          </Field>
          <Field label="Max memory (MiB)" vi="Bộ nhớ tối đa" hint="0 = no cap">
            <input className={textInput} type="number" min="0" value={p.resources.max_memory_mb} onChange={(e) => set((q) => (q.resources.max_memory_mb = Number(e.target.value)))} />
          </Field>
          <Field label="Max ports / sandbox" vi="Cổng tối đa">
            <input className={textInput} type="number" min="1" value={p.resources.max_ports_per_sandbox} onChange={(e) => set((q) => (q.resources.max_ports_per_sandbox = Number(e.target.value)))} />
          </Field>
        </div>
        <div className="mt-5 grid grid-cols-1 gap-5 md:grid-cols-2">
          <Field label="Allowed images (1 per line)" vi="Ảnh cho phép">
            <textarea className={cn(textInput, "h-24 py-2 font-mono text-xs")} value={lines(p.images.allowed)} onChange={(e) => set((q) => (q.images.allowed = fromLines(e.target.value)))} placeholder="alpine:3.20" />
          </Field>
          <Field label="Port ranges (min-max per line)" vi="Dải cổng">
            <textarea className={cn(textInput, "h-24 py-2 font-mono text-xs")} value={p.ports.ranges.map((r) => `${r.min}-${r.max}`).join("\n")} onChange={(e) => set((q) => (q.ports.ranges = fromLines(e.target.value).map((s) => { const [a, b] = s.split("-").map((x) => Number(x.trim())); return { min: a, max: b ?? a }; })))} placeholder="1024-65535" />
          </Field>
        </div>
      </Panel>

      <Panel className="p-6">
        <SectionTitle title="Egress" vi="Kết nối ra ngoài" />
        <div className="mt-5 flex flex-wrap items-center gap-2">
          {(["strict", "permissive"] as const).map((m) => (
            <button
              key={m}
              onClick={() => set((q) => (q.egress.mode = m))}
              aria-pressed={p.egress.mode === m}
              className={cn(
                "rounded-lg border px-3.5 py-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                p.egress.mode === m ? "border-neon/40 bg-neon/10 text-neon" : "border-line text-gray-400 hover:bg-fg/5",
              )}
            >
              {m === "strict" ? "Strict — only instance allowlist" : "Permissive — sandboxes allow any host"}
            </button>
          ))}
        </div>
        <div className="mt-5 max-w-md">
          <Field label="Instance egress allowlist (1 per line)" vi="Danh sách cho phép">
            <textarea className={cn(textInput, "h-24 py-2 font-mono text-xs")} value={lines(p.egress.allow)} onChange={(e) => set((q) => (q.egress.allow = fromLines(e.target.value)))} placeholder={"github.com\n*.npmjs.org"} />
          </Field>
        </div>
      </Panel>

      <Panel className="p-6">
        <SectionTitle title="Lifecycle" vi="Vòng đời sandbox" />
        <div className="mt-2">
          <ToggleRow title="Allow keep-alive sandboxes" vi="Cho phép keep-alive" desc="Sandboxes can opt out of idle suspension." on={p.lifecycle.allow_keep_alive} onChange={(v) => set((q) => (q.lifecycle.allow_keep_alive = v))} />
        </div>
        <div className="mt-5 max-w-xs">
          <Field label="Max idle timeout (seconds)" vi="Thời hạn chờ tối đa" hint="0 = no cap">
            <input className={textInput} type="number" min="0" value={p.lifecycle.max_idle_timeout_secs} onChange={(e) => set((q) => (q.lifecycle.max_idle_timeout_secs = Number(e.target.value)))} />
          </Field>
        </div>
        <div className="mt-6 flex items-center gap-3">
          <button className={btnNeon} onClick={save} disabled={!dirty}>
            <Save className="size-4" /> Save policy
          </button>
          <button
            className={btnGhost}
            onClick={() => {
              apiGet<Policy>("/v1/policy").then((d) => {
                setP(structuredClone(d));
                setDirty(false);
              });
            }}
            disabled={!dirty}
          >
            <RefreshCw className="size-4" /> Reset
          </button>
          {dirty && <Mono className="text-xs text-warn">unsaved changes</Mono>}
        </div>
      </Panel>
    </div>
  );
}

/* ══ 9 · Approvals → first-class page (/approvals) ═══════════════════

   The gated-action queue moved out of settings into its own ops page with
   filters, counters and a decision dialog. See views/approvals.tsx. */

/* ══ 10 · Audit Log ════════════════════════════════════════════════════ */

function AuditSection() {
  const [action, setAction] = useState("");
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [verify, setVerify] = useState<{ ok: boolean; checked: number } | null>(null);

  useEffect(() => {
    const q = new URLSearchParams({ limit: "30" });
    if (action.trim()) q.set("action", action.trim());
    apiGet<{ entries: AuditEntry[] }>(`/v1/audit?${q}`)
      .then((d) => setEntries(d.entries))
      .catch((e) => toast.error(e instanceof Error ? e.message : "Could not load audit log"));
    apiGet<{ ok: boolean; checked: number }>("/v1/audit/verify").then(setVerify).catch(() => {});
  }, [action]);

  return (
    <Panel className="p-6">
      <SectionTitle
        title="Audit trail"
        vi="Nhật ký hệ thống"
        right={
          verify && (
            <span className={cn("inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 font-mono text-[10px]", verify.ok ? "border-neon/30 bg-neon/10 text-neon" : "border-danger/30 bg-danger/10 text-danger")}>
              <FileClock className="size-3" /> chain {verify.ok ? "verified" : "broken"} · {verify.checked}
            </span>
          )
        }
      />
      <div className="mt-4 max-w-sm">
        <Input className="h-9 border-line bg-navy/60 text-sm" placeholder="Filter by action, e.g. sandbox.create" value={action} onChange={(e) => setAction(e.target.value)} />
      </div>
      <div className="mt-4 overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wider text-gray-600">
              <th className="pb-2 pr-4 font-semibold">Time</th>
              <th className="pb-2 pr-4 font-semibold">Actor</th>
              <th className="pb-2 pr-4 font-semibold">Action</th>
              <th className="pb-2 pr-4 font-semibold">Target</th>
              <th className="pb-2 font-semibold">Detail</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line-soft">
            {(entries ?? []).map((e) => (
              <tr key={e.id} className="text-gray-200">
                <td className="py-2.5 pr-4"><Mono className="text-xs text-gray-500">{new Date(e.at * 1000).toLocaleTimeString()}</Mono></td>
                <td className="py-2.5 pr-4 text-xs">{e.actor_name ?? e.actor_kind}</td>
                <td className="py-2.5 pr-4"><Mono className="text-xs text-info">{e.action}</Mono></td>
                <td className="py-2.5 pr-4"><Mono className="text-xs text-gray-400">{e.target ?? "—"}</Mono></td>
                <td className="py-2.5 text-xs text-gray-500">{e.detail ?? "—"}</td>
              </tr>
            ))}
            {!entries && (
              <tr>
                <td colSpan={5} className="py-4 text-sm text-gray-500">Loading audit log…</td>
              </tr>
            )}
            {entries && entries.length === 0 && (
              <tr>
                <td colSpan={5} className="py-4 text-sm text-gray-500">No matching entries · Không có mục nào.</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </Panel>
  );
}

/* ══ 11 · Domain & TLS ═════════════════════════════════════════════════ */

function DomainSection() {
  const [info, setInfo] = useState<DomainInfo | null>(null);
  const [form, setForm] = useState({ domain: "", tls_mode: "off", acme_email: "" });
  const [ver, setVer] = useState<{ ok: boolean; hint?: string; checks: { host: string; addrs: string[]; ok: boolean }[] } | null>(null);

  useEffect(() => {
    apiGet<DomainInfo>("/v1/domain")
      .then((d) => {
        setInfo(d);
        setForm({ domain: d.domain, tls_mode: d.tls_mode, acme_email: d.acme_email ?? "" });
      })
      .catch((e) => toast.error(e instanceof Error ? e.message : "Could not load domain"));
  }, []);

  const save = async () => {
    try {
      const d = await apiPut<DomainInfo>("/v1/domain", { domain: form.domain, tls_mode: form.tls_mode, ...(form.acme_email ? { acme_email: form.acme_email } : {}) });
      setInfo(d);
      toast.success("Domain saved · Đã lưu tên miền", { description: "A control-plane restart applies the new domain." });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not save the domain");
    }
  };

  const verify = async () => {
    try {
      const r = await apiPost<{ ok: boolean; hint?: string; checks: { host: string; addrs: string[]; ok: boolean }[] }>("/v1/domain/verify", {});
      setVer(r);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Verify failed");
    }
  };

  if (!info) return <Panel className="p-6 text-sm text-gray-500">Loading domain…</Panel>;

  return (
    <div className="space-y-5">
      <Panel className="p-6">
        <SectionTitle title="Custom domain" vi="Tên miền tùy chỉnh" />
        <div className="mt-5 grid grid-cols-1 gap-5 md:grid-cols-3">
          <Field label="Domain" vi="Tên miền" hint="Empty = console host only.">
            <input className={textInput} value={form.domain} onChange={(e) => setForm({ ...form, domain: e.target.value })} placeholder="console.4aicoding.dev" />
          </Field>
          <Field label="TLS mode" vi="Chế độ TLS">
            <FancySelect
              ariaLabel="TLS mode"
              value={form.tls_mode}
              onChange={(v) => setForm({ ...form, tls_mode: v })}
              options={[
                { value: "off", label: "off — plain HTTP", vi: "không mã hoá" },
                { value: "acme", label: "acme — Let's Encrypt", vi: "tự động" },
                { value: "cert", label: "cert — own certificate", vi: "chứng chỉ riêng" },
              ]}
            />
          </Field>
          <Field label="ACME email" vi="Email ACME" hint="Only used by the acme TLS mode.">
            <input className={textInput} type="email" value={form.acme_email} onChange={(e) => setForm({ ...form, acme_email: e.target.value })} placeholder="ops@4aicoding.dev" />
          </Field>
        </div>
        <div className="mt-6 flex flex-wrap items-center gap-3">
          <button className={btnNeon} onClick={save}>
            <Save className="size-4" /> Save domain
          </button>
          <button className={btnGhost} onClick={verify}>
            <RefreshCw className="size-4" /> Verify DNS
          </button>
          {info.restart_required && (
            <Mono className="text-xs text-warn">restart required · cần khởi động lại</Mono>
          )}
        </div>
        {ver && (
          <div className="mt-4 rounded-xl border border-line bg-navy/50 p-4">
            {ver.hint && <p className="mb-2 text-xs text-gray-500">{ver.hint}</p>}
            {ver.checks.map((c) => (
              <div key={c.host} className="flex items-center gap-2 py-1 text-sm">
                {c.ok ? <CheckCircle2 className="size-4 text-neon" /> : <XCircle className="size-4 text-danger" />}
                <Mono className="text-xs text-gray-300">{c.host}</Mono>
                <Mono className="text-xs text-gray-500">{c.addrs.join(", ")}</Mono>
              </div>
            ))}
          </div>
        )}
      </Panel>
      <Panel className="p-6">
        <SectionTitle title="Running configuration" vi="Cấu hình đang chạy" />
        <div className="mt-3 space-y-2 text-sm">
          <div className="flex justify-between border-b border-line-soft py-2">
            <Mono className="text-xs text-gray-500">console host</Mono>
            <Mono className="text-xs text-gray-200">{info.console_host || "—"}</Mono>
          </div>
          <div className="flex justify-between border-b border-line-soft py-2">
            <Mono className="text-xs text-gray-500">running domain</Mono>
            <Mono className="text-xs text-gray-200">{info.running.domain || "(console host)"}</Mono>
          </div>
          <div className="flex justify-between py-2">
            <Mono className="text-xs text-gray-500">preview URL template</Mono>
            <Mono className="text-xs text-info">{info.preview_example || "—"}</Mono>
          </div>
        </div>
      </Panel>
    </div>
  );
}

/* ══ 12 · Danger Zone ══════════════════════════════════════════════════ */

function DangerSection() {
  const signOut = usePlatform((s) => s.signOut);

  return (
    <Panel className="border-danger/30 p-6">
      <SectionTitle title="Irreversible actions" vi="Hành động không thể hoàn tác" />
      <div className="mt-3 divide-y divide-line-soft">
        <div className="flex flex-wrap items-center justify-between gap-3 py-4">
          <div>
            <div className="text-sm text-gray-200">Sign out everywhere</div>
            <p className="mt-0.5 text-xs text-gray-500">Revoke this browser session and return to the login screen · Đăng xuất khỏi mọi thiết bị.</p>
          </div>
          <button className={btnGhost} onClick={() => void signOut()}>
            <LogOut className="size-4" /> Sign out
          </button>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-3 py-4">
          <div>
            <div className="text-sm text-gray-200">Reset local console preferences</div>
            <p className="mt-0.5 text-xs text-gray-500">Clears theme, profile, notification and session preferences stored in this browser · Xóa các tùy chọn cục bộ.</p>
          </div>
          <button
            className={btnDanger}
            onClick={() => {
              const keys = ["sbx-set-general", "sbx-set-density", "sbx-set-notify", "sbx-set-profile", "sbx-set-security", "sbx-set-sessions"];
              keys.forEach((k) => localStorage.removeItem(k));
              toast.success("Local preferences reset · Đã xóa tùy chọn cục bộ");
            }}
          >
            <TriangleAlert className="size-4" /> Reset preferences
          </button>
        </div>
      </div>
    </Panel>
  );
}

/* ══ shell ═════════════════════════════════════════════════════════════ */

export function SettingsView() {
  const [section, setSection] = useState<SectionId>(() => {
    const p = typeof window !== "undefined" ? window.location.pathname : "";
    return settingsSectionFromPath(p) ?? DEFAULT_SECTION;
  });

  useEffect(() => {
    // keep the URL in sync: /settings/<section>
    const seg = window.location.pathname.split("/").filter(Boolean);
    if (seg[0] !== "settings" || seg[1] !== section) {
      history.replaceState(null, "", section === DEFAULT_SECTION ? "/settings" : `/settings/${section}`);
    }
  }, [section]);

  useEffect(() => {
    const on = () => {
      const s = settingsSectionFromPath(window.location.pathname);
      if (window.location.pathname.split("/").filter(Boolean)[0] === "settings") setSection(s ?? DEFAULT_SECTION);
    };
    window.addEventListener("popstate", on);
    return () => window.removeEventListener("popstate", on);
  }, []);

  const meta = useMemo(() => SECTIONS.find((s) => s.id === section)!, [section]);

  const groups: Array<"workspace" | "account" | "system"> = ["workspace", "account", "system"];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Settings"
        subtitle="Instance, account and system preferences"
        vi="Cài đặt hệ thống, tài khoản và phiên bản"
        actions={
          <span className="inline-flex items-center gap-2 rounded-lg border border-line bg-surface px-3 py-1.5">
            <span className="size-1.5 rounded-full bg-neon dot-live" />
            <Mono className="text-xs text-gray-400">{meta.label}</Mono>
            <span className="text-[11px] text-gray-600">· {meta.vi}</span>
          </span>
        }
      />

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[248px_1fr]">
        {/* ── settings sub-menu rail ─────────────────────────────── */}
        <nav aria-label="Settings" className="lg:sticky lg:top-[84px] lg:self-start">
          <Panel className="overflow-hidden p-2">
            {groups.map((g) => (
              <div key={g} className="py-1 first:pt-1">
                <div className="px-3 pt-3 pb-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-gray-600">
                  {GROUP_LABEL[g][0]} <span className="text-gray-700">· {GROUP_LABEL[g][1]}</span>
                </div>
                {SECTIONS.filter((s) => s.group === g).map((s) => {
                  const Icon = s.icon;
                  const active = s.id === section;
                  return (
                    <button
                      key={s.id}
                      onClick={() => setSection(s.id)}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "group relative flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring",
                        active ? "bg-neon/10 font-medium text-ink" : "text-gray-400 hover:bg-fg/[0.04] hover:text-gray-200",
                        s.id === "danger" && !active && "text-danger/80 hover:text-danger",
                      )}
                    >
                      <span
                        className={cn(
                          "absolute left-0 top-1/2 h-6 w-[3px] -translate-y-1/2 rounded-full transition-all",
                          active ? "bg-neon shadow-[0_0_8px_var(--glow-live)]" : "bg-transparent",
                        )}
                      />
                      <Icon className={cn("size-[18px] shrink-0 transition-colors", active ? "text-neon" : s.id === "danger" ? "text-danger/70" : "text-gray-500 group-hover:text-gray-300")} />
                      <span className="flex-1 text-left">{s.label}</span>
                      <span className={cn("text-[10px] transition-opacity", active ? "text-neon/70 opacity-100" : "text-gray-600 opacity-0 group-hover:opacity-100")}>
                        {s.vi}
                      </span>
                    </button>
                  );
                })}
              </div>
            ))}
          </Panel>
        </nav>

        {/* ── section content ────────────────────────────────────── */}
        <div className="min-w-0 space-y-5">
          {section === "general" && <GeneralSection />}
          {section === "appearance" && <AppearanceSection />}
          {section === "notifications" && <NotificationsSection />}
          {section === "profile" && <ProfileSection />}
          {section === "security" && <SecuritySection />}
          {section === "sessions" && <SessionsSection />}
          {section === "api-keys" && <ApiKeysSection />}
          {section === "policy" && <PolicySection />}
          {section === "audit" && <AuditSection />}
          {section === "domain" && <DomainSection />}
          {section === "danger" && <DangerSection />}
        </div>
      </div>
    </div>
  );
}
