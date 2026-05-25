import {useEffect, useMemo, useState} from "react";
import {
  CheckCircle2,
  CloudDownload,
  Database,
  FileText,
  KeyRound,
  Megaphone,
  RefreshCcw,
  Send,
  Shield,
  Trophy,
  UsersRound,
  XCircle,
} from "lucide-react";
import {Accounts, CombatTrack, RefreshCombatTrack, SubmitCombat} from "../../wailsjs/go/desktop/API";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {Badge} from "../components/ui/Badge";
import {Button} from "../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../components/ui/Card";
import {Input} from "../components/ui/Input";
import {pageMeta} from "../lib/iscc";
import {cn} from "../lib/utils";

const ALL_ACCOUNTS = "__all__";

export function CombatPage() {
  const meta = pageMeta.combat;
  const [payload, setPayload] = useState(null);
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [submitResult, setSubmitResult] = useState(null);
  const [form, setForm] = useState({
    account_name: ALL_ACCOUNTS,
    challenge_ids: [],
    flag: "",
  });

  useEffect(() => {
    reload();
  }, []);

  const challenges = payload?.challenges || [];
  const enabledAccounts = useMemo(() => (accounts || []).filter((item) => item.enabled), [accounts]);
  const selectedChallengeSet = useMemo(() => new Set(form.challenge_ids || []), [form.challenge_ids]);
  const selectedChallenges = useMemo(
    () => challenges.filter((item) => selectedChallengeSet.has(item.id)),
    [challenges, selectedChallengeSet],
  );
  const selectedAccountLabel = form.account_name === ALL_ACCOUNTS ? "全部启用账号" : form.account_name || "未选择";
  const submitResults = submitResult?.results || [];

  async function reload() {
    setLoading(true);
    setError("");
    try {
      const [combatPayload, accountsPayload] = await Promise.all([CombatTrack(), Accounts()]);
      const nextAccounts = accountsPayload?.accounts || [];
      setPayload(combatPayload || null);
      setAccounts(nextAccounts);
      setForm((current) => {
        const accountExists = current.account_name === ALL_ACCOUNTS || nextAccounts.some((item) => item.name === current.account_name);
        return {
          ...current,
          account_name: accountExists ? current.account_name : ALL_ACCOUNTS,
          challenge_ids: (current.challenge_ids || []).filter((id) => (combatPayload?.challenges || []).some((item) => item.id === id)),
        };
      });
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  }

  async function refreshCache() {
    setRefreshing(true);
    setError("");
    try {
      const [combatPayload, accountsPayload] = await Promise.all([RefreshCombatTrack(), Accounts()]);
      const nextAccounts = accountsPayload?.accounts || [];
      setPayload(combatPayload || null);
      setAccounts(nextAccounts);
      setMessage(`实战题缓存已刷新，时间 ${combatPayload?.snapshot_at || "-"}`);
      setForm((current) => {
        const accountExists = current.account_name === ALL_ACCOUNTS || nextAccounts.some((item) => item.name === current.account_name);
        return {
          ...current,
          account_name: accountExists ? current.account_name : ALL_ACCOUNTS,
          challenge_ids: (current.challenge_ids || []).filter((id) => (combatPayload?.challenges || []).some((item) => item.id === id)),
        };
      });
    } catch (err) {
      setError(String(err));
    } finally {
      setRefreshing(false);
    }
  }

  function updateField(key, value) {
    setForm((current) => ({...current, [key]: value}));
  }

  function toggleChallenge(id) {
    setForm((current) => {
      const existing = new Set(current.challenge_ids || []);
      if (existing.has(id)) {
        existing.delete(id);
      } else {
        existing.add(id);
      }
      return {...current, challenge_ids: Array.from(existing)};
    });
  }

  function selectAllChallenges() {
    updateField("challenge_ids", challenges.map((item) => item.id));
  }

  async function submitSelected() {
    setSubmitting(true);
    setError("");
    setMessage("");
    setSubmitResult(null);
    try {
      const result = await SubmitCombat({
        account_name: form.account_name,
        challenge_ids: form.challenge_ids,
        flag: form.flag,
      });
      setSubmitResult(result || null);
      setMessage(`已完成 ${result?.total || 0} 条提交流程，成功 ${result?.success_count || 0} 条`);
    } catch (err) {
      setError(String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <PageContainer className="max-w-[1480px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <Button variant="outline" size="sm" onClick={refreshCache} disabled={loading || refreshing || submitting}>
            <RefreshCcw className={cn("h-4 w-4", refreshing && "animate-spin")} />
            {refreshing ? "刷新缓存中..." : "刷新缓存"}
          </Button>
        }
      />

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <SummaryCard label="实战题数" value={payload?.summary?.challenge_count ?? 0} icon={KeyRound} tone="emerald" />
        <SummaryCard label="下载资源" value={payload?.summary?.resource_count ?? 0} icon={CloudDownload} tone="blue" />
        <SummaryCard label="榜单人数" value={payload?.summary?.scoreboard_count ?? 0} icon={Trophy} tone="amber" />
        <SummaryCard label="提交账号" value={selectedAccountLabel} icon={UsersRound} tone="slate" />
        <SummaryCard
          label={payload?.summary?.using_cache ? "本地缓存" : "已选题目"}
          value={payload?.summary?.using_cache ? (payload?.summary?.cache_updated_at || payload?.snapshot_at || "-") : selectedChallenges.length}
          icon={payload?.summary?.using_cache ? Database : Shield}
          tone="slate"
        />
      </div>

      {(message || error) ? (
        <div className={cn("rounded-lg border px-4 py-3 text-sm", error ? "border-destructive/40 bg-destructive/10 text-destructive" : "border-primary/30 bg-primary/10 text-primary")}>
          {error || message}
        </div>
      ) : null}

      <div className="grid gap-5 xl:grid-cols-[0.9fr_1.1fr]">
        <AnnouncementBoard payload={payload} loading={loading} />

        <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
          <CardHeader className="border-b border-border/70 bg-zinc-50/70">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <CardTitle>题目提交</CardTitle>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button variant="outline" size="sm" onClick={selectAllChallenges} disabled={!challenges.length || submitting}>
                  全选题目
                </Button>
                <Button variant="outline" size="sm" onClick={() => updateField("challenge_ids", [])} disabled={!form.challenge_ids.length || submitting}>
                  清空
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4 p-4">
            <div className="grid gap-3 lg:grid-cols-[0.9fr_1.1fr]">
              <Field label="提交账号">
                <select
                  className="flex h-11 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={form.account_name}
                  onChange={(event) => updateField("account_name", event.target.value)}
                >
                  <option value={ALL_ACCOUNTS}>全部启用账号</option>
                  {enabledAccounts.map((item) => (
                    <option key={item.id} value={item.name}>
                      {item.name} · {item.username}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="统一提交 flag">
                <Input
                  value={form.flag}
                  onChange={(event) => updateField("flag", event.target.value)}
                  placeholder="iscc2026{...}"
                  className="h-11"
                />
              </Field>
            </div>

            <div className="grid gap-3 md:grid-cols-4">
              <MetaRow label="提交 action" value={payload?.submission?.action} />
              <MetaRow label="flag 字段" value={payload?.submission?.flag_field} />
              <MetaRow label="题目字段" value={payload?.submission?.challenge_field} />
              <MetaRow label="nonce" value={payload?.nonce || "-"} />
            </div>

            <div className="rounded-2xl border border-border bg-zinc-50/60 p-3">
              <div className="mb-3 flex items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold text-zinc-950">实战题列表</div>
                </div>
                <Badge variant="outline" className="bg-white">已选 {selectedChallenges.length}</Badge>
              </div>
              <div className="log-session-list max-h-[520px] space-y-2 overflow-y-auto pr-1">
                {challenges.map((item) => {
                  const selected = selectedChallengeSet.has(item.id);
                  const latestResults = (submitResult?.results || []).filter((row) => row.challenge_id === item.id);
                  return (
                    <div
                      key={item.id}
                      className={cn(
                        "rounded-xl border bg-white p-3 transition-colors",
                        selected ? "border-cyan-300 ring-1 ring-cyan-100" : "border-border",
                      )}
                    >
                      <div className="flex items-start justify-between gap-3">
                        <button
                          type="button"
                          onClick={() => toggleChallenge(item.id)}
                          className="grid min-w-0 flex-1 grid-cols-[24px_minmax(0,1fr)] gap-3 text-left"
                        >
                          <span
                            className={cn(
                              "mt-0.5 flex h-5 w-5 items-center justify-center rounded border text-[10px] font-bold",
                              selected ? "border-cyan-500 bg-cyan-500 text-white" : "border-zinc-300 bg-white text-zinc-500",
                            )}
                          >
                            {selected ? "✓" : ""}
                          </span>
                          <span className="min-w-0">
                            <span className="flex flex-wrap items-center gap-2">
                              <span className="font-semibold text-zinc-950">{item.name || `实战题 ${item.id}`}</span>
                              <Badge variant="outline">ID {item.id}</Badge>
                              {item.category ? <Badge variant="outline">{item.category}</Badge> : null}
                              {item.verify_mode ? <Badge variant="outline">{item.verify_mode}</Badge> : null}
                            </span>
                            <span className="mt-1 line-clamp-2 block text-xs leading-5 text-muted-foreground">{item.description || "暂无题目说明。"}</span>
                          </span>
                        </button>
                      </div>

                      <div className="mt-3 flex flex-wrap gap-2 text-xs">
                        <span className="rounded-full bg-zinc-100 px-2.5 py-1 text-zinc-700">分值 {item.value ?? 0}</span>
                        <span className="rounded-full bg-zinc-100 px-2.5 py-1 text-zinc-700">solves {item.solves ?? 0}</span>
                        {(item.files || []).map((file, index) => (
                          <a key={`${item.id}-${file}`} href={file} target="_blank" rel="noreferrer" className="rounded-full border border-border bg-white px-2.5 py-1 text-zinc-700 hover:border-zinc-400">
                            附件 {index + 1}
                          </a>
                        ))}
                      </div>

                      {latestResults.length ? (
                        <div className="mt-3 space-y-2">
                          {latestResults.map((result) => (
                            <ResultLine key={`${result.account_name}-${result.challenge_id}-${result.submitted_at}-${result.message}`} item={result} />
                          ))}
                        </div>
                      ) : null}
                    </div>
                  );
                })}
                {!loading && !challenges.length ? <EmptyState text="当前本地缓存里没有实战题题目列表。" compact /> : null}
              </div>
            </div>

            <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
              <CardHeader className="border-b border-border/70 bg-zinc-50/70">
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <FileText className="h-4 w-4 text-zinc-600" />
                    <div>
                      <CardTitle>提交日志</CardTitle>
                      <CardDescription>
                        显示本次实战题提交的逐条结果，可直接判断是否提交成功。
                      </CardDescription>
                    </div>
                  </div>
                  <Badge variant="outline" className="bg-white">
                    {submitResult ? `成功 ${submitResult.success_count || 0} / ${submitResult.total || 0}` : "暂无记录"}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="space-y-3 p-4">
                {submitResult ? (
                  <div className="grid gap-3 md:grid-cols-4">
                    <MetaRow label="提交时间" value={submitResult.submitted_at} />
                    <MetaRow label="提交账号" value={submitResult.account_name} />
                    <MetaRow label="提交总数" value={String(submitResult.total || 0)} />
                    <MetaRow label="失败条数" value={String(submitResult.failure_count || 0)} />
                  </div>
                ) : null}

                <div className="log-session-list max-h-[360px] space-y-2 overflow-y-auto pr-1">
                  {submitResults.length ? (
                    submitResults.map((item, index) => (
                      <ResultLine
                        key={`${item.account_name}-${item.challenge_id}-${item.submitted_at}-${index}`}
                        item={item}
                      />
                    ))
                  ) : (
                    <EmptyState text="还没有提交日志。提交后会在这里显示每个账号、每道题的返回结果。" compact />
                  )}
                </div>
              </CardContent>
            </Card>

            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-white px-4 py-3">
              <div className="text-sm text-muted-foreground">
                将向 <span className="font-semibold text-zinc-950">{selectedAccountLabel}</span> 提交 <span className="font-semibold text-zinc-950">{selectedChallenges.length}</span> 道题。
              </div>
              <Button onClick={submitSelected} disabled={submitting || !form.account_name || !form.flag.trim() || !form.challenge_ids.length}>
                <Send className="h-4 w-4" />
                {submitting ? "提交中..." : "提交已选题目"}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  );
}

function AnnouncementBoard({payload, loading}) {
  return (
    <Card className="overflow-hidden border-zinc-200 bg-white shadow-sm">
      <CardHeader className="border-b border-border/70 bg-zinc-50/70">
        <div className="flex items-center gap-2">
          <Megaphone className="h-4 w-4 text-zinc-600" />
          <div>
            <CardTitle>公告栏</CardTitle>
            <CardDescription>
              {payload?.summary?.using_cache ? `当前展示本地缓存，更新时间 ${payload?.summary?.cache_updated_at || payload?.snapshot_at || "-"}` : "当前展示最新远端同步结果。"}
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="log-session-list max-h-[760px] space-y-4 overflow-y-auto p-4">
        {loading ? <EmptyState text="正在读取本地实战题缓存..." compact /> : null}

        {(payload?.notices || []).length ? (
          <Section title="重点提示">
            {(payload?.notices || []).map((item) => (
              <div key={item} className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-3 text-sm leading-6 text-amber-900">
                {item}
              </div>
            ))}
          </Section>
        ) : null}

        {(payload?.stages || []).length ? (
          <Section title="阶段说明">
            {(payload?.stages || []).map((stage) => (
              <div key={stage.title} className="rounded-xl border border-border bg-background p-4">
                <div className="text-sm font-semibold">{stage.title}</div>
                <div className="mt-2 space-y-2 text-sm leading-6 text-muted-foreground">
                  {(stage.description || []).map((line, index) => (
                    <div key={`${stage.title}-${index}`}>{line}</div>
                  ))}
                </div>
              </div>
            ))}
          </Section>
        ) : null}

        {(payload?.resources || []).length ? (
          <Section title="资源下载">
            {(payload?.resources || []).map((item) => (
              <a
                key={`${item.label}-${item.url}`}
                href={item.url}
                target="_blank"
                rel="noreferrer"
                className="block rounded-xl border border-border bg-background p-4 hover:border-zinc-400 hover:bg-zinc-50"
              >
                <div className="text-sm font-semibold">{item.label}</div>
                <div className="mt-1 break-all text-xs text-muted-foreground">{item.url}</div>
              </a>
            ))}
          </Section>
        ) : null}

        {(payload?.scoreboard || []).length ? (
          <Section title="通关榜单">
            {(payload?.scoreboard || []).map((row, index) => (
              <div key={`${row.team}-${index}`} className="grid grid-cols-[minmax(0,1fr)_112px_64px] gap-3 rounded-lg border border-border bg-background px-3 py-3 text-sm">
                <div className="truncate font-medium">{row.team}</div>
                <div className="text-muted-foreground">{row.passed_at || "-"}</div>
                <div className="text-right font-semibold">{row.score || "-"}</div>
              </div>
            ))}
          </Section>
        ) : null}

        {!loading && !(payload?.notices || []).length && !(payload?.stages || []).length && !(payload?.resources || []).length && !(payload?.scoreboard || []).length ? (
          <EmptyState text="当前没有同步到公告信息。" compact />
        ) : null}
      </CardContent>
    </Card>
  );
}

function SummaryCard({label, value, icon: Icon, tone = "slate"}) {
  const toneMap = {
    emerald: "border-emerald-100 bg-emerald-50 text-emerald-700",
    amber: "border-amber-100 bg-amber-50 text-amber-700",
    blue: "border-cyan-100 bg-cyan-50 text-cyan-700",
    slate: "border-zinc-200 bg-white text-zinc-700",
  };
  return (
    <Card className={cn("overflow-hidden shadow-sm", toneMap[tone])}>
      <CardHeader className="flex-row items-start justify-between space-y-0 p-4">
        <div className="min-w-0">
          <CardDescription className="text-xs">{label}</CardDescription>
          <CardTitle className="mt-2 truncate text-2xl">{value}</CardTitle>
        </div>
        <div className="rounded-md bg-black/5 p-2">
          <Icon className="h-4 w-4" />
        </div>
      </CardHeader>
    </Card>
  );
}

function Section({title, children}) {
  return (
    <div className="space-y-2">
      <div className="text-xs font-semibold uppercase tracking-wide text-zinc-500">{title}</div>
      {children}
    </div>
  );
}

function Field({label, children}) {
  return (
    <div className="space-y-2">
      <div className="text-xs font-semibold text-muted-foreground">{label}</div>
      {children}
    </div>
  );
}

function MetaRow({label, value}) {
  return (
    <div className="rounded-lg border border-border bg-background px-3 py-3">
      <div className="text-xs font-semibold text-muted-foreground">{label}</div>
      <div className="mt-1 break-all text-sm">{value || "-"}</div>
    </div>
  );
}

function ResultLine({item}) {
  return (
    <div className={cn("rounded-lg border px-3 py-2 text-sm", item.success ? "border-emerald-200 bg-emerald-50 text-emerald-900" : "border-rose-200 bg-rose-50 text-rose-900")}>
      <div className="flex flex-wrap items-center gap-2">
        {item.success ? <CheckCircle2 className="h-4 w-4 text-emerald-600" /> : <XCircle className="h-4 w-4 text-rose-600" />}
        <span className="font-semibold">{item.account_name || "账号"} / {item.challenge_id}</span>
        {item.challenge_name ? <Badge variant="outline" className="bg-white/70">{item.challenge_name}</Badge> : null}
        {item.status_code ? <Badge variant="outline" className="bg-white/70">HTTP {item.status_code}</Badge> : null}
        {item.verify_mode ? <Badge variant="outline" className="bg-white/70">{item.verify_mode}</Badge> : null}
        {item.submitted_at ? <span className="text-[11px] opacity-70">{item.submitted_at}</span> : null}
      </div>
      <div className="mt-1 break-all text-xs font-medium opacity-90">{item.message || "-"}</div>
      {item.raw && item.raw !== item.message ? (
        <div className="mt-1 break-all rounded-md bg-black/5 px-2 py-1 text-[11px] opacity-80">
          {item.raw}
        </div>
      ) : null}
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
