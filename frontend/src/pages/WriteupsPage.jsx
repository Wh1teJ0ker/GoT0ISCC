import {useEffect, useMemo, useState} from "react";
import {
  AlertTriangle,
  CheckCircle2,
  Cloud,
  FileQuestion,
  FileText,
  Filter,
  History,
  Download,
  RefreshCcw,
  Search,
  ShieldAlert,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import {SyncWriteups, Writeups} from "../../wailsjs/go/desktop/API";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {Badge} from "../components/ui/Badge";
import {Button} from "../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../components/ui/Card";
import {Input} from "../components/ui/Input";
import {cn} from "../lib/utils";
import {pageMeta} from "../lib/iscc";

const STATUS_FILTERS = [
  {value: "all", label: "全部"},
  {value: "missing", label: "缺交"},
  {value: "needs_fix", label: "异常"},
  {value: "submitted", label: "已提交"},
  {value: "sync_failed", label: "同步失败"},
];

function statusMeta(status) {
  switch (status) {
    case "submitted":
      return {label: "已提交", icon: CheckCircle2, tone: "border-emerald-200 bg-emerald-50 text-emerald-700"};
    case "needs_fix":
      return {label: "需核对", icon: AlertTriangle, tone: "border-amber-200 bg-amber-50 text-amber-700"};
    case "missing":
      return {label: "缺交 WP", icon: XCircle, tone: "border-rose-200 bg-rose-50 text-rose-700"};
    case "sync_failed":
      return {label: "同步失败", icon: ShieldAlert, tone: "border-slate-300 bg-slate-100 text-slate-700"};
    default:
      return {label: "待同步", icon: FileQuestion, tone: "border-zinc-200 bg-zinc-50 text-zinc-700"};
  }
}

function syncMeta(status) {
  if (status === "synced") {
    return {label: "已同步", tone: "border-emerald-200 bg-emerald-50 text-emerald-700"};
  }
  return {label: "未同步", tone: "border-slate-300 bg-slate-100 text-slate-700"};
}

export function WriteupsPage() {
  const meta = pageMeta.writeups;
  const [payload, setPayload] = useState({summary: null, accounts: [], items: [], records: []});
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [accountFilter, setAccountFilter] = useState("all");
  const [selectedKey, setSelectedKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    reload();
  }, []);

  const selected = useMemo(
    () => payload.items.find((item) => item.key === selectedKey) || payload.items[0] || null,
    [payload.items, selectedKey],
  );

  const filtered = useMemo(() => {
    const text = query.trim().toLowerCase();
    return payload.items.filter((item) => {
      const challenge = item.challenge || {};
      if (accountFilter !== "all" && item.account !== accountFilter) {
        return false;
      }
      if (statusFilter !== "all" && item.status !== statusFilter) {
        return false;
      }
      if (!text) {
        return true;
      }
      return [
        item.key,
        item.account,
        item.submit_identity,
        item.expected_filename,
        item.section_label,
        challenge.challenge_id,
        challenge.title,
        ...(item.remote_records || []).map((record) => record.filename),
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(text);
    });
  }, [payload.items, query, statusFilter, accountFilter]);

  const accountRecords = useMemo(() => {
    if (!selected) {
      return [];
    }
    return (payload.records || []).filter((record) => record.account === selected.account);
  }, [payload.records, selected]);

  async function reload() {
    setBusy(true);
    setError("");
    try {
      const result = await Writeups();
      const items = result?.items || [];
      setPayload({
        summary: result?.summary || null,
        accounts: result?.accounts || [],
        items,
        records: result?.records || [],
      });
      if (!items.find((item) => item.key === selectedKey) && items[0]) {
        setSelectedKey(items[0].key);
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  async function syncRemote() {
    setSyncing(true);
    setError("");
    try {
      const result = await SyncWriteups();
      const items = result?.items || [];
      setPayload({
        summary: result?.summary || null,
        accounts: result?.accounts || [],
        items,
        records: result?.records || [],
      });
      if (!items.find((item) => item.key === selectedKey) && items[0]) {
        setSelectedKey(items[0].key);
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setSyncing(false);
    }
  }

  return (
    <PageContainer className="max-w-[1500px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <>
            <Button variant="outline" size="sm" onClick={reload} disabled={busy || syncing}>
              <RefreshCcw className={cn("h-4 w-4", busy && "animate-spin")} />
              刷新本地快照
            </Button>
            <Button size="sm" onClick={syncRemote} disabled={busy || syncing}>
              <Download className={cn("h-4 w-4", syncing && "animate-pulse")} />
              {syncing ? "抓取远端中..." : "抓取远端并保存"}
            </Button>
          </>
        }
      />

      <div className="grid auto-rows-[88px] gap-2 md:grid-cols-2 xl:grid-cols-6">
        <SummaryCard label="应交 WP" value={payload.summary?.total ?? 0} icon={FileText} tone="zinc" />
        <SummaryCard label="远端已交" value={payload.summary?.submitted ?? 0} icon={CheckCircle2} tone="emerald" />
        <SummaryCard label="缺交" value={payload.summary?.missing ?? 0} icon={XCircle} tone="rose" />
        <SummaryCard label="需核对" value={payload.summary?.needs_fix ?? 0} icon={AlertTriangle} tone="amber" />
        <SummaryCard label="远端记录" value={payload.summary?.remote_records ?? 0} icon={History} tone="blue" />
        <SummaryCard label="同步失败" value={payload.summary?.failed_accounts ?? 0} icon={ShieldAlert} tone="slate" />
      </div>

      {error ? (
        <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive shadow-sm">
          {error}
        </div>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-[minmax(380px,0.92fr)_minmax(0,1.45fr)]">
        <aside className="space-y-3">
          <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
            <CardContent className="grid gap-2 p-3">
              <div className="flex min-h-10 items-center justify-between gap-3">
                <div>
                  <CardTitle className="text-sm">远端 WP 监控</CardTitle>
                  <CardDescription className="mt-1 text-xs">
                    {payload.summary?.last_scanned_at ? `本地快照时间：${payload.summary.last_scanned_at}` : "未抓取"}
                  </CardDescription>
                </div>
                <Badge variant="outline" className="h-8 rounded-md bg-white px-3 text-xs font-medium">
                  {filtered.length} / {payload.summary?.total ?? 0}
                </Badge>
              </div>

              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索账号、题号、题名、远端文件名" className="pl-9" />
              </div>

              <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
                <div className="relative">
                  <Filter className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <select
                    className="h-10 w-full appearance-none rounded-md border border-input bg-white pl-9 pr-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    value={accountFilter}
                    onChange={(event) => setAccountFilter(event.target.value)}
                  >
                    <option value="all">全部账号</option>
                    {payload.accounts.map((account) => (
                      <option key={account.account} value={account.account}>{account.account}</option>
                    ))}
                  </select>
                </div>
                <Badge variant="outline" className="flex h-10 items-center rounded-md bg-white px-3 text-xs font-medium">
                  {accountFilter === "all" ? "全部账号" : accountFilter}
                </Badge>
              </div>

              <div className="grid grid-cols-5 gap-2">
                {STATUS_FILTERS.map((item) => (
                  <Button
                    key={item.value}
                    variant={statusFilter === item.value ? "default" : "outline"}
                    size="sm"
                    className="px-2 text-xs"
                    onClick={() => setStatusFilter(item.value)}
                  >
                    {item.label}
                  </Button>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
            <CardHeader className="border-b border-border/70 px-3 py-3">
              <div className="flex min-h-10 items-center justify-between gap-3">
                <div>
                  <CardTitle className="text-sm">账号概览</CardTitle>
                </div>
                <Cloud className="h-4 w-4 text-muted-foreground" />
              </div>
            </CardHeader>
            <CardContent className="log-session-list max-h-[250px] space-y-2 overflow-y-auto p-2">
              {payload.accounts.map((account) => (
                <button
                  key={account.account}
                  className={cn(
                    "grid w-full grid-cols-[minmax(0,1fr)_auto] gap-3 rounded-md border px-3 py-2 text-left text-sm transition-colors",
                    accountFilter === account.account ? "border-zinc-900 bg-zinc-950 text-zinc-100" : "border-border bg-white hover:bg-zinc-50",
                  )}
                  onClick={() => setAccountFilter(account.account)}
                >
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{account.account}</span>
                    <span className={cn("mt-0.5 block truncate text-[11px]", accountFilter === account.account ? "text-zinc-400" : "text-muted-foreground")}>
                      {account.submit_identity || "未配置提交身份"} / 远端 {account.remote_records || 0}
                    </span>
                  </span>
                  <span className={cn("text-xs", accountFilter === account.account ? "text-zinc-400" : "text-muted-foreground")}>
                    缺 {account.missing} / 异 {account.needs_fix} / 交 {account.submitted}
                  </span>
                </button>
              ))}
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
            <CardHeader className="border-b border-border/70 px-3 py-3">
              <div className="flex min-h-10 items-center justify-between gap-3">
                <div>
                  <CardTitle className="text-sm">已解题清单</CardTitle>
                  <CardDescription className="mt-1 text-xs">练武题和擂台题都按账号检查</CardDescription>
                </div>
                <Filter className="h-4 w-4 text-muted-foreground" />
              </div>
            </CardHeader>
            <CardContent className="log-session-list max-h-[560px] space-y-2 overflow-y-auto p-2">
              {busy ? <div className="px-2 py-4 text-sm text-muted-foreground">正在读取本地 WP 快照...</div> : null}
              {syncing ? <div className="px-2 py-4 text-sm text-muted-foreground">正在抓取信息系统 WP 历史并保存到本地...</div> : null}
              {!busy && filtered.length === 0 ? <EmptyState text="当前筛选条件下没有账号题目。" /> : null}
              {filtered.map((item) => (
                <ChallengeCard key={item.key} item={item} active={selected?.key === item.key} onClick={() => setSelectedKey(item.key)} />
              ))}
            </CardContent>
          </Card>
        </aside>

        <Card className="min-h-[760px] overflow-hidden border-zinc-200 bg-white shadow-sm">
          <CardHeader className="border-b border-border/70 bg-zinc-50/70 p-4">
            <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_auto] xl:items-start">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <CardTitle className="truncate text-base">
                    {selected ? `${selected.account} / ${selected.challenge?.challenge_id} ${selected.challenge?.title}` : "WP 详情"}
                  </CardTitle>
                  {selected ? <StatusBadge status={selected.status} /> : null}
                  {selected ? <SyncBadge status={selected.sync_status} /> : null}
                </div>
                <CardDescription className="mt-1 truncate text-xs">
                  {selected ? `${selected.section_label} / 期望文件名：${selected.expected_filename || "-"}` : "选择左侧账号题目查看远端提交状态"}
                </CardDescription>
              </div>
              <Button variant="outline" size="sm" onClick={reload} disabled={busy || syncing}>
                <RefreshCcw className={cn("h-4 w-4", busy && "animate-spin")} />
                刷新本地快照
              </Button>
            </div>
          </CardHeader>

          <CardContent className="space-y-4 p-4">
            {!selected ? <EmptyState text="当前没有可展示的题目。" /> : null}

            {selected ? (
              <>
                <div className="grid gap-2 md:grid-cols-4">
                  <InfoBox label="提交身份" value={selected.submit_identity} icon={ShieldCheck} compact />
                  <InfoBox label="远端匹配" value={`${selected.remote_attempts || 0} 条记录`} icon={History} compact />
                  <InfoBox label="同步状态" value={selected.sync_message || selected.sync_status || "-"} icon={Cloud} />
                  <InfoBox label="已解时间" value={selected.platform_solved_at || selected.last_submitted_at || "-"} icon={RefreshCcw} />
                </div>

                <div className="grid gap-3 lg:grid-cols-[1fr_0.9fr]">
                  <Card className="border-zinc-200 shadow-none">
                    <CardHeader className="p-4">
                      <CardTitle className="text-sm">监控规则</CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-2 p-4 pt-0">
                      <CheckRow ok={selected.platform_solved} label="账号已解出该题" />
                      <CheckRow ok={selected.sync_status === "synced"} label="已同步该账号远端 WP 历史" />
                      <CheckRow ok={selected.remote_submitted} label="远端存在匹配本题的 WP 记录" />
                      <CheckRow ok={selected.status === "submitted"} label="远端记录符合当前提交要求" />
                      <CheckRow ok={(selected.remote_attempts || 0) <= 3} label="单题提交次数不超过 3 次" soft />
                    </CardContent>
                  </Card>

                  <Card className="border-zinc-200 shadow-none">
                    <CardHeader className="p-4">
                      <CardTitle className="text-sm">官方规则摘要</CardTitle>
                      <CardDescription>{payload.summary?.monitor_endpoint || "https://information.isclab.org.cn/wpupload"}</CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-2 p-4 pt-0 text-xs leading-5 text-zinc-600">
                      <RuleLine text="所有参赛选手均须提交所解出题目的 WriteUp，练武题和擂台题都需要。" />
                      <RuleLine text="必须使用填写报名信息的本系统账号提交；文件名格式为：本系统注册邮箱-题目名称.docx。" />
                      <RuleLine text="每次仅允许提交一道题，每道题最多 3 次提交机会，单个文件不超过 5MB。" />
                    </CardContent>
                  </Card>
                </div>

                <div className="grid gap-3 lg:grid-cols-2">
                  <IssueList title="阻塞/异常" items={selected.issues || []} empty="没有发现阻塞项。" tone="rose" />
                  <IssueList title="警告项" items={selected.warnings || []} empty="没有警告项。" tone="amber" />
                </div>

                <Card className="border-zinc-200 shadow-none">
                  <CardHeader className="p-4">
                    <CardTitle className="text-sm">本题远端匹配记录</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-2 p-4 pt-0">
                    {(selected.remote_records || []).length === 0 ? <EmptyState text="远端历史里没有匹配到本题 WP 文件。" compact /> : null}
                    {(selected.remote_records || []).map((record) => (
                      <RecordLine key={`${record.sequence}-${record.filename}`} record={record} />
                    ))}
                  </CardContent>
                </Card>

                <Card className="border-zinc-200 shadow-none">
                  <CardHeader className="p-4">
                    <CardTitle className="text-sm">该账号全部远端记录</CardTitle>
                  </CardHeader>
                  <CardContent className="log-session-list max-h-[260px] space-y-2 overflow-y-auto p-4 pt-0">
                    {accountRecords.length === 0 ? <EmptyState text="该账号远端历史暂无 WP 记录。" compact /> : null}
                    {accountRecords.map((record) => (
                      <RecordLine key={`${record.account}-${record.sequence}-${record.filename}`} record={record} subtle />
                    ))}
                  </CardContent>
                </Card>
              </>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  );
}

function SummaryCard({label, value, icon: Icon, tone = "zinc"}) {
  const colors = {
    zinc: "bg-zinc-500",
    rose: "bg-rose-500",
    amber: "bg-amber-500",
    emerald: "bg-emerald-500",
    blue: "bg-blue-500",
    slate: "bg-slate-500",
  };
  return (
    <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
      <CardHeader className="relative flex-row items-center justify-between space-y-0 p-3">
        <span className={cn("absolute inset-x-0 top-0 h-0.5", colors[tone])} />
        <div className="min-w-0">
          <CardDescription className="truncate text-xs font-medium text-zinc-500">{label}</CardDescription>
          <CardTitle className="mt-1 text-xl tabular-nums text-zinc-950">{value}</CardTitle>
        </div>
        <div className={cn("rounded-md p-1.5 text-white", colors[tone])}>
          <Icon className="h-3.5 w-3.5" />
        </div>
      </CardHeader>
    </Card>
  );
}

function ChallengeCard({item, active, onClick}) {
  const challenge = item.challenge || {};
  return (
    <button
      className={cn(
        "w-full rounded-md border px-3 py-3 text-left transition-colors",
        active ? "border-zinc-900 bg-zinc-950 text-zinc-100" : "border-border bg-white hover:border-zinc-300 hover:bg-zinc-50",
      )}
      onClick={onClick}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold">{item.account} / {challenge.challenge_id} {challenge.title}</div>
          <div className={cn("mt-1 line-clamp-1 text-xs", active ? "text-zinc-400" : "text-muted-foreground")}>
            {item.section_label} / {item.expected_filename || "缺少期望文件名"}
          </div>
        </div>
        <StatusBadge status={item.status} active={active} />
      </div>
      <div className={cn("mt-2 flex items-center gap-3 text-[11px]", active ? "text-zinc-500" : "text-muted-foreground")}>
        <span>远端 {item.remote_attempts || 0}</span>
        <span>{item.issues?.length || 0} 异常</span>
        <span>{item.sync_status === "synced" ? "已同步" : "未同步"}</span>
      </div>
    </button>
  );
}

function StatusBadge({status, active = false}) {
  const meta = statusMeta(status);
  const Icon = meta.icon;
  return (
    <span className={cn("inline-flex h-7 shrink-0 items-center gap-1.5 rounded border px-2 text-xs font-medium", active ? "border-white/10 bg-white/10 text-zinc-200" : meta.tone)}>
      <Icon className="h-3.5 w-3.5" />
      {meta.label}
    </span>
  );
}

function SyncBadge({status}) {
  const meta = syncMeta(status);
  return (
    <span className={cn("inline-flex h-7 shrink-0 items-center rounded border px-2 text-xs font-medium", meta.tone)}>
      {meta.label}
    </span>
  );
}

function InfoBox({label, value, icon: Icon, compact = false}) {
  return (
    <div className="min-h-[74px] rounded-md border border-zinc-200 bg-white px-3 py-2.5">
      <div className="flex items-center gap-2 text-[11px] font-semibold uppercase text-zinc-500">
        <Icon className="h-3.5 w-3.5 shrink-0" />
        {label}
      </div>
      <div className={cn("mt-1.5 text-sm font-medium leading-5 text-zinc-900", compact ? "truncate" : "line-clamp-2 break-all")}>{value || "-"}</div>
    </div>
  );
}

function CheckRow({ok, label, soft = false}) {
  const Icon = ok ? CheckCircle2 : soft ? AlertTriangle : XCircle;
  return (
    <div className={cn("flex items-center gap-2 rounded-md border px-3 py-2 text-sm", ok ? "border-emerald-200 bg-emerald-50 text-emerald-700" : soft ? "border-amber-200 bg-amber-50 text-amber-700" : "border-rose-200 bg-rose-50 text-rose-700")}>
      <Icon className="h-4 w-4 shrink-0" />
      <span>{label}</span>
    </div>
  );
}

function RuleLine({text}) {
  return (
    <div className="rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2">
      {text}
    </div>
  );
}

function IssueList({title, items, empty, tone}) {
  const className = tone === "rose" ? "border-rose-200 bg-rose-50 text-rose-700" : "border-amber-200 bg-amber-50 text-amber-700";
  return (
    <Card className="border-zinc-200 shadow-none">
      <CardHeader className="p-4">
        <CardTitle className="text-sm">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 p-4 pt-0">
        {items.length === 0 ? <div className="rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2 text-sm text-muted-foreground">{empty}</div> : null}
        {items.map((item) => (
          <div key={`${item.code}-${item.message}`} className={cn("rounded-md border px-3 py-2 text-sm", className)}>
            <div className="font-medium">{item.code}</div>
            <div className="mt-1 text-xs opacity-80">{item.message}</div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function RecordLine({record, subtle = false}) {
  const ok = record.match_status === "matched" && (record.issues || []).length === 0;
  return (
    <div className={cn("rounded-md border px-3 py-2 text-sm", ok ? "border-emerald-200 bg-emerald-50 text-emerald-800" : subtle ? "border-zinc-200 bg-zinc-50 text-zinc-700" : "border-amber-200 bg-amber-50 text-amber-800")}>
      <div className="flex items-center justify-between gap-3">
        <span className="min-w-0 truncate font-medium">{record.filename}</span>
        <span className="shrink-0 text-xs opacity-70">#{record.sequence || "-"}</span>
      </div>
      <div className="mt-1 flex flex-wrap gap-2 text-[11px] opacity-75">
        <span>{record.match_status === "matched" ? "已匹配" : "未匹配"}</span>
        {record.challenge_title ? <span>题名：{record.challenge_title}</span> : null}
        {(record.issues || []).length ? <span>{record.issues.length} 异常</span> : null}
      </div>
    </div>
  );
}

function EmptyState({text, compact = false}) {
  return (
    <div className={cn("rounded-lg border border-dashed border-border bg-muted/20 text-center text-sm text-muted-foreground", compact ? "p-4" : "p-8")}>
      {text}
    </div>
  );
}
