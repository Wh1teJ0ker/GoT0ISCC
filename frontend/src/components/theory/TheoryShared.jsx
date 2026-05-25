import {Badge} from "../ui/Badge";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../ui/Card";
import {cn} from "../../lib/utils";
import {displayValue} from "./utils";

export const selectClassName =
  "flex h-10 w-full rounded-xl border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export const textareaClassName =
  "w-full rounded-2xl border border-input bg-background px-3 py-3 text-sm leading-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export function SummaryCard({label, value, note, icon: Icon, tone = "slate"}) {
  const toneMap = {
    emerald: "border-emerald-100 bg-emerald-50/85 text-emerald-700",
    amber: "border-amber-100 bg-amber-50/85 text-amber-700",
    blue: "border-cyan-100 bg-cyan-50/85 text-cyan-700",
    slate: "border-zinc-200 bg-white text-zinc-700",
  };

  return (
    <Card className={cn("overflow-hidden shadow-sm", toneMap[tone])}>
      <CardHeader className="flex-row items-start justify-between space-y-0 p-3">
        <div className="min-w-0">
          <CardDescription className="text-xs">{label}</CardDescription>
          <CardTitle className="mt-1.5 truncate text-xl">{displayValue(value)}</CardTitle>
          <div className="mt-1 line-clamp-1 text-[11px] leading-4 text-current/75">{displayValue(note)}</div>
        </div>
        <div className="rounded-lg bg-black/5 p-1.5">
          <Icon className="h-3.5 w-3.5" />
        </div>
      </CardHeader>
    </Card>
  );
}

export function SurfaceMetric({label, value, note, icon: Icon}) {
  return (
    <div className="rounded-[22px] border border-white/10 bg-white/7 p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-slate-300">{label}</div>
          <div className="mt-2 text-lg font-semibold text-white">{displayValue(value)}</div>
        </div>
        <div className="rounded-xl bg-white/10 p-2 text-cyan-100">
          <Icon className="h-4 w-4" />
        </div>
      </div>
      <div className="mt-2 text-xs leading-5 text-slate-300">{displayValue(note)}</div>
    </div>
  );
}

export function QuickInfo({label, value}) {
  return (
    <div className="rounded-2xl border border-zinc-200 bg-zinc-50/90 px-4 py-3">
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">{label}</div>
      <div className="mt-2 text-sm font-semibold text-zinc-950">{displayValue(value)}</div>
    </div>
  );
}

export function StatusPill({status, label}) {
  const statusMap = {
    matched: "border-emerald-200 bg-emerald-50 text-emerald-800",
    exact: "border-emerald-200 bg-emerald-50 text-emerald-800",
    weak: "border-amber-200 bg-amber-50 text-amber-800",
    pending: "border-zinc-200 bg-zinc-100 text-zinc-700",
    ai: "border-cyan-200 bg-cyan-50 text-cyan-800",
    failed: "border-rose-200 bg-rose-50 text-rose-800",
  };

  return (
    <div className={cn("rounded-full border px-3 py-1 text-xs font-semibold", statusMap[status] || statusMap.pending)}>
      {label}
    </div>
  );
}

export function InfoTile({label, value, note, icon: Icon, tone = "slate"}) {
  const toneMap = {
    emerald: "border-emerald-100 bg-emerald-50/85",
    amber: "border-amber-100 bg-amber-50/85",
    blue: "border-cyan-100 bg-cyan-50/85",
    slate: "border-zinc-200 bg-white",
  };

  return (
    <div className={cn("rounded-xl border p-3 shadow-sm", toneMap[tone])}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-xs font-semibold text-muted-foreground">{label}</div>
          <div className="mt-1.5 break-all text-sm font-semibold text-zinc-950">{displayValue(value)}</div>
          <div className="mt-0.5 text-[11px] leading-4 text-muted-foreground">{displayValue(note)}</div>
        </div>
        <div className="rounded-lg bg-black/5 p-1.5 text-zinc-700">
          <Icon className="h-3.5 w-3.5" />
        </div>
      </div>
    </div>
  );
}

export function OptionCard({item, active, aiPicked}) {
  return (
    <div
      className={cn(
        "rounded-[22px] border px-4 py-4 text-sm shadow-sm transition-colors",
        active ? "border-emerald-300 bg-emerald-50/80 text-emerald-950" : "border-zinc-200 bg-white text-zinc-900",
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-black/5 font-semibold">{item.key}</div>
        {active ? <Badge variant="outline">题库命中</Badge> : null}
        {aiPicked ? <Badge variant="outline">AI 推荐</Badge> : null}
      </div>
      <div className="mt-3 leading-6">{item.content}</div>
    </div>
  );
}

export function MiniLine({label, value}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-white/8 pb-3 last:border-b-0 last:pb-0">
      <span className="text-xs text-slate-300">{label}</span>
      <span className="max-w-[60%] truncate text-sm font-medium text-white">{displayValue(value)}</span>
    </div>
  );
}

export function MetaRow({label, value}) {
  return (
    <div className="rounded-lg border border-border bg-background px-3 py-2.5">
      <div className="text-xs font-semibold text-muted-foreground">{label}</div>
      <div className="mt-0.5 break-all text-xs text-zinc-900">{displayValue(value)}</div>
    </div>
  );
}

export function Field({label, children}) {
  return (
    <div className="space-y-1.5">
      <div className="text-xs font-semibold text-muted-foreground">{label}</div>
      {children}
    </div>
  );
}

export function MetaSwitch({label, checked, onChange}) {
  return (
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className={cn(
        "flex h-11 items-center justify-between rounded-xl border px-3 text-left shadow-sm transition-colors",
        checked ? "border-emerald-200 bg-emerald-50 text-emerald-950" : "border-border bg-white text-foreground",
      )}
    >
      <div>
        <div className="text-xs font-semibold text-muted-foreground">{label}</div>
        <div className="mt-1 text-sm">{checked ? "已启用" : "未启用"}</div>
      </div>
      <div className={cn("h-3 w-3 rounded-full", checked ? "bg-emerald-500" : "bg-zinc-300")} />
    </button>
  );
}

export function Alert({text, tone = "destructive"}) {
  return (
    <div
      className={cn(
        "rounded-xl border px-4 py-3 text-sm",
        tone === "destructive" ? "border-destructive/40 bg-destructive/10 text-destructive" : "border-primary/30 bg-primary/10 text-primary",
      )}
    >
      {text}
    </div>
  );
}

export function EmptyState({text, compact = false}) {
  return (
    <div
      className={cn(
        "rounded-xl border border-dashed border-border bg-muted/20 text-center text-sm text-muted-foreground",
        compact ? "p-4" : "p-8",
      )}
    >
      {text}
    </div>
  );
}
