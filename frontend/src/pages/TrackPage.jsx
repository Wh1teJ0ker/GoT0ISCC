import {useEffect, useMemo, useState} from "react";
import {
  AlertTriangle,
  CheckCircle2,
  FolderOpen,
  RefreshCcw,
  Search,
  Server,
  ShieldAlert,
  Swords,
  Wrench,
} from "lucide-react";
import {PythonSandboxPanel} from "../components/sandbox/PythonSandboxPanel";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {Badge} from "../components/ui/Badge";
import {Button} from "../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../components/ui/Card";
import {callWails} from "../lib/wails";
import {Input} from "../components/ui/Input";
import {cn} from "../lib/utils";

function normalize(text) {
  return String(text || "").trim().toLowerCase();
}

function matchText(challenge, keyword) {
  if (!keyword) {
    return true;
  }
  const haystack = [
    challenge.challenge_id,
    challenge.title,
    challenge.category,
    challenge.kind,
    challenge.key,
    ...(challenge.accounts || []).map((item) => item.account),
    ...(challenge.asset_warnings || []),
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return haystack.includes(keyword);
}

function issueTone(challenge, active) {
  if (challenge.submitted) {
    return active
      ? "border-emerald-400/30 bg-emerald-500/10 text-emerald-50"
      : "border-emerald-200 bg-emerald-50 text-emerald-700";
  }
  if ((challenge.asset_warnings || []).length || challenge.attachment_mismatch) {
    return active
      ? "border-amber-400/30 bg-amber-500/10 text-amber-50"
      : "border-amber-200 bg-amber-50 text-amber-700";
  }
  return active
    ? "border-sky-400/30 bg-sky-500/10 text-sky-50"
    : "border-sky-200 bg-sky-50 text-sky-700";
}

function SummaryCard({label, value, hint, icon: Icon, tone = "slate"}) {
  const toneMap = {
    emerald: "border-emerald-100 bg-emerald-50 text-emerald-700",
    amber: "border-amber-100 bg-amber-50 text-amber-700",
    cyan: "border-cyan-100 bg-cyan-50 text-cyan-700",
    red: "border-rose-100 bg-rose-50 text-rose-700",
    slate: "border-zinc-200 bg-white text-zinc-700",
  };
  return (
    <Card className={cn("overflow-hidden shadow-sm", toneMap[tone])}>
      <CardHeader className="flex-row items-start justify-between space-y-0 p-4">
        <div className="min-w-0">
          <CardDescription className="text-xs">{label}</CardDescription>
          <CardTitle className="mt-2 text-2xl">{value}</CardTitle>
          {hint ? <p className="mt-2 text-xs text-muted-foreground">{hint}</p> : null}
        </div>
        <div className="rounded-md bg-black/5 p-2">
          <Icon className="h-4 w-4" />
        </div>
      </CardHeader>
    </Card>
  );
}

function MetaPill({label, value}) {
  return (
    <div className="inline-flex items-center gap-2 rounded-full border border-border bg-background/90 px-3 py-1.5 text-xs leading-5">
      <span className="font-semibold text-muted-foreground">{label}</span>
      <span className="font-medium text-foreground">{value || "-"}</span>
    </div>
  );
}

export function TrackPage({meta, loader, trackKey}) {
  const [payload, setPayload] = useState(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("");
  const [account, setAccount] = useState("");
  const [submitFilter, setSubmitFilter] = useState("all");
  const [selectedKey, setSelectedKey] = useState("");

  useEffect(() => {
    reload();
  }, [trackKey]);

  const categories = useMemo(() => {
    const values = new Set();
    (payload?.challenges || []).forEach((item) => {
      if (item.category) {
        values.add(item.category);
      }
    });
    return Array.from(values).sort((a, b) => a.localeCompare(b, "zh-CN"));
  }, [payload]);

  const accountOptions = useMemo(() => {
    const values = new Set();
    (payload?.accounts || []).forEach((item) => {
      if (item.name) {
        values.add(item.name);
      }
    });
    return Array.from(values).sort((a, b) => a.localeCompare(b, "zh-CN"));
  }, [payload]);

  const filteredChallenges = useMemo(() => {
    const keyword = normalize(query);
    return (payload?.challenges || []).filter((item) => {
      if (!matchText(item, keyword)) {
        return false;
      }
      if (category && item.category !== category) {
        return false;
      }
      if (account && !(item.accounts || []).some((row) => row.account === account)) {
        return false;
      }
      if (submitFilter === "submitted" && !item.submitted) {
        return false;
      }
      if (submitFilter === "pending" && item.submitted) {
        return false;
      }
      if (submitFilter === "warning" && !(item.asset_warnings || []).length && !item.attachment_mismatch) {
        return false;
      }
      return true;
    });
  }, [payload, query, category, account, submitFilter]);

  const selectedChallenge = useMemo(() => {
    if (!filteredChallenges.length) {
      return null;
    }
    return filteredChallenges.find((item) => item.key === selectedKey) || filteredChallenges[0];
  }, [filteredChallenges, selectedKey]);
  const sandboxContext = useMemo(
    () => ({
      track_key: trackKey,
      track_title: meta.title,
      snapshot_at: payload?.snapshot_at || "",
      source_type: payload?.source_type || "",
      challenge: selectedChallenge,
    }),
    [meta.title, payload?.snapshot_at, payload?.source_type, selectedChallenge, trackKey],
  );
  const sandboxBadges = useMemo(
    () => [
      {label: "类别", value: selectedChallenge?.category || "未分类"},
      {label: "提交状态", value: selectedChallenge?.submitted ? "已提交" : "待补"},
      {label: "附件", value: selectedChallenge?.attachments?.length ?? 0},
      {label: "远程入口", value: selectedChallenge?.remote_targets?.length ?? 0},
    ],
    [selectedChallenge],
  );
  const sandboxExtraFiles = useMemo(() => {
    if (!selectedChallenge?.solve_path || !selectedChallenge?.solve_script) {
      return null;
    }
    return {
      "solve.py": selectedChallenge.solve_script,
    };
  }, [selectedChallenge?.solve_path, selectedChallenge?.solve_script]);

  useEffect(() => {
    if (!selectedChallenge) {
      setSelectedKey("");
      return;
    }
    if (selectedChallenge.key !== selectedKey) {
      setSelectedKey(selectedChallenge.key);
    }
  }, [selectedChallenge, selectedKey]);

  async function reload(silent = false) {
    if (silent) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError("");
    try {
      const next = await callWails(() => loader(), null);
      setPayload(next || null);
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  return (
    <PageContainer className="max-w-[1500px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        description={meta.description}
        action={
          <>
            <Badge variant="outline">{payload?.source_type || "source"}</Badge>
            <Button variant="outline" size="sm" onClick={() => reload(true)} disabled={refreshing}>
              <RefreshCcw className="h-4 w-4" />
              {refreshing ? "刷新中..." : "刷新"}
            </Button>
          </>
        }
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-6">
        <SummaryCard label="题目总数" value={payload?.summary?.total_challenges ?? 0} hint={payload?.snapshot_at || "-"} icon={FolderOpen} />
        <SummaryCard label="已解题目" value={payload?.summary?.solved_challenges ?? 0} hint="至少有一个账号已提交" icon={CheckCircle2} tone="emerald" />
        <SummaryCard label="待补题目" value={payload?.summary?.pending_challenges ?? 0} hint="当前仍未提交" icon={Wrench} tone="cyan" />
        <SummaryCard label="变更题目" value={payload?.summary?.changed_challenges ?? 0} hint="题面或资产变化" icon={Swords} tone="amber" />
        <SummaryCard label="告警题目" value={payload?.summary?.warning_challenges ?? 0} hint="附件/远程状态异常" icon={ShieldAlert} tone="red" />
        <SummaryCard label="接入账号" value={payload?.summary?.total_accounts ?? 0} hint="有提交跟踪记录的账号" icon={Server} />
      </div>

      {error ? (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      ) : null}

      {!error && !loading && !(payload?.challenges || []).length ? (
        <div className="rounded-md border border-dashed border-border bg-muted/20 px-4 py-4 text-sm text-muted-foreground">
          当前没有读取到{payload?.display_name || meta.title}数据。请先执行远端同步，或确认当前运行目录下的 `data/got0iscc.db` 已包含对应赛道数据。
        </div>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-[360px_minmax(0,1fr)]">
        <Card className="overflow-hidden xl:sticky xl:top-8 xl:h-fit">
          <CardHeader className="border-b border-border/70 bg-gradient-to-r from-slate-100 to-white">
            <div className="flex items-start justify-between gap-3">
              <div>
                <CardTitle>{payload?.display_name || meta.title} 列表</CardTitle>
              </div>
              <Badge variant="secondary">{filteredChallenges.length}/{payload?.challenges?.length || 0}</Badge>
            </div>
            <div className="mt-3 space-y-2">
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索题目、类别、账号" className="pl-9" />
              </div>
              <div className="grid gap-2 md:grid-cols-3 xl:grid-cols-1 2xl:grid-cols-3">
                <select className="h-10 rounded-md border border-input bg-background px-3 text-sm" value={category} onChange={(event) => setCategory(event.target.value)}>
                  <option value="">全部类别</option>
                  {categories.map((item) => (
                    <option key={item} value={item}>{item}</option>
                  ))}
                </select>
                <select className="h-10 rounded-md border border-input bg-background px-3 text-sm" value={account} onChange={(event) => setAccount(event.target.value)}>
                  <option value="">全部账号</option>
                  {accountOptions.map((item) => (
                    <option key={item} value={item}>{item}</option>
                  ))}
                </select>
                <select className="h-10 rounded-md border border-input bg-background px-3 text-sm" value={submitFilter} onChange={(event) => setSubmitFilter(event.target.value)}>
                  <option value="all">全部状态</option>
                  <option value="submitted">已提交</option>
                  <option value="pending">未提交</option>
                  <option value="warning">仅看告警</option>
                </select>
              </div>
            </div>
          </CardHeader>
          <CardContent className="max-h-[calc(100vh-18rem)] space-y-3 overflow-y-auto p-4">
            {loading ? <div className="text-sm text-muted-foreground">加载赛道数据中...</div> : null}
            {!loading && filteredChallenges.length === 0 ? (
              <div className="rounded-md border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
                当前筛选条件下没有匹配题目。
              </div>
            ) : null}
            {filteredChallenges.map((item) => {
              const active = selectedChallenge?.key === item.key;
              return (
                <button
                  key={item.key}
                  className={cn(
                    "w-full rounded-xl border px-3 py-3 text-left transition-all",
                    active ? "border-zinc-900 bg-zinc-950 text-zinc-100 shadow-[0_16px_40px_rgba(24,24,27,0.18)]" : "border-border bg-background hover:border-zinc-300 hover:bg-zinc-50",
                  )}
                  onClick={() => setSelectedKey(item.key)}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold">{item.challenge_id} · {item.title}</div>
                      <div className={cn("mt-1 text-xs", active ? "text-zinc-400" : "text-muted-foreground")}>
                        {item.category} · {item.kind || "unknown"}
                      </div>
                    </div>
                    <Badge variant="outline" className={cn("border", issueTone(item, active))}>
                      {item.submitted ? "已提交" : "待补"}
                    </Badge>
                  </div>
                  <div className={cn("mt-3 grid gap-1 text-[11px] leading-5", active ? "text-zinc-400" : "text-muted-foreground")}>
                    <span>提交账号 {item.submitted_account_count || 0}</span>
                    <span>附件 {item.attachments?.length || 0} / 远程 {item.remote_targets?.length || 0}</span>
                    <span>{item.updated_at ? `更新于 ${item.updated_at}` : "-"}</span>
                    {(item.asset_warnings || []).length ? <span>告警 {(item.asset_warnings || []).length} 条</span> : null}
                  </div>
                </button>
              );
            })}
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card className="overflow-hidden">
            <CardHeader className="border-b border-border/70 bg-[radial-gradient(circle_at_top_left,_rgba(2,132,199,0.12),_transparent_36%),linear-gradient(180deg,_rgba(248,250,252,1),_rgba(241,245,249,0.92))]">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle>{selectedChallenge ? `${selectedChallenge.challenge_id} · ${selectedChallenge.title}` : "赛道详情"}</CardTitle>
                    {selectedChallenge?.category ? <Badge variant="secondary">{selectedChallenge.category}</Badge> : null}
                    {selectedChallenge?.changed ? <Badge variant="outline">题面变更</Badge> : null}
                    {selectedChallenge?.attachment_mismatch ? <Badge variant="destructive">附件不一致</Badge> : null}
                  </div>
                  <CardDescription className="mt-2">
                    {selectedChallenge?.detail_url ? `题目详情：${selectedChallenge.detail_url}` : ""}
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4 p-4">
              {selectedChallenge ? (
                <>
                  <div className="flex flex-wrap gap-2 text-sm">
                    <MetaPill label="题目 Key" value={selectedChallenge.key} />
                    <MetaPill label="题目目录" value={selectedChallenge.dir_path || "未记录"} />
                    <MetaPill label="描述文件" value={selectedChallenge.description_path || "未记录"} />
                    <MetaPill label="最近快照" value={selectedChallenge.updated_at || payload?.snapshot_at} />
                  </div>

                  {(selectedChallenge.asset_warnings || []).length ? (
                    <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
                      <div className="mb-2 flex items-center gap-2 font-semibold">
                        <AlertTriangle className="h-4 w-4" />
                        资产告警
                      </div>
                      <div className="space-y-1">
                        {selectedChallenge.asset_warnings.map((item) => (
                          <div key={item}>{item}</div>
                        ))}
                      </div>
                    </div>
                  ) : null}

                  <div className="grid gap-4 xl:grid-cols-[1.05fr_0.95fr]">
                    <Card className="border-dashed shadow-none">
                      <CardHeader className="p-4 pb-0">
                        <CardTitle className="text-base">账号提交状态</CardTitle>
                      </CardHeader>
                      <CardContent className="space-y-3 p-4">
                        {(selectedChallenge.accounts || []).length === 0 ? (
                          <div className="rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">当前题目还没有账号状态记录。</div>
                        ) : (
                          selectedChallenge.accounts.map((item) => (
                            <div key={item.account} className="rounded-lg border border-border bg-white px-3 py-3">
                              <div className="flex items-start justify-between gap-3">
                                <div>
                                  <div className="text-sm font-semibold">{item.account}</div>
                                  <div className="mt-1 text-xs text-muted-foreground">
                                    {item.solver_status || "unknown"} · {item.platform_solved ? "平台已过" : "未过平台"}
                                  </div>
                                </div>
                                <Badge variant={item.submitted ? "default" : "secondary"}>{item.submitted ? "已提交" : "未提交"}</Badge>
                              </div>
                              <div className="mt-3 grid gap-2 text-xs text-muted-foreground md:grid-cols-2">
                                <div>最后提交 {item.last_submitted_at || "-"}</div>
                                <div>最后活跃 {item.last_seen_at || "-"}</div>
                                <div>提交结果 {item.submission_message || "-"}</div>
                                <div>附件 {item.attachment_count || 0} / 远程 {item.remote_target_count || 0}</div>
                              </div>
                            </div>
                          ))
                        )}
                      </CardContent>
                    </Card>

                    <div className="space-y-4">
                      <Card className="border-dashed shadow-none">
                      <CardHeader className="p-4 pb-0">
                        <CardTitle className="text-base">附件信息</CardTitle>
                      </CardHeader>
                        <CardContent className="space-y-3 p-4">
                          {(selectedChallenge.attachments || []).length === 0 ? (
                            <div className="rounded-md border border-dashed border-border p-5 text-sm text-muted-foreground">当前题目没有附件。</div>
                          ) : (
                            selectedChallenge.attachments.map((item) => (
                              <div key={`${item.stored_name || item.name}-${item.sha256}`} className="rounded-lg border border-border bg-muted/10 px-3 py-3">
                                <div className="text-sm font-semibold">{item.name || item.stored_name}</div>
                                <div className="mt-1 break-all text-xs text-muted-foreground">{item.local_path || item.shared_path || item.url || "-"}</div>
                              </div>
                            ))
                          )}
                        </CardContent>
                      </Card>

                      <Card className="border-dashed shadow-none">
                      <CardHeader className="p-4 pb-0">
                        <CardTitle className="text-base">远程入口</CardTitle>
                      </CardHeader>
                        <CardContent className="space-y-3 p-4">
                          {(selectedChallenge.remote_targets || []).length === 0 ? (
                            <div className="rounded-md border border-dashed border-border p-5 text-sm text-muted-foreground">当前题目没有远程目标。</div>
                          ) : (
                            selectedChallenge.remote_targets.map((item, index) => (
                              <div key={`${item.value}-${index}`} className="rounded-lg border border-border bg-muted/10 px-3 py-3">
                                <div className="text-sm font-semibold">{item.value}</div>
                                <div className="mt-1 text-xs text-muted-foreground">{item.kind || "remote"} · {item.host || "-"}:{item.port || "-"}</div>
                              </div>
                            ))
                          )}
                        </CardContent>
                      </Card>
                    </div>
                  </div>
                </>
              ) : (
                <div className="rounded-md border border-dashed border-border p-10 text-center text-sm text-muted-foreground">
                  -
                </div>
              )}
            </CardContent>
          </Card>

          <PythonSandboxPanel
            title={`${meta.title} Python 沙盒`}
            scopeLabel={meta.title}
            contextTitle={selectedChallenge ? `${selectedChallenge.challenge_id} · ${selectedChallenge.title}` : `${meta.title} 通用上下文`}
            contextPayload={sandboxContext}
            contextBadges={sandboxBadges}
            extraFiles={sandboxExtraFiles}
            initialCode={selectedChallenge?.solve_script || ""}
          />
        </div>
      </div>
    </PageContainer>
  );
}
