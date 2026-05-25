import {
  Brain,
  CheckCircle2,
  ClipboardCheck,
  Database,
  Play,
  RefreshCcw,
  Save,
  Search,
  Square,
} from "lucide-react";
import {PageContainer} from "../../components/layout/PageContainer";
import {PageHeader} from "../../components/layout/PageHeader";
import {Badge} from "../../components/ui/Badge";
import {Button} from "../../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../../components/ui/Card";
import {Input} from "../../components/ui/Input";
import {
  Alert,
  EmptyState,
  Field,
  InfoTile,
  MetaRow,
  SummaryCard,
  selectClassName,
  textareaClassName,
} from "../../components/theory/TheoryShared";
import {pageMeta} from "../../lib/iscc";
import {cn} from "../../lib/utils";
import {useTheoryWorkspace} from "./useTheoryWorkspace";

export function TheoryBankPage() {
  const meta = pageMeta.theoryBank;
  const workspace = useTheoryWorkspace();
  const {
    loading,
    error,
    searchQuery,
    setSearchQuery,
    searching,
    searchResult,
    searchError,
    reviewDashboard,
    reviewDraft,
    reviewError,
    reviewItems,
    saveReview,
    savingReview,
    searchResult: result,
    selectedReview,
    selectedReviewID,
    setReviewDraft,
    setSelectedReviewID,
    stats,
    updateReviewOption,
    refreshBankData,
    runSearch,
    aiReviewStatus,
    aiReviewForm,
    setAIReviewForm,
    aiReviewBusy,
    aiReviewError,
    startAIReview,
    stopAIReview,
  } = workspace;

  const summaryCards = [
    {label: "总题数", value: reviewDashboard?.total_questions ?? 0, note: "全部记录", icon: Database, tone: "slate"},
    {label: "已复核", value: reviewDashboard?.reviewed_questions ?? 0, note: "正式可用", icon: CheckCircle2, tone: "emerald"},
    {label: "待复核", value: reviewDashboard?.pending_review ?? 0, note: "-", icon: ClipboardCheck, tone: "amber"},
  ];

  return (
    <PageContainer className="max-w-[1600px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <>
            <Button variant="outline" size="sm" onClick={() => runSearch()} disabled={searching || !searchQuery.trim()}>
              <Search className={cn("h-4 w-4", searching && "animate-pulse")} />
              搜索题库
            </Button>
            <Button variant="outline" size="sm" onClick={refreshBankData} disabled={loading}>
              <RefreshCcw className={cn("h-4 w-4", loading && "animate-spin")} />
              刷新数据
            </Button>
          </>
        }
      />

      {(error || reviewError || searchError || aiReviewError) ? (
        <div className="space-y-3">
          {error ? <Alert tone="destructive" text={error} /> : null}
          {reviewError ? <Alert tone="destructive" text={reviewError} /> : null}
          {searchError ? <Alert tone="destructive" text={searchError} /> : null}
          {aiReviewError ? <Alert tone="destructive" text={aiReviewError} /> : null}
        </div>
      ) : null}

      <div className="grid gap-3 md:grid-cols-3">
        {summaryCards.map((item) => (
          <SummaryCard key={item.label} {...item} />
        ))}
      </div>

      <div className="grid gap-4 xl:grid-cols-[320px_minmax(0,1.32fr)]">
        <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
          <CardHeader className="border-b border-border/70 px-4 py-3">
            <CardTitle className="text-base">人工复核队列</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 p-4">
            <div className="grid gap-2 sm:grid-cols-3">
              <InfoTile label="总题数" value={reviewDashboard?.total_questions ?? 0} note="全部记录" icon={Database} tone="slate" />
              <InfoTile label="已复核" value={reviewDashboard?.reviewed_questions ?? 0} note="正式可用" icon={CheckCircle2} tone="emerald" />
              <InfoTile label="待复核" value={reviewDashboard?.pending_review ?? 0} note="-" icon={ClipboardCheck} tone="amber" />
            </div>

            <div className="max-h-[720px] space-y-2 overflow-auto pr-1">
              {reviewItems.map((item) => (
                <button
                  type="button"
                  key={item.id}
                  onClick={() => {
                    setSelectedReviewID(item.id);
                    setReviewDraft({
                      id: item.id,
                      question: item.question || "",
                      selection_type: item.selection_type || (item.answer_keys?.length > 1 ? "multiple" : "single"),
                      review_status: item.review_status || "approved",
                      review_reason: item.review_reason || "",
                      options: (item.options || []).map((option) => ({
                        ...option,
                        is_correct: (item.answer_keys || []).includes(option.key),
                      })),
                    });
                  }}
                  className={cn(
                    "w-full rounded-xl border px-3 py-2.5 text-left transition-all",
                    item.id === selectedReviewID
                      ? "border-cyan-300 bg-cyan-50 shadow-sm ring-1 ring-cyan-200"
                      : "border-zinc-200 bg-white hover:border-zinc-300 hover:bg-zinc-50",
                  )}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">#{item.id}</Badge>
                    <Badge variant="outline">{item.review_status || "pending"}</Badge>
                    {item.needs_review ? <Badge variant="outline">待复核</Badge> : <Badge variant="outline">已可用</Badge>}
                  </div>
                  <div className="mt-1.5 line-clamp-2 text-[13px] font-semibold leading-5 text-zinc-950">{item.question}</div>
                </button>
              ))}
              {!reviewItems.length ? <EmptyState text="-" compact /> : null}
            </div>
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
            <CardHeader className="border-b border-border/70 px-4 py-3">
              <CardTitle className="text-base">AI 批量复核</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 p-4">
              <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
                <InfoTile label="运行状态" value={aiReviewStatus?.status || "idle"} note={aiReviewStatus?.message || "-"} icon={Brain} tone="blue" />
                <InfoTile label="已处理" value={aiReviewStatus?.reviewed ?? 0} note={`approved ${aiReviewStatus?.approved ?? 0}`} icon={CheckCircle2} tone="emerald" />
                <InfoTile label="剩余" value={aiReviewStatus?.remaining ?? reviewDashboard?.pending_review ?? 0} note={`batch ${aiReviewStatus?.current_batch ?? 0}`} icon={ClipboardCheck} tone="amber" />
                <InfoTile label="数据库" value={stats?.database_path} note={aiReviewStatus?.started_at || "-"} icon={Database} tone="slate" />
              </div>

              <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-4">
                <Field label="限制数量">
                  <Input
                    className="h-9 rounded-lg"
                    value={aiReviewForm.limit}
                    onChange={(event) => setAIReviewForm((current) => ({...current, limit: Number(event.target.value || 0)}))}
                    placeholder="0 = 全部"
                  />
                </Field>
                <Field label="批大小">
                  <Input
                    className="h-9 rounded-lg"
                    value={aiReviewForm.batch_size}
                    onChange={(event) => setAIReviewForm((current) => ({...current, batch_size: Number(event.target.value || 12)}))}
                  />
                </Field>
                <Field label="超时秒数">
                  <Input
                    className="h-9 rounded-lg"
                    value={aiReviewForm.timeout_seconds}
                    onChange={(event) => setAIReviewForm((current) => ({...current, timeout_seconds: Number(event.target.value || 180)}))}
                  />
                </Field>
                <Field label="推理强度">
                  <select
                    className={cn(selectClassName, "h-9 rounded-lg px-2.5 text-[12px]")}
                    value={aiReviewForm.reasoning_effort || "high"}
                    onChange={(event) => setAIReviewForm((current) => ({...current, reasoning_effort: event.target.value}))}
                  >
                    <option value="minimal">minimal</option>
                    <option value="low">low</option>
                    <option value="medium">medium</option>
                    <option value="high">high</option>
                    <option value="xhigh">xhigh</option>
                  </select>
                </Field>
              </div>

              <div className="grid gap-2 md:grid-cols-2">
                <label className="flex items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 text-sm">
                  <input
                    type="checkbox"
                    checked={Boolean(aiReviewForm.only_pending)}
                    onChange={(event) => setAIReviewForm((current) => ({...current, only_pending: event.target.checked}))}
                  />
                  只复核待处理题
                </label>
                <label className="flex items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 text-sm">
                  <input
                    type="checkbox"
                    checked={Boolean(aiReviewForm.dry_run)}
                    onChange={(event) => setAIReviewForm((current) => ({...current, dry_run: event.target.checked}))}
                  />
                  dry-run
                </label>
              </div>

              <div className="flex flex-wrap gap-2">
                <Button onClick={startAIReview} disabled={aiReviewBusy || aiReviewStatus?.running}>
                  <Play className="h-4 w-4" />
                  开始 AI 批量复核
                </Button>
                <Button variant="outline" onClick={stopAIReview} disabled={aiReviewBusy || !aiReviewStatus?.running}>
                  <Square className="h-4 w-4" />
                  停止复核
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
            <CardHeader className="border-b border-border/70 px-4 py-3">
              <CardTitle className="text-base">题库搜索</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 p-4">
              <div className="grid gap-2 lg:grid-cols-[minmax(0,1fr)_140px]">
                <Input
                  value={searchQuery}
                  onChange={(event) => setSearchQuery(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      runSearch();
                    }
                  }}
                  placeholder="输入题干关键字、完整题目或当前理论题"
                  className="h-10 rounded-lg border-zinc-200 bg-zinc-50"
                />
                <Button onClick={() => runSearch()} disabled={searching || !searchQuery.trim()} className="h-10 rounded-lg px-4">
                  <Search className={cn("h-4 w-4", searching && "animate-pulse")} />
                  搜索题库
                </Button>
              </div>

              <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
                <MetaRow label="索引生成时间" value={result?.summary?.generated_at || stats?.generated_at} />
                <MetaRow label="数据库路径" value={result?.summary?.database_path || stats?.database_path} />
                <MetaRow label="待复核数量" value={result?.summary?.review_pending ?? reviewDashboard?.pending_review ?? 0} />
                <MetaRow label="命中过的题" value={result?.summary?.captured_count ?? reviewDashboard?.captured_questions ?? 0} />
                <MetaRow label="累计抓题次数" value={result?.summary?.capture_hits ?? reviewDashboard?.capture_hits ?? 0} />
              </div>

              <div className="space-y-2">
                <div className="text-sm font-semibold text-zinc-950">搜索结果</div>
                <div className="max-h-[260px] space-y-2 overflow-auto pr-1">
                  {(searchResult?.items || []).map((item) => (
                    <div key={item.id} className="rounded-xl border border-zinc-200 bg-gradient-to-br from-white to-zinc-50 p-3 shadow-sm">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="outline">score {item.score}</Badge>
                        {item.multi_answer ? <Badge variant="outline">多答案</Badge> : null}
                      </div>
                      <div className="mt-2 text-[13px] font-semibold leading-5 text-zinc-950">{item.question}</div>
                      <div className="mt-1 text-[11px] leading-4 text-muted-foreground">{item.match_reason}</div>
                    </div>
                  ))}
                </div>
                {!searching && searchQuery.trim() && (searchResult?.items || []).length === 0 ? (
                  <EmptyState text="当前题库里没有找到足够接近的结果。" compact />
                ) : null}
              </div>
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
            <CardHeader className="border-b border-border/70 px-3 py-2.5">
              <CardTitle className="text-[15px]">复核编辑器</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2.5 p-3">
              {!reviewDraft ? (
                <EmptyState text="-" compact />
              ) : (
                <>
                  <div className="flex flex-wrap items-center gap-1.5">
                    <Badge variant="outline">#{selectedReview?.id || reviewDraft.id}</Badge>
                    <Badge variant="outline">{selectedReview?.source_kind || "source"}</Badge>
                    <Badge variant="outline">{selectedReview?.review_status || reviewDraft.review_status || "approved"}</Badge>
                  </div>

                  <Field label="题目">
                    <textarea
                      className={cn(textareaClassName, "min-h-[78px] rounded-lg px-2.5 py-2 text-[12.5px] leading-5")}
                      value={reviewDraft.question || ""}
                      onChange={(event) => setReviewDraft((current) => ({...current, question: event.target.value}))}
                    />
                  </Field>

                  <div className="grid gap-1.5 md:grid-cols-2">
                    <Field label="题型">
                      <select
                        className={cn(selectClassName, "h-8 rounded-md px-2.5 text-[12px]")}
                        value={reviewDraft.selection_type || "single"}
                        onChange={(event) => setReviewDraft((current) => ({...current, selection_type: event.target.value}))}
                      >
                        <option value="single">single</option>
                        <option value="multiple">multiple</option>
                      </select>
                    </Field>
                    <Field label="审核状态">
                      <select
                        className={cn(selectClassName, "h-8 rounded-md px-2.5 text-[12px]")}
                        value={reviewDraft.review_status || "approved"}
                        onChange={(event) => setReviewDraft((current) => ({...current, review_status: event.target.value}))}
                      >
                        <option value="approved">approved</option>
                        <option value="pending">pending</option>
                        <option value="rejected">rejected</option>
                        <option value="captured">captured</option>
                      </select>
                    </Field>
                  </div>

                  <Field label="审核备注">
                    <Input
                      className="h-8 rounded-md px-2.5 text-[12px]"
                      value={reviewDraft.review_reason || ""}
                      onChange={(event) => setReviewDraft((current) => ({...current, review_reason: event.target.value}))}
                      placeholder="例如：新抓取题，已人工确认正确答案"
                    />
                  </Field>

                  <div className="space-y-1.5">
                    {(reviewDraft.options || []).map((option, index) => (
                      <div key={`${option.key}-${index}`} className="rounded-lg border border-zinc-200 bg-zinc-50 p-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <div className="flex h-6 w-6 items-center justify-center rounded-full bg-white text-[11px] font-semibold text-zinc-900 shadow-sm">
                            {option.key}
                          </div>
                          <label className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                            <input
                              type="checkbox"
                              checked={Boolean(option.is_correct)}
                              onChange={(event) => updateReviewOption(index, {is_correct: event.target.checked})}
                            />
                            标记为正确答案
                          </label>
                        </div>
                        <textarea
                          className={cn(textareaClassName, "mt-1.5 min-h-[52px] rounded-md bg-white px-2.5 py-1.5 text-[12.5px] leading-5")}
                          value={option.content || ""}
                          onChange={(event) => updateReviewOption(index, {content: event.target.value})}
                        />
                      </div>
                    ))}
                  </div>

                  <Button onClick={saveReview} disabled={savingReview} className="h-9 w-full rounded-md text-[13px]">
                    <Save className={cn("h-4 w-4", savingReview && "animate-pulse")} />
                    保存复核结果
                  </Button>
                </>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </PageContainer>
  );
}
