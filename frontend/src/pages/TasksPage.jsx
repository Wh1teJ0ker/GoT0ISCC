import {useEffect, useMemo, useState} from "react";
import {useNavigate} from "react-router-dom";
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Play,
  RefreshCcw,
  Square,
  TerminalSquare,
  Zap,
} from "lucide-react";
import {StartTask, StopAllTasks, StopTask, Tasks} from "../../wailsjs/go/desktop/API";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {Badge} from "../components/ui/Badge";
import {Button} from "../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../components/ui/Card";
import {Input} from "../components/ui/Input";
import {pageMeta} from "../lib/iscc";
import {cn} from "../lib/utils";

const defaultForm = {
  command: "solve",
  account: "",
  section: "challenges",
  ids: "",
  flag: "",
  workers: 2,
  force: false,
  force_download: false,
  force_solve: false,
  no_submit: false,
};

const sectionLabels = {
  challenges: "练武题",
  arena: "擂台题",
  theory: "理论题",
  combat: "实战题",
};

function isRunning(status) {
  return ["starting", "running", "stopping"].includes(String(status || "").toLowerCase());
}

function statusTone(status) {
  const value = String(status || "").toLowerCase();
  if (value === "running" || value === "starting") {
    return "border-emerald-200 bg-emerald-50 text-emerald-700";
  }
  if (value === "finished") {
    return "border-cyan-200 bg-cyan-50 text-cyan-700";
  }
  if (value === "failed") {
    return "border-rose-200 bg-rose-50 text-rose-700";
  }
  if (value === "stopped") {
    return "border-amber-200 bg-amber-50 text-amber-700";
  }
  return "border-zinc-200 bg-zinc-50 text-zinc-700";
}

function normalizeIDs(value) {
  return String(value || "")
    .split(/[,\s，、]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function serializeIDs(ids) {
  return Array.from(new Set(ids.map((item) => String(item || "").trim()).filter(Boolean))).join(",");
}

function challengeLabel(item) {
  const title = item?.title ? ` · ${item.title}` : "";
  const category = item?.category ? ` / ${item.category}` : "";
  return `${item?.id || "-"}${title}${category}`;
}

function SummaryCard({label, value, icon: Icon, tone = "slate"}) {
  const toneMap = {
    emerald: "border-emerald-100 bg-emerald-50 text-emerald-700",
    cyan: "border-cyan-100 bg-cyan-50 text-cyan-700",
    amber: "border-amber-100 bg-amber-50 text-amber-700",
    red: "border-rose-100 bg-rose-50 text-rose-700",
    slate: "border-zinc-200 bg-white text-zinc-700",
  };
  return (
    <Card className={cn("overflow-hidden shadow-sm", toneMap[tone])}>
      <CardHeader className="flex-row items-start justify-between space-y-0 p-4">
        <div>
          <CardDescription className="text-xs">{label}</CardDescription>
          <CardTitle className="mt-2 text-2xl">{value}</CardTitle>
        </div>
        <div className="rounded-md bg-black/5 p-2">
          <Icon className="h-4 w-4" />
        </div>
      </CardHeader>
    </Card>
  );
}

export function TasksPage() {
  const meta = pageMeta.tasks;
  const navigate = useNavigate();
  const [payload, setPayload] = useState(null);
  const [form, setForm] = useState(defaultForm);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [actionBusy, setActionBusy] = useState("");
  const [message, setMessage] = useState("");
  const [taskError, setTaskError] = useState("");
  const [query, setQuery] = useState("");

  useEffect(() => {
    reload();
  }, []);

  const jobs = payload?.jobs || [];
  const commands = payload?.commands || [];
  const accounts = payload?.accounts || [];
  const sections = payload?.available_tracks || ["challenges"];
  const challengeOptions = payload?.challenge_options || {};
  const currentChallengeOptions = challengeOptions[form.section] || [];
  const selectedIDs = useMemo(() => normalizeIDs(form.ids), [form.ids]);
  const selectedIDSet = useMemo(() => new Set(selectedIDs), [selectedIDs]);
  const selectedChallengeMap = useMemo(() => {
    const next = new Map();
    currentChallengeOptions.forEach((item) => next.set(String(item.id), item));
    return next;
  }, [currentChallengeOptions]);
  const runningJobs = useMemo(() => jobs.filter((item) => isRunning(item.status)), [jobs]);
  const filteredJobs = useMemo(() => {
    const keyword = String(query || "").trim().toLowerCase();
    if (!keyword) {
      return jobs;
    }
    return jobs.filter((item) => {
      const haystack = [item.title, item.command, item.account, item.status].filter(Boolean).join(" ").toLowerCase();
      return haystack.includes(keyword);
    });
  }, [jobs, query]);

  useEffect(() => {
    if (!commands.length) {
      return;
    }
    if (!commands.find((item) => item.id === form.command)) {
      setForm((current) => ({...current, command: commands[0].id}));
    }
  }, [commands, form.command]);

  async function reload(silent = false) {
    if (silent) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setTaskError("");
    try {
      const next = await Tasks();
      setPayload(next || null);
    } catch (err) {
      setTaskError(String(err));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  function updateField(key, value) {
    setForm((current) => ({...current, [key]: value}));
  }

  function appendChallengeID(value) {
    const id = String(value || "").trim();
    if (!id) {
      return;
    }
    setForm((current) => ({...current, ids: serializeIDs([...normalizeIDs(current.ids), id])}));
  }

  function removeChallengeID(value) {
    const id = String(value || "").trim();
    setForm((current) => ({...current, ids: serializeIDs(normalizeIDs(current.ids).filter((item) => item !== id))}));
  }

  async function submitTask() {
    setSubmitting(true);
    setTaskError("");
    setMessage("");
    try {
      const created = await StartTask({
        ...form,
        workers: Math.max(1, Math.min(32, Number(form.workers || 6))),
        force: Boolean(form.force),
        force_download: Boolean(form.force_download),
        force_solve: Boolean(form.force_solve),
        no_submit: Boolean(form.no_submit),
      });
      setMessage(`任务已启动：${created.title || created.id}`);
      await reload(true);
    } catch (err) {
      setTaskError(String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function stopOne(id) {
    setActionBusy(id);
    setTaskError("");
    setMessage("");
    try {
      await StopTask(id);
      setMessage("任务已停止");
      await reload(true);
    } catch (err) {
      setTaskError(String(err));
    } finally {
      setActionBusy("");
    }
  }

  async function stopAll() {
    setActionBusy("all");
    setTaskError("");
    setMessage("");
    try {
      await StopAllTasks();
      setMessage("运行中的任务已全部停止");
      await reload(true);
    } catch (err) {
      setTaskError(String(err));
    } finally {
      setActionBusy("");
    }
  }

  function openLog(job) {
    navigate(`/logs?job=${encodeURIComponent(job.id)}`);
  }

  return (
    <PageContainer className="max-w-[1500px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <>
            <div className="flex h-9 items-center gap-2 rounded-md border border-border bg-background px-3 text-xs text-muted-foreground shadow-sm">
              <Clock3 className="h-3.5 w-3.5" />
              <span>{payload?.config_path || "-"}</span>
            </div>
            <Button variant="outline" size="sm" onClick={() => reload(true)} disabled={refreshing || submitting}>
              <RefreshCcw className="h-4 w-4" />
              {refreshing ? "刷新中..." : "刷新"}
            </Button>
            <Button variant="outline" size="sm" onClick={stopAll} disabled={!runningJobs.length || actionBusy === "all"}>
              <Square className="h-4 w-4" />
              {actionBusy === "all" ? "停止中..." : "停止全部"}
            </Button>
          </>
        }
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
        <SummaryCard label="任务总数" value={payload?.summary?.total_jobs ?? 0} icon={TerminalSquare} />
        <SummaryCard label="运行中" value={payload?.summary?.running_jobs ?? 0} icon={Zap} tone="emerald" />
        <SummaryCard label="已完成" value={payload?.summary?.finished_jobs ?? 0} icon={CheckCircle2} tone="cyan" />
        <SummaryCard label="失败任务" value={payload?.summary?.failed_jobs ?? 0} icon={AlertTriangle} tone="red" />
        <SummaryCard label="已停止" value={payload?.summary?.stopped_jobs ?? 0} icon={Square} tone="amber" />
      </div>

      {(message || taskError) ? (
        <div className={cn("rounded-md border px-4 py-3 text-sm", taskError ? "border-destructive/40 bg-destructive/10 text-destructive" : "border-primary/30 bg-primary/10 text-primary")}>
          {taskError || message}
        </div>
      ) : null}

      <div className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>启动任务</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 md:grid-cols-2">
              <Field label="命令">
                <select className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm" value={form.command} onChange={(event) => updateField("command", event.target.value)}>
                  {commands.map((item) => (
                    <option key={item.id} value={item.id}>{item.label}</option>
                  ))}
                </select>
              </Field>
              <Field label="账号">
                <select className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm" value={form.account} onChange={(event) => updateField("account", event.target.value)}>
                  <option value="">全部启用账号</option>
                  {accounts.map((item) => (
                    <option key={item.name} value={item.name}>{item.name}</option>
                  ))}
                </select>
              </Field>
              <Field label="赛道">
                <select className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm" value={form.section} onChange={(event) => updateField("section", event.target.value)}>
                  {sections.map((item) => (
                    <option key={item} value={item}>{sectionLabels[item] || item}</option>
                  ))}
                </select>
              </Field>
              <Field label="并发数">
                <Input type="number" min="1" max="32" value={form.workers} onChange={(event) => updateField("workers", event.target.value)} />
                <div className="mt-1 text-[11px] leading-4 text-muted-foreground">同步资产时用于并发抓取题目详情和下载附件，建议 4-12，最高 32。</div>
              </Field>
              <Field label="题目 ID">
                <div className="space-y-2">
                  <select
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
                    value=""
                    onChange={(event) => appendChallengeID(event.target.value)}
                    disabled={!currentChallengeOptions.length}
                  >
                    <option value="">
                      {currentChallengeOptions.length ? `下拉选择${sectionLabels[form.section] || form.section}题目 ID` : "当前赛道暂无缓存题目"}
                    </option>
                    {currentChallengeOptions.map((item) => (
                      <option key={`${item.section}:${item.id}`} value={item.id} disabled={selectedIDSet.has(String(item.id))}>
                        {challengeLabel(item)}
                      </option>
                    ))}
                  </select>
                  <Input value={form.ids} onChange={(event) => updateField("ids", event.target.value)} placeholder="如 15,17,21，可手动批量填写" />
                  <div className="flex flex-wrap items-center gap-2 text-[11px] leading-4 text-muted-foreground">
                    <span>已缓存 {currentChallengeOptions.length} 道题，已选 {selectedIDs.length} 个 ID。</span>
                    {selectedIDs.length ? (
                      <button type="button" className="font-medium text-primary hover:underline" onClick={() => updateField("ids", "")}>
                        清空
                      </button>
                    ) : null}
                  </div>
                  {selectedIDs.length ? (
                    <div className="flex max-h-20 flex-wrap gap-1.5 overflow-y-auto rounded-md border border-border bg-muted/20 p-2">
                      {selectedIDs.map((id) => {
                        const challenge = selectedChallengeMap.get(id);
                        return (
                          <Badge key={id} variant="outline" className="gap-1 border-border bg-background">
                            <span>{challenge ? challengeLabel(challenge) : id}</span>
                            <button type="button" className="text-muted-foreground hover:text-destructive" onClick={() => removeChallengeID(id)}>
                              移除
                            </button>
                          </Badge>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              </Field>
              <Field label="Flag">
                <Input value={form.flag} onChange={(event) => updateField("flag", event.target.value)} placeholder="submit-flag 时填写" />
              </Field>
            </div>

            <div className="grid gap-2 md:grid-cols-2">
              <CheckOption label="强制提交/重试" checked={form.force} onChange={(value) => updateField("force", value)} />
              <CheckOption label="强制重下附件" checked={form.force_download} onChange={(value) => updateField("force_download", value)} />
              <CheckOption label="强制重跑解题" checked={form.force_solve} onChange={(value) => updateField("force_solve", value)} />
              <CheckOption label="拿到 flag 不提交" checked={form.no_submit} onChange={(value) => updateField("no_submit", value)} />
            </div>

            <div className="flex flex-wrap justify-end gap-3">
              <Button variant="outline" size="sm" onClick={() => setForm(defaultForm)} disabled={submitting}>重置表单</Button>
              <Button size="sm" onClick={submitTask} disabled={submitting}>
                <Play className="h-4 w-4" />
                {submitting ? "启动中..." : "启动任务"}
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="overflow-hidden">
          <CardHeader className="border-b border-border/70 bg-gradient-to-r from-slate-100 to-white">
            <div className="flex items-start justify-between gap-3">
              <div>
                <CardTitle>任务历史</CardTitle>
              </div>
              <Badge variant="secondary">{filteredJobs.length}/{jobs.length}</Badge>
            </div>
            <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索命令、账号、状态" />
          </CardHeader>
          <CardContent className="max-h-[calc(100vh-24rem)] space-y-3 overflow-y-auto p-4">
            {loading ? <div className="text-sm text-muted-foreground">加载任务中...</div> : null}
            {!loading && filteredJobs.length === 0 ? (
              <div className="rounded-md border border-dashed border-border p-8 text-center text-sm text-muted-foreground">-</div>
            ) : null}
            {filteredJobs.map((job) => (
              <div key={job.id} className="rounded-xl border border-border bg-background px-3 py-3">
                <div className="flex items-start justify-between gap-3">
                  <button className="min-w-0 text-left" onClick={() => openLog(job)}>
                    <div className="truncate text-sm font-semibold">{job.title || job.id}</div>
                    <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">{job.command || "无命令记录"}</div>
                  </button>
                  <Badge variant="outline" className={cn("border", statusTone(job.status))}>{job.status || "unknown"}</Badge>
                </div>
                <div className="mt-3 grid gap-2 text-xs text-muted-foreground md:grid-cols-2">
                  <div>账号 {job.account || "全部账号"}</div>
                  <div>开始 {job.started_at || "-"}</div>
                  <div>结束 {job.finished_at || "-"}</div>
                  <div>日志 {job.log_path || "-"}</div>
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Button size="sm" variant="outline" onClick={() => openLog(job)}>查看日志</Button>
                  {isRunning(job.status) ? (
                    <Button size="sm" variant="destructive" onClick={() => stopOne(job.id)} disabled={actionBusy === job.id}>
                      <Square className="h-4 w-4" />
                      {actionBusy === job.id ? "停止中..." : "停止"}
                    </Button>
                  ) : null}
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  );
}

function Field({label, children}) {
  return (
    <div className="space-y-1.5 text-sm">
      <span className="block text-[13px] font-medium">{label}</span>
      {children}
    </div>
  );
}

function CheckOption({label, checked, onChange}) {
  return (
    <label className="flex items-center gap-3 rounded-lg border border-border bg-muted/20 px-3 py-2.5 text-sm">
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span>{label}</span>
    </label>
  );
}
