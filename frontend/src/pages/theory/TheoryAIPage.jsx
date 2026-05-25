import {Brain, RefreshCcw, Save, Sparkles, Wifi} from "lucide-react";
import {PageContainer} from "../../components/layout/PageContainer";
import {PageHeader} from "../../components/layout/PageHeader";
import {Badge} from "../../components/ui/Badge";
import {Button} from "../../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../../components/ui/Card";
import {Input} from "../../components/ui/Input";
import {
  Alert,
  Field,
  InfoTile,
  MetaRow,
  MetaSwitch,
  SummaryCard,
  selectClassName,
  textareaClassName,
} from "../../components/theory/TheoryShared";
import {formatConfidence} from "../../components/theory/utils";
import {pageMeta} from "../../lib/iscc";
import {cn} from "../../lib/utils";
import {useTheoryWorkspace} from "./useTheoryWorkspace";

export function TheoryAIPage() {
  const meta = pageMeta.theoryAI;
  const workspace = useTheoryWorkspace();
  const {
    ai,
    aiAvailability,
    aiDraft,
    aiError,
    aiSettings,
    error,
    loading,
    match,
    refreshAIPageData,
    saveAISettings,
    savingAI,
    setAIDraft,
    testAISettings,
    testingAI,
  } = workspace;

  const summaryCards = [
    {label: "AI 开关", value: aiDraft?.enabled ? "enabled" : "disabled", note: "当前配置状态", icon: Sparkles, tone: "amber"},
    {label: "当前模型", value: aiDraft?.model || "gpt-5.4", note: "固定使用 GPT-5.4", icon: Brain, tone: "blue"},
    {label: "服务状态", value: aiAvailability?.status || "unknown", note: aiAvailability?.message || "-", icon: Sparkles, tone: aiAvailability?.ok ? "emerald" : "amber"},
  ];

  return (
    <PageContainer className="max-w-[1480px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <Button variant="outline" size="sm" onClick={refreshAIPageData} disabled={loading || savingAI}>
            <RefreshCcw className={cn("h-4 w-4", loading && "animate-spin")} />
            刷新状态
          </Button>
        }
      />

      {(error || aiError) ? (
        <div className="space-y-3">
          {error ? <Alert tone="destructive" text={error} /> : null}
          {aiError ? <Alert tone="destructive" text={aiError} /> : null}
        </div>
      ) : null}

      <div className="grid gap-4 md:grid-cols-3">
        {summaryCards.map((item) => (
          <SummaryCard key={item.label} {...item} />
        ))}
      </div>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
        <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
          <CardHeader className="border-b border-border/70">
            <CardTitle>AI 配置</CardTitle>
          </CardHeader>
          <CardContent className="space-y-5 p-5">
            <div className="grid gap-3 md:grid-cols-2">
              <MetaSwitch
                label="启用 AI 判题"
                checked={Boolean(aiDraft?.enabled)}
                onChange={(checked) => setAIDraft((current) => ({...(current || {}), enabled: checked}))}
              />
              <MetaRow label="配置存储" value={aiSettings?.config_path || "-"} />
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <Field label="Base URL">
                <Input
                  className="h-11 rounded-xl"
                  value={aiDraft?.base_url || ""}
                  onChange={(event) => setAIDraft((current) => ({...(current || {}), base_url: event.target.value}))}
                  placeholder="https://api.openai.com/v1"
                />
              </Field>
              <Field label="模型">
                <Input className="h-11 rounded-xl" value={aiDraft?.model || "gpt-5.4"} readOnly />
              </Field>
              <Field label="API Key">
                <Input
                  className="h-11 rounded-xl"
                  type="password"
                  value={aiDraft?.api_key || ""}
                  onChange={(event) => setAIDraft((current) => ({...(current || {}), api_key: event.target.value}))}
                  placeholder="sk-..."
                />
              </Field>
              <Field label="推理强度">
                <select
                  className={selectClassName}
                  value={aiDraft?.reasoning_effort || "high"}
                  onChange={(event) => setAIDraft((current) => ({...(current || {}), reasoning_effort: event.target.value}))}
                >
                  <option value="minimal">minimal</option>
                  <option value="low">low</option>
                  <option value="medium">medium</option>
                  <option value="high">high</option>
                  <option value="xhigh">xhigh</option>
                </select>
              </Field>
            </div>

            <Field label="系统提示词">
              <textarea
                className={cn(textareaClassName, "min-h-[180px]")}
                value={aiDraft?.prompt || ""}
                onChange={(event) => setAIDraft((current) => ({...(current || {}), prompt: event.target.value}))}
              />
            </Field>

            <div className="grid gap-3 md:grid-cols-2">
              <Button onClick={saveAISettings} disabled={savingAI || !aiDraft} className="w-full">
                <Save className={cn("h-4 w-4", savingAI && "animate-pulse")} />
                保存 AI 设置
              </Button>
              <Button variant="outline" onClick={testAISettings} disabled={testingAI || !aiDraft} className="w-full">
                <Wifi className={cn("h-4 w-4", testingAI && "animate-pulse")} />
                测试可用性
              </Button>
            </div>
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
            <CardHeader className="border-b border-border/70">
              <CardTitle>可用性测试</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 p-5">
              <InfoTile
                label="测试状态"
                value={aiAvailability?.status || "-"}
                note={aiAvailability?.message || "未测试"}
                icon={Wifi}
                tone={aiAvailability?.ok ? "emerald" : "amber"}
              />
              <div className="grid gap-3 md:grid-cols-2">
                <MetaRow label="模型" value={aiAvailability?.model || aiDraft?.model || "gpt-5.4"} />
                <MetaRow label="耗时" value={aiAvailability?.latency_ms ? `${aiAvailability.latency_ms} ms` : "-"} />
                <MetaRow label="HTTP 状态" value={aiAvailability?.http_status_code || "-"} />
                <MetaRow label="检查时间" value={aiAvailability?.checked_at || "-"} />
              </div>
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-zinc-200/80 shadow-sm">
            <CardHeader className="border-b border-border/70">
              <CardTitle>当前 AI 判题状态</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 p-5">
              <InfoTile label="模型" value={ai?.model || "gpt-5.4"} note="当前运行模型" icon={Brain} tone="blue" />
              <InfoTile label="AI 服务状态" value={aiAvailability?.status || "unknown"} note={aiAvailability?.message || "-"} icon={Sparkles} tone={aiAvailability?.ok ? "emerald" : "amber"} />
              <InfoTile label="最近判题结果" value={ai?.status || "disabled"} note={ai?.reason || ai?.error || "尚未触发"} icon={Sparkles} tone="amber" />
              <InfoTile label="匹配状态" value={match?.status || "pending"} note={`${match?.method || "local-bank"} · ${formatConfidence(match?.confidence)}`} icon={Brain} tone="emerald" />
              <div className="rounded-2xl border border-zinc-200 bg-zinc-50 p-4">
                <div className="text-xs font-semibold text-muted-foreground">当前推荐</div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {(ai?.recommended_options || []).length ? (
                    ai.recommended_options.map((item) => (
                      <Badge key={item} variant="outline">{item}</Badge>
                    ))
                  ) : (
                    <span className="text-sm text-muted-foreground">-</span>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </PageContainer>
  );
}
