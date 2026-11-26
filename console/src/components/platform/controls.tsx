"use client";

/* ── Platform form controls ───────────────────────────────────────────
   Shared, fully styled controls used across the platform views so every
   page gets the same behaviour:
   · ViewTabs    — in-page tabs that switch content with a sliding
                   underline (pure client state, NO routing, no page jump)
   · CheckBox    — themed checkbox (dark border, neon check, indeterminate)
   · FancySelect — Radix dropdown styled for the SBX dark surface
   · TextField / TextArea — consistent inputs
   · ModeCard    — radio-style selectable card (network modes, density…)
   · TableShell  — responsive wrapper that gives every table a horizontal
                   scroll on narrow screens instead of breaking layout   */

import { motion } from "framer-motion";
import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { type ReactNode } from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import { cn } from "@/lib/utils";

/* ══ ViewTabs — smooth client-side tab bar ════════════════════════════ */

export function ViewTabs<T extends string>({
  tabs,
  value,
  onChange,
  className,
}: {
  tabs: Array<{ id: T; label: string; vi?: string; count?: number; icon?: typeof CheckIcon; danger?: boolean }>;
  value: T;
  onChange: (v: T) => void;
  className?: string;
}) {
  return (
    <div
      role="tablist"
      aria-orientation="horizontal"
      className={cn(
        "flex min-w-full gap-1 overflow-x-auto border-b border-line pb-px [scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
        className,
      )}
    >
      {tabs.map((t) => {
        const active = t.id === value;
        const Icon = t.icon;
        return (
          <button
            key={t.id}
            role="tab"
            aria-selected={active}
            onClick={() => onChange(t.id)}
            className={cn(
              "relative flex shrink-0 items-center gap-2 rounded-t-lg px-3 py-2.5 text-[13px] font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring",
              active ? "text-ink" : "text-gray-400 hover:bg-fg/[0.03] hover:text-gray-200",
              t.danger && !active && "hover:text-danger",
            )}
          >
            {Icon && <Icon className={cn("size-4 transition-colors", active ? "text-neon" : "text-gray-500")} />}
            {t.label}
            {typeof t.count === "number" && (
              <span
                className={cn(
                  "rounded-full border px-1.5 py-px font-mono text-[10px] leading-4 transition-colors",
                  active ? "border-neon/40 bg-neon/10 text-neon" : "border-line text-gray-500",
                )}
              >
                {t.count}
              </span>
            )}
            {t.vi && <span className={cn("hidden text-[11px] font-normal transition-opacity lg:inline", active ? "text-gray-500" : "text-gray-600")}>{t.vi}</span>}
            {active && (
              <motion.span
                layoutId="view-tabs-underline"
                className="absolute inset-x-2 -bottom-px h-[2.5px] rounded-full bg-neon shadow-[0_0_10px_var(--glow-live)]"
                transition={{ type: "spring", stiffness: 500, damping: 38 }}
              />
            )}
          </button>
        );
      })}
    </div>
  );
}

/* ══ CheckBox ══════════════════════════════════════════════════════════ */

export function CheckBox({
  checked,
  onChange,
  indeterminate,
  disabled,
  label,
  vi,
  className,
  ariaLabel,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  indeterminate?: boolean;
  disabled?: boolean;
  label?: ReactNode;
  vi?: string;
  className?: string;
  ariaLabel?: string;
}) {
  const box = (
    <button
      type="button"
      role="checkbox"
      aria-checked={indeterminate ? "mixed" : checked}
      aria-label={ariaLabel ?? (typeof label === "string" ? label : undefined)}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        "grid size-[18px] shrink-0 place-items-center rounded-[5px] border transition-all outline-none focus-visible:ring-2 focus-visible:ring-ring",
        checked || indeterminate
          ? "border-neon bg-neon text-navy shadow-[0_0_8px_var(--glow-mid)]"
          : "border-gray-600 bg-navy/60 hover:border-gray-400",
        disabled && "pointer-events-none opacity-40",
      )}
    >
      {indeterminate ? (
        <span className="h-0.5 w-2.5 rounded-full bg-navy" />
      ) : checked ? (
        <CheckIcon className="size-3" strokeWidth={3.5} />
      ) : null}
    </button>
  );
  if (!label && !vi) return <span className={className}>{box}</span>;
  return (
    <label className={cn("flex cursor-pointer items-center gap-2.5 select-none", disabled && "cursor-default opacity-60", className)}>
      {box}
      {label && <span className="text-sm text-gray-300">{label}</span>}
      {vi && <span className="text-[11px] text-gray-600">· {vi}</span>}
    </label>
  );
}

/* ══ FancySelect — themed Radix dropdown ══════════════════════════════ */

const selectTrigger =
  "flex h-10 w-full min-w-0 items-center justify-between gap-2 rounded-lg border border-line bg-navy/60 px-3 text-sm text-gray-100 outline-none transition-colors data-[placeholder]:text-gray-600 focus-visible:ring-2 focus-visible:ring-ring hover:bg-navy/80 disabled:cursor-not-allowed disabled:opacity-40 [&>span:last-of-type]:shrink-0 [&>span:first-of-type]:truncate";

export function FancySelect({
  value,
  onChange,
  options,
  placeholder,
  disabled,
  className,
  ariaLabel,
  groups,
}: {
  value: string;
  onChange: (v: string) => void;
  options: Array<{ value: string; label: string; vi?: string; disabled?: boolean }>;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  ariaLabel?: string;
  groups?: Array<{ label: string; items: Array<{ value: string; label: string; vi?: string; disabled?: boolean }> }>;
}) {
  const found = [...options, ...(groups ?? []).flatMap((g) => g.items)].find((o) => o.value === value);
  return (
    <SelectPrimitive.Root value={value} onValueChange={onChange} disabled={disabled}>
      <SelectPrimitive.Trigger aria-label={ariaLabel} className={cn(selectTrigger, className)}>
        <span className={cn("truncate", !found && "text-gray-600")}>{found ? found.label : (placeholder ?? "Select…")}</span>
        <SelectPrimitive.Icon asChild>
          <ChevronDownIcon className="size-4 shrink-0 text-gray-500" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          position="popper"
          sideOffset={6}
          className="z-50 max-h-72 min-w-[var(--radix-select-trigger-width)] overflow-hidden rounded-xl border border-line bg-[#0d1424] shadow-[0_12px_40px_rgba(0,0,0,0.5)] [scrollbar-width:thin]"
        >
          <SelectPrimitive.Viewport className="p-1.5">
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value} disabled={o.disabled} vi={o.vi}>
                {o.label}
              </SelectItem>
            ))}
            {groups?.map((g) => (
              <div key={g.label}>
                <div className="px-2.5 pb-1 pt-2.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-gray-600">{g.label}</div>
                {g.items.map((o) => (
                  <SelectItem key={o.value} value={o.value} disabled={o.disabled} vi={o.vi}>
                    {o.label}
                  </SelectItem>
                ))}
              </div>
            ))}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}

function SelectItem({
  children,
  value,
  vi,
  disabled,
}: {
  children: ReactNode;
  value: string;
  vi?: string;
  disabled?: boolean;
}) {
  return (
    <SelectPrimitive.Item
      value={value}
      disabled={disabled}
      className="flex cursor-pointer items-center justify-between gap-3 rounded-lg px-2.5 py-2 text-sm text-gray-200 outline-none transition-colors data-[disabled]:pointer-events-none data-[disabled]:opacity-40 data-[highlighted]:bg-neon/10 data-[highlighted]:text-neon data-[state=checked]:text-neon"
    >
      <span className="flex min-w-0 flex-1 flex-col">
        <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
        {vi && <span className="text-[10.5px] text-gray-600">{vi}</span>}
      </span>
      <SelectPrimitive.ItemIndicator>
        <CheckIcon className="size-3.5 shrink-0 text-neon" />
      </SelectPrimitive.ItemIndicator>
    </SelectPrimitive.Item>
  );
}

/* ══ TextField / TextArea ═════════════════════════════════════════════ */

export const fieldCls =
  "h-10 w-full min-w-0 rounded-lg border border-line bg-navy/60 px-3 text-sm text-gray-100 placeholder:text-gray-600 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring hover:bg-navy/80 disabled:cursor-not-allowed disabled:opacity-40";

export function TextField({
  value,
  onChange,
  placeholder,
  type = "text",
  disabled,
  className,
  ariaLabel,
  min,
  max,
  step,
  mono,
  autoFocus,
  onKeyDown,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  type?: string;
  disabled?: boolean;
  className?: string;
  ariaLabel?: string;
  min?: number;
  max?: number;
  step?: number;
  mono?: boolean;
  autoFocus?: boolean;
  onKeyDown?: (e: React.KeyboardEvent<HTMLInputElement>) => void;
}) {
  return (
    <input
      type={type}
      value={value}
      min={min}
      max={max}
      step={step}
      autoFocus={autoFocus}
      aria-label={ariaLabel}
      disabled={disabled}
      placeholder={placeholder}
      onKeyDown={onKeyDown}
      onChange={(e) => onChange(e.target.value)}
      className={cn(fieldCls, mono && "font-mono", className)}
    />
  );
}

export function TextArea({
  value,
  onChange,
  placeholder,
  rows = 4,
  disabled,
  className,
  mono,
  ariaLabel,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  rows?: number;
  disabled?: boolean;
  className?: string;
  mono?: boolean;
  ariaLabel?: string;
}) {
  return (
    <textarea
      rows={rows}
      value={value}
      aria-label={ariaLabel}
      disabled={disabled}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      className={cn(
        "w-full rounded-lg border border-line bg-navy/60 px-3 py-2.5 text-sm leading-relaxed text-gray-100 placeholder:text-gray-600 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring hover:bg-navy/80 disabled:cursor-not-allowed disabled:opacity-40",
        mono && "font-mono text-[12.5px]",
        className,
      )}
    />
  );
}

/* ══ Field row (label + control + hint, responsive) ═══════════════════ */

export function FieldRow({
  label,
  vi,
  hint,
  children,
  className,
}: {
  label: string;
  vi?: string;
  hint?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex min-w-0 flex-col gap-1.5", className)}>
      <span className="text-xs font-medium text-gray-300">
        {label}
        {vi && <span className="ml-1.5 text-[11px] text-gray-600">· {vi}</span>}
      </span>
      {children}
      {hint && <span className="text-[11px] leading-relaxed text-gray-600">{hint}</span>}
    </div>
  );
}

/* ══ ModeCard — radio-style selectable card ═══════════════════════════ */

export function ModeCard({
  title,
  desc,
  vi,
  selected,
  onSelect,
  disabled,
  tone = "neon",
}: {
  title: string;
  desc: string;
  vi?: string;
  selected: boolean;
  onSelect: () => void;
  disabled?: boolean;
  tone?: "neon" | "info" | "warn" | "danger";
}) {
  const ring = {
    neon: "border-neon/60 bg-neon/[0.07] shadow-[inset_0_0_0_1px_var(--glow-mid)]",
    info: "border-info/60 bg-info/[0.07] shadow-[inset_0_0_0_1px_rgba(56,189,248,0.25)]",
    warn: "border-warn/60 bg-warn/[0.07]",
    danger: "border-danger/60 bg-danger/[0.07]",
  }[tone];
  const dot = { neon: "bg-neon", info: "bg-info", warn: "bg-warn", danger: "bg-danger" }[tone];
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      disabled={disabled}
      onClick={onSelect}
      className={cn(
        "flex items-start gap-3 rounded-xl border p-4 text-left transition-all outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40",
        selected ? ring : "border-line bg-surface hover:border-gray-500 hover:bg-fg/[0.02]",
      )}
    >
      <span
        className={cn(
          "mt-0.5 grid size-[18px] shrink-0 place-items-center rounded-full border transition-all",
          selected ? cn(dot, "border-transparent") : "border-gray-600 bg-navy/60",
        )}
      >
        {selected && <span className="size-1.5 rounded-full bg-navy" />}
      </span>
      <span className="min-w-0">
        <span className={cn("block text-sm font-semibold", selected ? "text-ink" : "text-gray-200")}>{title}</span>
        {vi && <span className="mt-0.5 block text-[11px] text-gray-500">{vi}</span>}
        <span className="mt-1 block text-xs leading-relaxed text-gray-500">{desc}</span>
      </span>
    </button>
  );
}

/* ══ TableShell — responsive table wrapper ════════════════════════════ */

export function TableShell({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        "-mx-1 overflow-x-auto rounded-xl border border-line [scrollbar-width:thin] [&::-webkit-scrollbar]:h-1.5 [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-gray-700",
        className,
      )}
    >
      <table className="w-full min-w-[560px] border-collapse text-sm">{children}</table>
    </div>
  );
}

export function Th({ children, className }: { children?: ReactNode; className?: string }) {
  return (
    <th
      className={cn(
        "border-b border-line bg-navy/70 px-3.5 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500 first:rounded-tl-xl last:rounded-tr-xl",
        className,
      )}
    >
      {children}
    </th>
  );
}

export function Td({ children, className }: { children?: ReactNode; className?: string }) {
  return (
    <td className={cn("border-b border-line-soft px-3.5 py-2.5 align-middle text-[13px] text-gray-200 last:border-0", className)}>
      {children}
    </td>
  );
}
