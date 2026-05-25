import {useEffect, useMemo, useRef, useState} from "react";
import {useSearchParams} from "react-router-dom";
import {
  Activity,
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Binary,
  CheckCircle2,
  ChevronDown,
  Clock3,
  Copy,
  Download,
  Eye,
  EyeOff,
  FileSearch2,
  FileText,
  Filter,
  PauseCircle,
  PlayCircle,
  RefreshCcw,
  ScrollText,
  Search,
  TerminalSquare,
  UserRound,
  XCircle,
} from "lucide-react";
import {LogContent, Logs} from "../../wailsjs/go/desktop/API";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {Badge} from "../components/ui/Badge";
import {Button} from "../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../components/ui/Card";
import {Input} from "../components/ui/Input";
import {cn} from "../lib/utils";
import {pageMeta} from "../lib/iscc";

const REFRESH_OPTIONS = [
  {value: "0", label: "关闭实时"},
  {value: "5", label: "5s"},
  {value: "10", label: "10s"},
  {value: "30", label: "30s"},
];

const LIVE_RUNNING_REFRESH_MS = 2200;

function terminalLineType(line) {
  const text = line.trim();
  if (!text) {
    return "empty";
  }
  if (/\[(error|fatal)\]/i.test(text) || /网络请求失败|traceback|exception|ssl/i.test(text)) {
    return "error";
  }
  if (/\[(warn|warning)\]/i.test(text) || /retry|stopped|stop/i.test(text)) {
    return "warn";
  }
  if (/accepted=true|login ok|finished|success|proxy ready/i.test(text)) {
    return "ok";
  }
  if (/\[system\]/i.test(text) || /job started|command=|section=|remote solved/i.test(text)) {
    return "system";
  }
  if (/\[[^\]]+\]/.test(text)) {
    return "account";
  }
  return "plain";
}

function parseLogLines(content) {
  if (!content) {
    return [];
  }
  return content.split(/\r?\n/).map((line, index) => {
    const match = line.match(/^(\[[^\]]+\])(\[[^\]]+\])?\s?(.*)$/);
    const accountGuess = line.match(/\[([^\[\]]+)\]/g)?.map((part) => part.replace(/^\[|\]$/g, "")) || [];
    const account = accountGuess.find((item) => item && !/^\+\d/.test(item) && item !== "system" && item !== "error" && item !== "warn" && item !== "warning") || "";
    return {
      id: `${index}-${line}`,
      no: String(index + 1).padStart(3, "0"),
      raw: line,
      head: match?.[1] || "",
      tag: match?.[2] || "",
      message: match?.[3] ?? line,
      type: terminalLineType(line),
      account,
      text: line.toLowerCase(),
    };
  });
}

function sourceTone(sourceType) {
  switch (sourceType) {
    default:
      return "border-zinc-200 bg-zinc-50 text-zinc-700";
  }
}

function sourceLabel(sourceType) {
  switch (sourceType) {
    default:
      return sourceType || "Unknown";
  }
}

function statusTone(status, active = false) {
  const value = String(status || "").toLowerCase();
  if (value === "running") {
    return "border-emerald-100 bg-emerald-50/80 text-emerald-600";
  }
  if (value.includes("fail") || value.includes("error")) {
    return "border-rose-100 bg-rose-50/80 text-rose-600";
  }
  if (value.includes("finish") || value.includes("success") || value === "done") {
    return "border-sky-100 bg-sky-50/80 text-sky-600";
  }
  if (value.includes("stop")) {
    return "border-amber-100 bg-amber-50/85 text-amber-600";
  }
  return active ? "border-zinc-200 bg-white text-zinc-700" : "border-zinc-200 bg-zinc-50 text-zinc-600";
}

function summaryTone(tone = "slate") {
  return {
    cyan: "border-zinc-200 bg-white text-zinc-700",
    sky: "border-zinc-200 bg-white text-zinc-700",
    red: "border-zinc-200 bg-white text-zinc-700",
    green: "border-zinc-200 bg-white text-zinc-700",
    amber: "border-zinc-200 bg-white text-zinc-700",
    slate: "border-zinc-200 bg-white text-zinc-700",
  }[tone];
}

function summaryAccent(tone = "slate") {
  return {
    cyan: "bg-sky-200",
    sky: "bg-blue-200",
    red: "bg-rose-200",
    green: "bg-emerald-200",
    amber: "bg-amber-200",
    slate: "bg-zinc-200",
  }[tone];
}

function summaryIconTone(tone = "slate") {
  return {
    cyan: "border-sky-100 bg-sky-50/70 text-sky-600",
    sky: "border-blue-100 bg-blue-50/70 text-blue-600",
    red: "border-rose-100 bg-rose-50/70 text-rose-600",
    green: "border-emerald-100 bg-emerald-50/70 text-emerald-600",
    amber: "border-amber-100 bg-amber-50/75 text-amber-600",
    slate: "border-zinc-200 bg-zinc-50 text-zinc-700",
  }[tone];
}

function formatBytes(bytes) {
  const value = Number(bytes || 0);
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function sessionMatches(item, text, status, sourceType) {
  const haystack = [item.title, item.command, item.description, item.account, item.source_type, item.name].filter(Boolean).join(" ").toLowerCase();
  if (text && !haystack.includes(text)) {
    return false;
  }
  if (status && item.status !== status && item.source_type !== status) {
    return false;
  }
  if (sourceType && item.source_type !== sourceType) {
    return false;
  }
  return true;
}

function isJobRunning(status) {
  return String(status || "").trim().toLowerCase() === "running";
}

export function LogsPage() {
  const meta = pageMeta.logs;
  const [searchParams, setSearchParams] = useSearchParams();
  const terminalRef = useRef(null);
  const detailCardRef = useRef(null);
  const [payload, setPayload] = useState({summary: null, jobs: []});
  const [selected, setSelected] = useState(null);
  const [content, setContent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [rawMode, setRawMode] = useState(false);
  const [query, setQuery] = useState("");
  const [onlyErrors, setOnlyErrors] = useState(false);
  const [accountFilter, setAccountFilter] = useState("");
  const [autoFollow, setAutoFollow] = useState(true);
  const [sessionQuery, setSessionQuery] = useState("");
  const [sessionStatusFilter, setSessionStatusFilter] = useState("");
  const [sessionSourceFilter, setSessionSourceFilter] = useState("");
  const [refreshSeconds, setRefreshSeconds] = useState("0");
  const [lastRefreshedAt, setLastRefreshedAt] = useState("");

  const selectedKey = useMemo(() => (selected ? `${selected.kind}:${selected.id}` : ""), [selected]);
  const parsedLines = useMemo(() => parseLogLines(content?.content || ""), [content]);
  const selectedJob = useMemo(
    () => (selected?.kind === "job" ? payload.jobs.find((item) => item.id === selected.id) || null : null),
    [payload.jobs, selected],
  );
  const selectedIsRunning = useMemo(() => isJobRunning(selectedJob?.status), [selectedJob]);

  const availableAccounts = useMemo(() => {
    const accounts = new Set();
    parsedLines.forEach((line) => {
      if (line.account) {
        accounts.add(line.account);
      }
    });
    return Array.from(accounts).sort((a, b) => a.localeCompare(b));
  }, [parsedLines]);

  const sessionStatuses = useMemo(() => {
    const values = new Set();
    payload.jobs.forEach((item) => item.status && values.add(item.status));
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [payload]);

  const sessionSourceTypes = useMemo(() => {
    const values = new Set();
    payload.jobs.forEach((item) => item.source_type && values.add(item.source_type));
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [payload]);

  const filteredLines = useMemo(() => {
    const text = query.trim().toLowerCase();
    return parsedLines.filter((line) => {
      if (onlyErrors && line.type !== "error" && line.type !== "warn") {
        return false;
      }
      if (accountFilter && line.account !== accountFilter) {
        return false;
      }
      if (text && !line.text.includes(text)) {
        return false;
      }
      return true;
    });
  }, [parsedLines, query, onlyErrors, accountFilter]);

  const lineStats = useMemo(() => {
    return parsedLines.reduce(
      (stats, line) => {
        stats.total += 1;
        stats[line.type] = (stats[line.type] || 0) + 1;
        return stats;
      },
      {total: 0, error: 0, warn: 0, ok: 0, system: 0, account: 0, plain: 0, empty: 0},
    );
  }, [parsedLines]);

  const filteredJobs = useMemo(() => {
    const text = sessionQuery.trim().toLowerCase();
    return payload.jobs.filter((item) => sessionMatches(item, text, sessionStatusFilter, sessionSourceFilter));
  }, [payload.jobs, sessionQuery, sessionStatusFilter, sessionSourceFilter]);

  useEffect(() => {
    reload();
  }, []);

  useEffect(() => {
    if (!selected) {
      const requestedJobID = searchParams.get("job");
      if (requestedJobID) {
        const matched = payload.jobs?.find((item) => item.id === requestedJobID);
        if (matched) {
          setSelected({kind: "job", id: matched.id});
          setSearchParams((current) => {
            current.delete("job");
            return current;
          }, {replace: true});
          return;
        }
      }
      const firstJob = filteredJobs[0] || payload.jobs?.[0];
      if (firstJob) {
        setSelected({kind: "job", id: firstJob.id});
      }
    }
  }, [payload, selected, filteredJobs, searchParams, setSearchParams]);

  useEffect(() => {
    if (selected?.kind && selected.kind !== "job") {
      setSelected(null);
      setContent(null);
    }
  }, [selected]);

  useEffect(() => {
    if (!selected) {
      setContent(null);
      return;
    }
    readContent(selected, false);
  }, [selected]);

  useEffect(() => {
    if (!autoFollow || !terminalRef.current) {
      return;
    }
    terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
  }, [filteredLines, rawMode, autoFollow]);

  useEffect(() => {
    if (selectedIsRunning) {
      return undefined;
    }
    const seconds = Number(refreshSeconds);
    if (!seconds || Number.isNaN(seconds)) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      reload(true);
    }, seconds * 1000);
    return () => window.clearInterval(timer);
  }, [refreshSeconds, selected, selectedIsRunning]);

  useEffect(() => {
    if (!selectedIsRunning) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      reload(true);
    }, LIVE_RUNNING_REFRESH_MS);
    return () => window.clearInterval(timer);
  }, [selectedIsRunning, selectedKey]);

  useEffect(() => {
    if (selectedIsRunning) {
      setAutoFollow(true);
    }
  }, [selectedIsRunning, selectedKey]);

  async function reload(silent = false) {
    if (!silent) {
      setLoading(true);
    }
    setError("");
    try {
      const nextPayload = await Logs();
      setPayload({
        summary: nextPayload?.summary || null,
        jobs: nextPayload?.jobs || [],
      });
      setLastRefreshedAt(new Date().toLocaleTimeString("zh-CN", {hour12: false}));
      if (selected) {
        await readContent(selected, true);
      }
    } catch (err) {
      setError(String(err));
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }

  async function readContent(target, silent = false) {
    if (!silent) {
      setBusy(true);
    }
    setError("");
    try {
      const result = await LogContent({id: target.id, kind: target.kind});
      setContent(result);
      if (!silent) {
        setQuery("");
        setOnlyErrors(false);
        setAccountFilter("");
      }
    } catch (err) {
      setError(String(err));
    } finally {
      if (!silent) {
        setBusy(false);
      }
    }
  }

  async function copyCurrentLog() {
    if (!content?.content) {
      setMessage("当前没有可复制的日志内容");
      return;
    }
    try {
      await navigator.clipboard.writeText(content.content);
      setMessage("当前日志已复制到剪贴板");
    } catch (err) {
      setError(`复制失败: ${String(err)}`);
    }
  }

  function downloadCurrentLog() {
    if (!content?.content) {
      setMessage("当前没有可导出的日志内容");
      return;
    }
    const blob = new Blob([content.content], {type: "text/plain;charset=utf-8"});
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    const filenameBase = (content.title || "got0iscc-log").replace(/[^\w\u4e00-\u9fa5.-]+/g, "-");
    anchor.href = url;
    anchor.download = `${filenameBase || "got0iscc-log"}.log`;
    anchor.click();
    URL.revokeObjectURL(url);
    setMessage("当前日志已导出");
  }

  function handleSelectSession(target) {
    setSelected(target);
    setMessage("");
    if (typeof window !== "undefined" && window.matchMedia("(max-width: 1279px)").matches) {
      window.setTimeout(() => {
        detailCardRef.current?.scrollIntoView({behavior: "smooth", block: "start"});
      }, 80);
    }
  }

  function scrollTerminal(direction) {
    if (!terminalRef.current) {
      return;
    }
    terminalRef.current.scrollTo({
      top: direction === "top" ? 0 : terminalRef.current.scrollHeight,
      behavior: "smooth",
    });
  }

  return (
    <PageContainer className="max-w-[1500px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        description="集中查看任务历史和实时终端输出。"
        action={
          <>
            <div className="flex h-9 items-center gap-2 rounded-md border border-border bg-background px-3 text-xs text-muted-foreground shadow-sm">
              <Clock3 className="h-3.5 w-3.5" />
              <span>最近刷新 {lastRefreshedAt || "--:--:--"}</span>
            </div>
            <Button variant="outline" size="sm" onClick={() => setRawMode((value) => !value)} disabled={busy}>
              {rawMode ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
              {rawMode ? "高亮模式" : "原始模式"}
            </Button>
            <Button variant="outline" size="sm" onClick={() => reload()} disabled={busy}>
              <RefreshCcw className="h-4 w-4" />
              刷新
            </Button>
          </>
        }
      />

      <div className="grid auto-rows-[84px] gap-2 md:grid-cols-3 xl:grid-cols-5">
        <SummaryCard label="任务总数" value={payload.summary?.total_jobs ?? 0} icon={TerminalSquare} tone="cyan" />
        <SummaryCard label="运行中" value={payload.summary?.running_jobs ?? 0} icon={Activity} tone="sky" />
        <SummaryCard label="失败任务" value={payload.summary?.failed_jobs ?? 0} icon={AlertTriangle} tone="red" />
        <SummaryCard label="已完成" value={payload.summary?.finished_jobs ?? 0} icon={CheckCircle2} tone="green" />
        <SummaryCard label="已停止" value={payload.summary?.stopped_jobs ?? 0} icon={PauseCircle} tone="amber" />
      </div>

      {(message || error) ? (
        <div className={`rounded-lg border px-4 py-3 text-sm shadow-sm ${error ? "border-destructive/40 bg-destructive/10 text-destructive" : "border-primary/30 bg-primary/10 text-primary"}`}>
          {error || message}
        </div>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-[minmax(340px,0.86fr)_minmax(0,1.55fr)]">
        <aside className="space-y-3">
          <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
            <CardContent className="grid gap-2 p-3">
              <div className="flex min-h-10 items-center justify-between gap-3">
                <div>
                  <CardTitle className="text-sm">会话筛选</CardTitle>
                  <CardDescription className="mt-1 text-xs">任务 {filteredJobs.length}</CardDescription>
                </div>
                <Badge variant="outline" className="h-8 rounded-md bg-white px-3 text-xs font-medium">
                  {refreshSeconds === "0" ? "Manual" : `${refreshSeconds}s`}
                </Badge>
              </div>

              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={sessionQuery}
                  onChange={(event) => setSessionQuery(event.target.value)}
                  placeholder="搜索标题、命令、账号、来源"
                  className="rounded-md bg-white pl-9"
                />
              </div>
              <div className="grid gap-2 sm:grid-cols-3 xl:grid-cols-1 2xl:grid-cols-3">
                <SelectField value={sessionStatusFilter} onChange={setSessionStatusFilter} icon={Filter}>
                  <option value="">全部状态</option>
                  {sessionStatuses.map((status) => <option key={status} value={status}>{status}</option>)}
                </SelectField>
                <SelectField value={sessionSourceFilter} onChange={setSessionSourceFilter} icon={TerminalSquare}>
                  <option value="">全部来源</option>
                  {sessionSourceTypes.map((type) => <option key={type} value={type}>{type}</option>)}
                </SelectField>
                <SelectField value={refreshSeconds} onChange={setRefreshSeconds} icon={refreshSeconds === "0" ? PauseCircle : PlayCircle}>
                  {REFRESH_OPTIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
                </SelectField>
              </div>
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
            <CardHeader className="border-b border-border/70 bg-white px-3 py-3">
              <div className="flex min-h-10 items-center justify-between gap-3">
                <div>
                  <CardTitle className="text-sm">任务历史</CardTitle>
                  <CardDescription className="mt-1 text-xs">{filteredJobs.length} 条匹配记录</CardDescription>
                </div>
                <TerminalSquare className="h-4 w-4 text-muted-foreground" />
              </div>
            </CardHeader>
            <CardContent className="log-session-list max-h-[430px] space-y-2 overflow-y-auto p-2">
              {loading ? <div className="text-sm text-muted-foreground">加载日志中...</div> : null}
              {!loading && filteredJobs.length === 0 ? <EmptyState text="当前筛选条件下没有任务会话。" /> : null}
              {filteredJobs.map((job) => (
                <LogSessionCard
                  key={job.id}
                  active={selectedKey === `job:${job.id}`}
                  title={job.title || job.id}
                  subtitle={job.command || "无命令行记录"}
                  status={job.status}
                  metaLeft={`账号 ${job.account || "-"}`}
                  metaRight={job.started_at || "-"}
                  sourceType={job.source_type}
                  path={job.log_path}
                  preview={job.tail || job.command || "当前还没有日志预览"}
                  live={isJobRunning(job.status)}
                  onClick={() => handleSelectSession({kind: "job", id: job.id})}
                />
              ))}
            </CardContent>
          </Card>

        </aside>

        <Card ref={detailCardRef} className="min-h-[820px] overflow-hidden border-zinc-200 bg-white shadow-sm">
          <CardHeader className="border-b border-zinc-200 bg-zinc-50/80 p-4">
            <div className="flex flex-col gap-3">
              <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_auto] xl:items-start">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle className="max-w-full truncate text-base">{content?.title || "日志终端"}</CardTitle>
                    {selectedIsRunning ? (
                      <Badge className="h-7 gap-1.5 rounded-md border border-sky-100 bg-sky-50/80 text-sky-600">
                        <span className="h-2 w-2 animate-pulse rounded-full bg-sky-400" />
                        LIVE
                      </Badge>
                    ) : null}
                  </div>
                  <CardDescription className="mt-1 max-w-4xl truncate text-xs">{content?.path || "未选择日志"}</CardDescription>
                </div>
                <div className="flex min-h-7 flex-wrap items-start gap-2">
                  {selectedJob ? (
                    <Badge variant="outline" className={cn("h-7 rounded-md font-medium", statusTone(selectedJob.status))}>{selectedJob.status || "unknown"}</Badge>
                  ) : null}
                  <Badge variant="secondary" className="h-7 rounded-md">{rawMode ? "RAW" : "高亮"}</Badge>
                  <Badge variant="outline" className="h-7 rounded-md bg-white">{content?.truncated ? "Tail 128KB" : "Full"}</Badge>
                  <Badge variant="outline" className="h-7 rounded-md bg-white">{onlyErrors ? "错误焦点" : "全部级别"}</Badge>
                </div>
              </div>

              <div className="grid gap-2 lg:grid-cols-[minmax(220px,1fr)_170px_minmax(356px,auto)]">
                <div className="relative min-w-0">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder="搜索关键字、错误文案、命令片段"
                    className="rounded-md bg-white pl-9"
                  />
                </div>
                <SelectField value={accountFilter} onChange={setAccountFilter} icon={UserRound}>
                  <option value="">全部账号</option>
                  {availableAccounts.map((account) => <option key={account} value={account}>{account}</option>)}
                </SelectField>
                <div className="grid grid-cols-[repeat(6,minmax(0,auto))] gap-2">
                  <Button className="h-10" variant={onlyErrors ? "default" : "outline"} size="sm" onClick={() => setOnlyErrors((value) => !value)} disabled={busy}>
                    <Filter className="h-4 w-4" />
                    仅错误
                  </Button>
                  <Button className="h-10" variant={autoFollow ? "default" : "outline"} size="sm" onClick={() => setAutoFollow((value) => !value)} disabled={busy}>
                    <ScrollText className="h-4 w-4" />
                    滚底
                  </Button>
                  <Button className="h-10 w-10" variant="outline" size="icon" aria-label="滚动到顶部" title="滚动到顶部" onClick={() => scrollTerminal("top")} disabled={busy || !content?.content}>
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button className="h-10 w-10" variant="outline" size="icon" aria-label="滚动到底部" title="滚动到底部" onClick={() => scrollTerminal("bottom")} disabled={busy || !content?.content}>
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                  <Button className="h-10 w-10" variant="outline" size="icon" aria-label="复制日志" title="复制日志" onClick={copyCurrentLog} disabled={busy || !content?.content}>
                    <Copy className="h-4 w-4" />
                  </Button>
                  <Button className="h-10 w-10" variant="outline" size="icon" aria-label="导出日志" title="导出日志" onClick={downloadCurrentLog} disabled={busy || !content?.content}>
                    <Download className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>
          </CardHeader>

          <CardContent className="space-y-3 p-4">
            <div className="grid gap-2 md:grid-cols-4">
              <TerminalInfo label="来源" value={content?.source || "-"} icon={Binary} />
              <TerminalInfo label="大小" value={formatBytes(content?.size_bytes ?? 0)} icon={ScrollText} compact />
              <TerminalInfo label="更新时间" value={content?.modified_at || "-"} icon={Clock3} />
              <TerminalInfo
                label="行数"
                value={selectedIsRunning ? `Live ${LIVE_RUNNING_REFRESH_MS / 1000}s` : `${filteredLines.length} / ${parsedLines.length || 0}`}
                icon={selectedIsRunning ? Activity : FileSearch2}
                compact
              />
            </div>

            {content?.description ? (
              <div className="rounded-md border border-zinc-200 bg-zinc-50 px-4 py-3 text-sm text-zinc-600">
                {content.description}
              </div>
            ) : null}

            {busy ? (
              <div className="rounded-lg border border-dashed border-zinc-300 bg-zinc-50 p-8 text-center text-sm text-muted-foreground">
                正在读取日志内容...
              </div>
            ) : null}

            {!busy && !content ? <EmptyState text="选择一条任务日志，右侧会显示完整内容。" /> : null}

            {!busy && content ? (
              <div className="overflow-hidden rounded-lg border border-zinc-200 bg-[linear-gradient(180deg,#ffffff,#f5f6f7)] shadow-[0_18px_36px_rgba(24,24,27,0.06)]">
                <div className="grid grid-cols-[auto_1fr_auto] items-center gap-4 border-b border-zinc-200 bg-zinc-50/90 px-4 py-3">
                  <div className="flex items-center gap-1.5">
                    <span className="h-3 w-3 rounded-full bg-rose-400" />
                    <span className="h-3 w-3 rounded-full bg-amber-400" />
                    <span className="h-3 w-3 rounded-full bg-emerald-400" />
                  </div>
                  <div className="min-w-0 truncate text-center font-mono text-[11px] text-zinc-500">got0iscc log terminal</div>
                  <div className="hidden text-right font-mono text-xs text-zinc-500 sm:block">
                    ERR {lineStats.error} / WARN {lineStats.warn} / OK {lineStats.ok}
                  </div>
                </div>

                {rawMode ? (
                  <pre ref={terminalRef} className="log-terminal min-h-[520px] max-h-[72vh] overflow-auto whitespace-pre-wrap bg-transparent px-4 py-4 font-mono text-[12px] leading-6 text-zinc-800">
                    {content.content || "日志文件为空"}
                  </pre>
                ) : (
                  <div ref={terminalRef} className="log-terminal min-h-[520px] max-h-[72vh] overflow-auto bg-transparent px-0 py-2">
                    {filteredLines.length === 0 ? (
                      <div className="px-5 py-5 text-sm text-zinc-500">当前筛选条件下没有匹配到日志行</div>
                    ) : null}
                    {filteredLines.map((line) => <TerminalLine key={line.id} line={line} />)}
                  </div>
                )}
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  );
}

function SummaryCard({label, value, icon: Icon, tone = "slate"}) {
  return (
    <Card className={cn("overflow-hidden rounded-lg shadow-sm", summaryTone(tone))}>
      <CardHeader className="relative flex-row items-center justify-between space-y-0 p-3">
        <span className={cn("absolute inset-x-0 top-0 h-0.5", summaryAccent(tone))} />
        <div className="min-w-0">
          <CardDescription className="truncate text-xs font-medium text-zinc-500">{label}</CardDescription>
          <CardTitle className="mt-1 text-xl tabular-nums text-zinc-950">{value}</CardTitle>
        </div>
        <div className={cn("rounded-md border p-1.5", summaryIconTone(tone))}>
          <Icon className="h-3.5 w-3.5" />
        </div>
      </CardHeader>
    </Card>
  );
}

function SelectField({value, onChange, icon: Icon, children, className}) {
  return (
    <div className={cn("relative", className)}>
      <Icon className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
      <select
        className="h-10 w-full appearance-none rounded-md border border-input bg-white pl-9 pr-9 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
    </div>
  );
}

function LogSessionCard({
  active,
  title,
  subtitle,
  status,
  metaLeft,
  metaRight,
  sourceType,
  onClick,
  path = "",
  preview = "",
  live = false,
}) {
  return (
    <div
      className={cn(
        "overflow-hidden rounded-md border transition-all",
        active
          ? "border-zinc-200 bg-zinc-50/95 text-zinc-900 shadow-[0_10px_24px_rgba(24,24,27,0.05)] ring-1 ring-zinc-200"
          : "border-border bg-white hover:border-zinc-300 hover:bg-zinc-50",
      )}
    >
      <button className="w-full px-3 py-3 text-left" onClick={onClick}>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold">{title}</div>
            <div className={cn("mt-1 line-clamp-2 text-xs", active ? "text-zinc-600" : "text-muted-foreground")}>
              {subtitle}
            </div>
          </div>
          <div className={cn("shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase", statusTone(status, active))}>
            {status || "unknown"}
          </div>
        </div>
        <div className={cn("mt-2 flex items-center justify-between gap-3 text-[11px]", active ? "text-zinc-500" : "text-muted-foreground")}>
          <span className="truncate">{metaLeft}</span>
          <span className="truncate">{metaRight}</span>
        </div>
      </button>

      <div className={cn("grid transition-[grid-template-rows] duration-300 ease-out", active ? "grid-rows-[1fr]" : "grid-rows-[0fr]")}>
        <div className="overflow-hidden">
          <div className={cn("border-t px-3 py-2.5", active ? "border-zinc-200 bg-white/80" : "border-border bg-muted/15")}>
            <div className="flex flex-wrap items-center gap-1.5 text-[10px] font-semibold uppercase">
              <span className={cn("rounded border px-1.5 py-0.5", statusTone(status, active))}>
                status {status || "unknown"}
              </span>
              <span className={cn("rounded border px-1.5 py-0.5", active ? sourceTone(sourceType) : "border-border bg-background text-muted-foreground")}>
                {sourceLabel(sourceType)}
              </span>
              {live ? (
                <span className="inline-flex items-center gap-1.5 rounded border border-sky-100 bg-sky-50/75 px-1.5 py-0.5 text-sky-600">
                  <span className="h-2 w-2 animate-pulse rounded-full bg-sky-400" />
                  live poll
                </span>
              ) : null}
            </div>

            {path ? (
              <div className={cn("mt-2 line-clamp-1 break-all text-[11px] leading-5", active ? "text-zinc-500" : "text-muted-foreground")}>
                {path}
              </div>
            ) : null}

            <pre className={cn("mt-2 line-clamp-2 whitespace-pre-wrap rounded border px-2.5 py-2 text-[11px] leading-5", active ? "border-zinc-200 bg-zinc-50 text-zinc-600" : "border-border bg-background text-muted-foreground")}>
              {preview || "当前没有可展开的日志预览"}
            </pre>
          </div>
        </div>
      </div>
    </div>
  );
}

function TerminalInfo({label, value, icon: Icon, compact = false}) {
  return (
    <div className="min-h-[74px] rounded-md border border-zinc-200 bg-white px-3 py-2.5">
      <div className="flex items-center gap-2 text-[11px] font-semibold uppercase text-zinc-500">
        <Icon className="h-3.5 w-3.5 shrink-0" />
        {label}
      </div>
      <div className={cn("mt-1.5 text-sm font-medium leading-5 text-zinc-900", compact ? "truncate" : "line-clamp-2 break-all")}>{value}</div>
    </div>
  );
}

function TerminalLine({line}) {
  const lineTone = {
    empty: "text-zinc-400",
    plain: "text-zinc-700",
    system: "text-sky-600",
    account: "text-blue-600",
    ok: "text-emerald-600",
    warn: "text-amber-600",
    error: "text-rose-600",
  }[line.type];

  const badge = {
    system: {icon: TerminalSquare, text: "SYS", tone: "border border-sky-100 bg-sky-50/75 text-sky-600"},
    account: {icon: UserRound, text: "ACC", tone: "border border-blue-100 bg-blue-50/75 text-blue-600"},
    ok: {icon: CheckCircle2, text: "OK", tone: "border border-emerald-100 bg-emerald-50/75 text-emerald-600"},
    warn: {icon: AlertTriangle, text: "WARN", tone: "border border-amber-100 bg-amber-50/80 text-amber-600"},
    error: {icon: XCircle, text: "ERR", tone: "border border-rose-100 bg-rose-50/80 text-rose-600"},
    plain: {icon: FileText, text: "TXT", tone: "border border-zinc-200 bg-zinc-100/80 text-zinc-600"},
    empty: {icon: FileText, text: "EMP", tone: "border border-zinc-200 bg-zinc-100/80 text-zinc-400"},
  }[line.type];

  const BadgeIcon = badge.icon;

  return (
    <div className="group grid grid-cols-[44px_56px_minmax(0,1fr)] items-start gap-3 px-3 py-1.5 font-mono text-[12px] leading-6 hover:bg-white/60 sm:grid-cols-[56px_62px_minmax(0,1fr)] sm:px-4">
      <div className="select-none text-right text-[10px] text-zinc-400">{line.no}</div>
      <div className={cn("mt-[2px] inline-flex items-center justify-center gap-1 rounded px-1.5 py-0.5 text-[10px] uppercase", badge.tone)}>
        <BadgeIcon className="h-3 w-3" />
        {badge.text}
      </div>
      <div className={cn("min-w-0 whitespace-pre-wrap break-words", lineTone)}>
        {line.head ? <span className="mr-2 text-zinc-400">{line.head}</span> : null}
        {line.tag ? <span className="mr-2 text-zinc-500">{line.tag}</span> : null}
        <span>{line.message || " "}</span>
      </div>
    </div>
  );
}

function EmptyState({text}) {
  return (
    <div className="rounded-lg border border-dashed border-border bg-muted/20 p-8 text-center text-sm text-muted-foreground">
      {text}
    </div>
  );
}
