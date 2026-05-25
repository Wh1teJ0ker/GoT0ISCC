import {useEffect, useMemo, useState} from "react";
import {
  Activity,
  ClipboardCheck,
  Clock3,
  Hash,
  PauseCircle,
  RefreshCcw,
  Sparkles,
  Trophy,
  Wand2,
} from "lucide-react";
import {PageContainer} from "../../components/layout/PageContainer";
import {PageHeader} from "../../components/layout/PageHeader";
import {Badge} from "../../components/ui/Badge";
import {Button} from "../../components/ui/Button";
import {Card, CardContent, CardHeader, CardTitle} from "../../components/ui/Card";
import {Input} from "../../components/ui/Input";
import {
  Alert,
  EmptyState,
  Field,
  InfoTile,
  OptionCard,
} from "../../components/theory/TheoryShared";
import {formatConfidence} from "../../components/theory/utils";
import {pageMeta} from "../../lib/iscc";
import {cn} from "../../lib/utils";
import {useTheoryWorkspace} from "./useTheoryWorkspace";

export function TheoryAutomationPage() {
  const meta = pageMeta.theoryAutomation;
  const workspace = useTheoryWorkspace();
  const {
    payload,
    loading,
    error,
    ai,
    aiAvailability,
    aiRecommendedOptions,
    automationError,
    automationForm,
    automationResult,
    automationStatus,
    manualSubmitBusy,
    manualSubmitError,
    manualSubmitOptions,
    manualSubmitResult,
    match,
    question,
    recommendedOptions,
    loadTheoryAccount,
    refreshAutomationData,
    refreshTheoryRemote,
    reviewDashboard,
    runAutomation,
    runningAutomation,
    selectedAccount,
    setSelectedAccount,
    setAutomationForm,
    stopAutomation,
    submitTheoryManual,
    stats,
    toggleManualSubmitOption,
    loadAISettings,
  } = workspace;
  const [runtimeNow, setRuntimeNow] = useState(Date.now());

  async function handleRefreshStatus() {
    await Promise.all([refreshAutomationData(), loadAISettings()]);
  }

  useEffect(() => {
    if (!runningAutomation) {
      return undefined;
    }
    setRuntimeNow(Date.now());
    const timer = window.setInterval(() => {
      setRuntimeNow(Date.now());
    }, 1000);
    return () => window.clearInterval(timer);
  }, [runningAutomation]);

  const latestStep = automationResult?.results?.[automationResult.results.length - 1] || null;
  const hasLocalSnapshot = Boolean(payload?.cache_status?.has_snapshot);
  const answerableSnapshot = Boolean(payload?.cache_status?.answerable ?? stats?.answerable);
  const completedSnapshot = Boolean(payload?.cache_status?.completed ?? stats?.completed);
  const progressMessage = stats?.progress_message || payload?.cache_status?.message || "";
  const emptyCacheMessage = payload?.cache_status?.last_remote_error || payload?.cache_status?.message || "本地暂无缓存，请点击刷新远端缓存。";
  const statusQuestionNumber = automationStatus?.current_number || latestStep?.next_question_number || latestStep?.question_number || 0;
  const statusQuestionTitle = automationStatus?.current_title || latestStep?.next_question || latestStep?.question || "";
  const runtimeSeconds = useMemo(() => {
    const startedSource = automationStatus?.current_started_at || automationStatus?.started_at;
    if (!runningAutomation || !startedSource) {
      return 0;
    }
    const startedAt = Date.parse(String(startedSource).replace(" ", "T"));
    if (Number.isNaN(startedAt)) {
      return 0;
    }
    return Math.max(0, Math.floor((runtimeNow - startedAt) / 1000));
  }, [runningAutomation, automationStatus?.current_started_at, automationStatus?.started_at, runtimeNow]);
  const isAutomationErrorState = ["failed", "stopped"].includes(automationStatus?.status || "");
  const runningStatusNote = runningAutomation && !isAutomationErrorState
    ? `已运行 ${runtimeSeconds}s`
    : (automationStatus?.message || automationResult?.stopped_reason || "-");
  const displayedQuestionNumber = question?.number || statusQuestionNumber;
  const displayedQuestionTitle = question?.title || statusQuestionTitle || progressMessage;
  const currentTheoryAccount = payload?.test_account?.name || selectedAccount || payload?.selected_account || "Wh1teJ0ker";
  const scoreSyncedWithoutValue = hasLocalSnapshot || Boolean(payload?.cache_status?.last_remote_sync_at) || Boolean(displayedQuestionNumber);
  const displayedScore = stats?.score_text || stats?.current_score || (completedSnapshot ? "已完成，等待远端成绩" : (scoreSyncedWithoutValue ? "远端未返回分数" : "待同步"));
  const scoreNote = stats?.total_score ? `总分 ${stats.total_score}` : (payload?.cache_status?.last_remote_sync_at ? `同步于 ${payload.cache_status.last_remote_sync_at}` : "远端返回后显示");
  const displayedAIMessage = runningAutomation && !isAutomationErrorState
    ? `自动答题正在运行，已运行 ${runtimeSeconds}s`
    : runningAutomation
      ? (automationStatus?.message || "自动答题正在运行")
    : (completedSnapshot ? (progressMessage || "远端当前没有可答题目。") : (latestStep?.decision_reason || ai?.error || ai?.reason || "尚未启用 AI 判题。"));
  const displayedAIStatus = runningAutomation ? (automationStatus?.status || "running") : (ai?.status || "disabled");
  const displayedAIOptions = runningAutomation && latestStep?.submitted_options?.length ? latestStep.submitted_options : aiRecommendedOptions;
  const displayedAIConfidence = runningAutomation && latestStep ? latestStep.confidence : ai?.confidence;
  const localOptions = recommendedOptions || [];
  const aiOptions = aiRecommendedOptions || [];
  const latestOptions = latestStep?.submitted_options || [];
  const manualCanSubmit = manualSubmitOptions.length > 0 && !manualSubmitBusy;

  return (
    <PageContainer className="max-w-[1680px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <>
            <Button variant="outline" size="sm" onClick={refreshAutomationData} disabled={loading || runningAutomation}>
              <RefreshCcw className={cn("h-4 w-4", loading && "animate-spin")} />
              读取本地缓存
            </Button>
            <Button variant="outline" size="sm" onClick={handleRefreshStatus} disabled={loading || runningAutomation}>
              <RefreshCcw className={cn("h-4 w-4", loading && "animate-spin")} />
              刷新状态
            </Button>
            <Button variant="outline" size="sm" onClick={() => refreshTheoryRemote(selectedAccount)} disabled={loading || runningAutomation}>
              <RefreshCcw className={cn("h-4 w-4", loading && "animate-spin")} />
              刷新远端缓存
            </Button>
          </>
        }
      />

      {(error || automationError || manualSubmitError) ? (
        <div className="space-y-3">
          {error ? <Alert tone="destructive" text={error} /> : null}
          {automationError ? <Alert tone="destructive" text={automationError} /> : null}
          {manualSubmitError ? <Alert tone="destructive" text={manualSubmitError} /> : null}
        </div>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(360px,0.9fr)]">
        <div className="space-y-4">
          <Card className="overflow-hidden border-zinc-200/80">
            <CardHeader className="border-b border-border/70 bg-[linear-gradient(180deg,_rgba(255,255,255,0.98),_rgba(248,250,252,0.94))] p-4">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div className="space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle>当前理论题与判题情报</CardTitle>
                    {recommendedOptions.length ? <Badge variant="outline">推荐 {recommendedOptions.join(", ")}</Badge> : null}
                    {match?.is_multi_answer ? <Badge variant="outline">多答案题</Badge> : null}
                  </div>
                </div>
                <div className="grid min-w-[260px] gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  <InfoTile
                    label="当前题号"
                    value={displayedQuestionNumber ? `第 ${displayedQuestionNumber} 题` : "-"}
                    note={completedSnapshot ? `${currentTheoryAccount} / 远端已无下一题` : (runningAutomation && statusQuestionNumber && statusQuestionNumber !== question?.number ? `${currentTheoryAccount} / 运行态第 ${statusQuestionNumber} 题` : `${currentTheoryAccount} / 远端题号`)}
                    icon={Hash}
                    tone="blue"
                  />
                  <InfoTile
                    label="当前分数"
                    value={displayedScore}
                    note={scoreNote}
                    icon={Trophy}
                    tone="emerald"
                  />
                  <InfoTile
                    label="AI 服务状态"
                    value={aiAvailability?.status || "unknown"}
                    note={aiAvailability?.message || "-"}
                    icon={Sparkles}
                    tone={aiAvailability?.ok ? "emerald" : "amber"}
                  />
                </div>
              </div>
            </CardHeader>

            <CardContent className="space-y-3 p-4">
              {!hasLocalSnapshot ? (
                <Alert tone="primary" text={emptyCacheMessage} />
              ) : null}
              {hasLocalSnapshot && !answerableSnapshot ? (
                <Alert tone={completedSnapshot ? "success" : "primary"} text={progressMessage || "远端缓存已同步，但当前页面没有可提交选项。"} />
              ) : null}
              <div className="grid gap-4 xl:grid-cols-[minmax(0,1.18fr)_320px]">
                <div className="space-y-3">
                  <div className="rounded-[18px] border border-zinc-200 bg-white p-3 shadow-sm">
                    <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">题目</div>
                    <div className="mt-2 text-sm font-semibold leading-6 text-zinc-950">{displayedQuestionTitle || emptyCacheMessage}</div>
                  </div>

                  <div className="grid gap-2 lg:grid-cols-2">
                    {(question?.options || []).map((item) => (
                      <OptionCard
                        key={item.key}
                        item={item}
                        active={recommendedOptions.includes(item.key)}
                        aiPicked={aiRecommendedOptions.includes(item.key)}
                      />
                    ))}
                    {!(question?.options || []).length ? <EmptyState text={hasLocalSnapshot ? (progressMessage || "当前没有可提交选项。") : "读取本地缓存不会联网；需要最新题目时点击右上角“刷新远端缓存”。"} compact /> : null}
                  </div>
                </div>

                <div className="space-y-3">
                  <div className="rounded-[18px] border border-amber-100 bg-amber-50/75 p-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <Sparkles className="h-4 w-4 text-amber-500" />
                      <div className="text-sm font-semibold text-zinc-950">AI 辅助判题</div>
                      <Badge variant="outline">{ai?.model || "gpt-5.4"}</Badge>
                    </div>
                    <div className="mt-2 text-sm leading-5 text-zinc-700">{displayedAIMessage}</div>
                    <div className="mt-2 flex flex-wrap gap-2">
                      <Badge variant="outline">{displayedAIStatus}</Badge>
                      {runningAutomation ? <Badge variant="outline">已完成 {automationStatus?.completed ?? 0}/{automationStatus?.max_questions || automationForm.max_questions}</Badge> : null}
                      {displayedAIOptions.length ? <Badge variant="outline">推荐 {displayedAIOptions.join(", ")}</Badge> : null}
                      <Badge variant="outline">{formatConfidence(displayedAIConfidence)}</Badge>
                    </div>
                    <div className="mt-3 grid gap-2 text-xs leading-5 text-zinc-700">
                      <div className="rounded-lg border border-amber-100 bg-white/70 px-2.5 py-2">
                        <span className="font-semibold text-zinc-950">本地题库候选：</span>{localOptions.length ? localOptions.join(", ") : "-"}
                        <span className="ml-2 text-muted-foreground">{match?.reason || ""}</span>
                      </div>
                      <div className="rounded-lg border border-amber-100 bg-white/70 px-2.5 py-2">
                        <span className="font-semibold text-zinc-950">AI 推荐：</span>{aiOptions.length ? aiOptions.join(", ") : "-"}
                        <span className="ml-2 text-muted-foreground">{ai?.reason || ai?.error || ""}</span>
                      </div>
                      <div className="rounded-lg border border-amber-100 bg-white/70 px-2.5 py-2">
                        <span className="font-semibold text-zinc-950">最近提交：</span>{latestOptions.length ? latestOptions.join(", ") : "-"}
                        <span className="ml-2 text-muted-foreground">{latestStep?.message || "等待提交结果"}</span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="space-y-4">
          <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
            <CardHeader className="border-b border-border/70 bg-[linear-gradient(180deg,_rgba(255,255,255,0.98),_rgba(248,250,252,0.94))] p-4">
              <CardTitle>自动答题与进度</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 p-4">
                <div className="rounded-xl border border-cyan-200 bg-cyan-50/70 px-3 py-2.5 text-sm text-zinc-700 shadow-sm">
                  当前账号：<span className="font-semibold text-zinc-950">{payload?.test_account?.name || selectedAccount || "Wh1teJ0ker"}</span>
                  <span className="mx-2 text-cyan-800/60">/</span>
                  <span>当前题号：</span><span className="font-semibold text-zinc-950">{displayedQuestionNumber ? `第 ${displayedQuestionNumber} 题` : "-"}</span>
                  <span className="mx-2 text-cyan-800/60">/</span>
                  <span>分数：</span><span className="font-semibold text-zinc-950">{displayedScore}</span>
                </div>

              <div className="grid gap-2.5">
                <Field label="理论题账号">
                  <select
                    className="h-10 w-full rounded-xl border border-zinc-200 bg-white px-3 text-sm"
                    value={selectedAccount || payload?.selected_account || ""}
                    onChange={(event) => {
                      const next = event.target.value;
                      setSelectedAccount(next);
                      loadTheoryAccount(next);
                    }}
                  >
                    {(payload?.accounts || []).map((item) => (
                      <option key={item.name} value={item.name}>
                        {item.name} {item.username ? `(${item.username})` : ""}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="本轮最大题数">
                  <Input
                    type="number"
                    min="1"
                    max="200"
                    value={automationForm.max_questions}
                    onChange={(event) => setAutomationForm((current) => ({...current, max_questions: Number(event.target.value || 1)}))}
                    className="h-10 rounded-xl bg-white"
                  />
                </Field>
                <div className="rounded-xl border border-emerald-200 bg-emerald-50/80 px-3 py-2.5 text-sm leading-5 text-emerald-900">
                  自动化策略：先用本地题库生成候选答案，再强制交给 AI 复核；AI 高置信才提交；等待 AI 返回期间持续显示已运行时长，直到手动停止或请求报错。
                </div>
              </div>

              <Button onClick={runAutomation} disabled={runningAutomation || loading || completedSnapshot} className="h-10 w-full rounded-xl px-4">
                <Wand2 className={cn("h-4 w-4", runningAutomation && "animate-pulse")} />
                {runningAutomation ? "自动答题中..." : (completedSnapshot ? "理论题已完成" : "开始自动答题")}
              </Button>

              <Button variant="outline" onClick={stopAutomation} disabled={!runningAutomation} className="h-10 w-full rounded-xl px-4">
                <PauseCircle className="h-4 w-4" />
                停止自动答题
              </Button>

              <div className="grid gap-2 sm:grid-cols-2">
                <InfoTile label="缓存时间" value={payload?.cache_status?.cached_at || "-"} note={payload?.cache_status?.source || "local"} icon={Clock3} tone="slate" />
                <InfoTile label="最近远端同步" value={payload?.cache_status?.last_remote_sync_at || "-"} note={payload?.cache_status?.last_remote_error || progressMessage || "-"} icon={RefreshCcw} tone="blue" />
                <InfoTile
                  label="命中过的题"
                  value={reviewDashboard?.captured_questions ?? 0}
                  note={`累计抓题 ${reviewDashboard?.capture_hits ?? 0} 次`}
                  icon={ClipboardCheck}
                  tone="amber"
                />
                <InfoTile
                  label="运行状态"
                  value={automationStatus?.status || (runningAutomation ? "running" : "idle")}
                  note={runningStatusNote}
                  icon={Activity}
                  tone="blue"
                />
              </div>

              <div className="rounded-xl border border-zinc-200 bg-white p-3 shadow-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-semibold text-zinc-950">人工选择并提交当前题</div>
                    <div className="mt-0.5 text-xs text-muted-foreground">AI 等待或结果不确定时，可以手动选择答案提交。</div>
                  </div>
                  {manualSubmitResult?.message ? <Badge variant="outline">{manualSubmitResult.message}</Badge> : null}
                </div>
                <div className="mt-3 grid gap-2 sm:grid-cols-2">
                  {(question?.options || []).map((item) => (
                    <button
                      key={item.key}
                      type="button"
                      onClick={() => toggleManualSubmitOption(item.key)}
                      className={cn(
                        "rounded-lg border px-3 py-2 text-left text-xs transition-colors",
                        manualSubmitOptions.includes(item.key)
                          ? "border-cyan-300 bg-cyan-50 text-cyan-950"
                          : "border-zinc-200 bg-zinc-50 text-zinc-800 hover:border-zinc-300",
                      )}
                    >
                      <div className="font-semibold">{item.key}</div>
                      <div className="mt-1 line-clamp-2 leading-4">{item.content}</div>
                    </button>
                  ))}
                  {!(question?.options || []).length ? <EmptyState text="暂无可提交选项，请先刷新远端缓存。" compact /> : null}
                </div>
                <Button onClick={submitTheoryManual} disabled={!manualCanSubmit} className="mt-3 h-10 w-full rounded-xl">
                  <Wand2 className={cn("h-4 w-4", manualSubmitBusy && "animate-pulse")} />
                  {manualSubmitBusy ? "人工提交中..." : `人工提交 ${manualSubmitOptions.join(", ") || ""}`}
                </Button>
              </div>

              {latestStep ? (
                <div className="rounded-xl border border-zinc-200 bg-zinc-50/80 p-3 text-sm">
                  <div className="font-semibold text-zinc-950">最近一题执行结果</div>
                  <div className="mt-2 grid gap-1.5 text-xs leading-5 text-zinc-700">
                    <div>题号：第 {latestStep.question_number || "-"} 题</div>
                    <div>决策：{latestStep.decision_stage || latestStep.decision_source || "-"} / 置信度 {formatConfidence(latestStep.confidence)}</div>
                    <div>提交答案：{(latestStep.submitted_options || []).join(", ") || "-"}</div>
                    <div>AI 重试：{latestStep.ai_attempts || 0} 次</div>
                    <div>结果：{latestStep.message || "-"}</div>
                  </div>
                </div>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </div>
    </PageContainer>
  );
}
