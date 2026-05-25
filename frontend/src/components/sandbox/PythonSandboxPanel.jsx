import {useEffect, useMemo, useState} from "react";
import {Clock3, FileText, Play, RefreshCcw, TerminalSquare} from "lucide-react";
import {RunPythonSandbox, SandboxProfiles} from "../../../wailsjs/go/desktop/API";
import {starterCode} from "../../lib/iscc";
import {callWails} from "../../lib/wails";
import {cn} from "../../lib/utils";
import {Badge} from "../ui/Badge";
import {Button} from "../ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../ui/Card";

function hasValue(value) {
  return value !== undefined && value !== null && String(value).trim() !== "";
}

function buildContextReadme(scopeLabel, contextTitle, fileCount) {
  return [
    `${scopeLabel || "题目管理"} Python 沙盒`,
    "",
    "已自动注入的上下文文件：",
    "- context/challenge.json",
    fileCount > 1 ? `- 额外文件 ${fileCount - 1} 个` : "-",
    "",
    `当前上下文：${contextTitle || "通用上下文"}`,
    "运行命令：python3 -I main.py",
  ].join("\n");
}

export function PythonSandboxPanel({
  title = "Python 沙盒",
  description = "",
  scopeLabel = "题目管理",
  contextTitle = "",
  contextPayload = null,
  contextBadges = [],
  extraFiles = null,
  initialCode = starterCode,
  className = "",
}) {
  const template = useMemo(() => String(initialCode || starterCode), [initialCode]);
  const [profiles, setProfiles] = useState([]);
  const [profileId, setProfileId] = useState("local-isolated");
  const [code, setCode] = useState(template);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState(null);

  useEffect(() => {
    setCode(template);
  }, [template]);

  useEffect(() => {
    let active = true;
    callWails(() => SandboxProfiles(), [])
      .then((items) => {
        if (!active) {
          return;
        }
        const nextProfiles = items || [];
        setProfiles(nextProfiles);
        setProfileId((current) => {
          if (current && nextProfiles.some((item) => item.id === current)) {
            return current;
          }
          return nextProfiles[0]?.id || "local-isolated";
        });
      })
      .catch((err) => {
        if (active) {
          setError(String(err));
        }
      });
    return () => {
      active = false;
    };
  }, []);

  const activeProfile = useMemo(
    () => profiles.find((item) => item.id === profileId) || profiles[0] || null,
    [profileId, profiles],
  );

  const injectedFiles = useMemo(() => {
    const nextFiles = {...(extraFiles || {})};
    if (!nextFiles["solve.py"] && String(initialCode || "").trim()) {
      nextFiles["solve.py"] = String(initialCode);
    }
    if (contextPayload) {
      nextFiles["context/challenge.json"] = JSON.stringify(
        {
          scope: scopeLabel,
          context_name: contextTitle || scopeLabel,
          payload: contextPayload,
        },
        null,
        2,
      );
    }
    nextFiles["context/README.txt"] = buildContextReadme(scopeLabel, contextTitle, Object.keys(nextFiles).length + 1);
    return nextFiles;
  }, [contextPayload, contextTitle, extraFiles, scopeLabel]);

  async function runSandbox() {
    setBusy(true);
    setError("");
    try {
      const response = await callWails(
        () =>
          RunPythonSandbox({
            code,
            files: injectedFiles,
            timeout_seconds: activeProfile?.default_timeout_seconds || 15,
            profile: profileId || "local-isolated",
          }),
        null,
      );
      setResult(response);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  function resetCode() {
    setCode(template);
    setError("");
    setResult(null);
  }

  return (
    <Card className={cn("overflow-hidden border-zinc-200 shadow-sm", className)}>
      <CardHeader className="border-b border-border/70 bg-[radial-gradient(circle_at_top_left,_rgba(8,145,178,0.14),_transparent_34%),linear-gradient(180deg,_rgba(255,255,255,0.98),_rgba(248,250,252,0.96))]">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle className="flex items-center gap-2 text-base">
                <TerminalSquare className="h-4 w-4" />
                {title}
              </CardTitle>
              <Badge variant="secondary">{activeProfile?.id || "local-isolated"}</Badge>
              <Badge variant="outline">{Object.keys(injectedFiles).length} 个注入文件</Badge>
            </div>
            <CardDescription className="mt-2">{description}</CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{activeProfile?.isolation || "process isolation"}</Badge>
            <Badge variant="outline">{activeProfile?.network_policy || "host network"}</Badge>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4 p-4">
        <div className="rounded-2xl border border-zinc-200 bg-white/90 p-4 shadow-sm">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="text-xs font-semibold uppercase tracking-[0.24em] text-muted-foreground">Context</div>
              <div className="mt-2 text-sm font-semibold text-zinc-950">{contextTitle || `${scopeLabel} 通用上下文`}</div>
            </div>
            <div className="flex items-center gap-2 rounded-full bg-zinc-100 px-3 py-1.5 text-xs text-zinc-700">
              <FileText className="h-3.5 w-3.5" />
              <span>`context/challenge.json` 已注入</span>
            </div>
          </div>
          {contextBadges.length ? (
            <div className="mt-3 flex flex-wrap gap-2">
              {contextBadges
                .filter((item) => item && hasValue(item.value))
                .map((item) => (
                  <MetaPill key={`${item.label}-${item.value}`} label={item.label} value={item.value} />
                ))}
            </div>
          ) : null}
        </div>

        <textarea
          className="min-h-[260px] w-full resize-y rounded-2xl border border-zinc-200 bg-zinc-950/95 p-4 font-mono text-[13px] leading-6 text-zinc-100 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-200"
          value={code}
          onChange={(event) => setCode(event.target.value)}
          spellCheck={false}
        />

        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1 rounded-full bg-zinc-100 px-3 py-1.5">
              <Clock3 className="h-3.5 w-3.5" />
              timeout {activeProfile?.default_timeout_seconds || 15}s
            </span>
            <span className="rounded-full bg-zinc-100 px-3 py-1.5">python3 -I main.py</span>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={resetCode} disabled={busy}>
              <RefreshCcw className="h-4 w-4" />
              重置模板
            </Button>
            <Button size="sm" onClick={runSandbox} disabled={busy}>
              <Play className="h-4 w-4" />
              {busy ? "运行中..." : "运行沙盒"}
            </Button>
          </div>
        </div>

        {error ? <pre className="console-block border-destructive/40 bg-destructive/10">{error}</pre> : null}

        {result ? (
          <div className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <MetaBox label="退出码" value={result.exit_code} />
              <MetaBox label="耗时" value={`${result.duration_ms} ms`} />
              <MetaBox label="工作目录" value={result.workdir || "-"} />
              <MetaBox label="命令" value={(result.command || []).join(" ") || "python3 -I main.py"} />
            </div>
            <div className="grid gap-3 xl:grid-cols-2">
              <div className="space-y-2">
                <div className="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">stdout</div>
                <pre className="console-block">{result.stdout || "[stdout empty]"}</pre>
              </div>
              <div className="space-y-2">
                <div className="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">stderr</div>
                <pre className="console-block bg-muted/70">{result.stderr || "[stderr empty]"}</pre>
              </div>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function MetaPill({label, value}) {
  return (
    <div className="inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-3 py-1.5 text-xs">
      <span className="font-semibold text-muted-foreground">{label}</span>
      <span className="font-medium text-zinc-900">{value}</span>
    </div>
  );
}

function MetaBox({label, value}) {
  return (
    <div className="rounded-xl border border-zinc-200 bg-white px-3 py-3 shadow-sm">
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">{label}</div>
      <div className="mt-2 break-all text-sm text-zinc-950">{value}</div>
    </div>
  );
}
