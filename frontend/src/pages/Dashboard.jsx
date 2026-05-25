import {useEffect, useMemo, useState} from "react";
import {
  Activity,
  Archive,
  ArrowUpRight,
  Database,
  Download,
  PackagePlus,
  RefreshCcw,
  ShieldCheck,
  TerminalSquare,
} from "lucide-react";
import {
  DashboardSummary,
  ExportMigrationBundle,
  InitializePythonEnv,
  InstallPythonPackages,
  Overview,
  PythonEnvStatus,
} from "../../wailsjs/go/desktop/API";
import {Badge} from "../components/ui/Badge";
import {Button} from "../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../components/ui/Card";
import {Input} from "../components/ui/Input";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {pageMeta} from "../lib/iscc";
import {cn} from "../lib/utils";

function formatBytes(value) {
  const size = Number(value || 0);
  if (size <= 0) {
    return "-";
  }
  const units = ["B", "KB", "MB", "GB"];
  let current = size;
  let index = 0;
  while (current >= 1024 && index < units.length - 1) {
    current /= 1024;
    index += 1;
  }
  return `${current.toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
}

function InfoRow({label, value}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200 bg-zinc-50/80 px-3 py-3">
      <span className="text-xs font-semibold text-muted-foreground">{label}</span>
      <span className="max-w-[68%] break-all text-right text-sm text-zinc-950">{value || "-"}</span>
    </div>
  );
}

export function Dashboard({overview, health, activeProfile}) {
  const meta = pageMeta.dashboard;
  const [pythonStatus, setPythonStatus] = useState(null);
  const [pythonBusy, setPythonBusy] = useState(false);
  const [pythonError, setPythonError] = useState("");
  const [pythonBinaryInput, setPythonBinaryInput] = useState("");
  const [packageInput, setPackageInput] = useState("");
  const [pythonResult, setPythonResult] = useState(null);
  const [bundleBusy, setBundleBusy] = useState(false);
  const [bundleResult, setBundleResult] = useState(null);
  const [bundleError, setBundleError] = useState("");
  const [liveOverview, setLiveOverview] = useState(overview || null);
  const [trackRows, setTrackRows] = useState([]);

  useEffect(() => {
    reloadDashboard();
  }, []);

  async function reloadDashboard() {
    setPythonError("");
    setBundleError("");
    try {
      const [nextOverview, nextPythonStatus, nextDashboard] = await Promise.all([Overview(), PythonEnvStatus(), DashboardSummary()]);
      setLiveOverview(nextOverview || null);
      setPythonStatus(nextPythonStatus || null);
      setTrackRows(nextDashboard?.rows || []);
    } catch (err) {
      setPythonError(String(err));
    }
  }

  async function initializeManagedPython() {
    setPythonBusy(true);
    setPythonError("");
    try {
      const result = await InitializePythonEnv({
        python_binary: pythonBinaryInput.trim(),
      });
      setPythonResult(result || null);
      const nextStatus = await PythonEnvStatus();
      setPythonStatus(nextStatus || null);
    } catch (err) {
      setPythonError(String(err));
    } finally {
      setPythonBusy(false);
    }
  }

  async function installPackages() {
    const packages = packageInput
      .split(/[\s,]+/)
      .map((item) => item.trim())
      .filter(Boolean);
    if (!packages.length) {
      setPythonError("请先输入要安装的包名");
      return;
    }
    setPythonBusy(true);
    setPythonError("");
    try {
      const result = await InstallPythonPackages({packages});
      setPythonResult(result || null);
      const nextStatus = await PythonEnvStatus();
      setPythonStatus(nextStatus || null);
    } catch (err) {
      setPythonError(String(err));
    } finally {
      setPythonBusy(false);
    }
  }

  async function exportBundle() {
    setBundleBusy(true);
    setBundleError("");
    try {
      const result = await ExportMigrationBundle();
      setBundleResult(result || null);
    } catch (err) {
      setBundleError(String(err));
    } finally {
      setBundleBusy(false);
    }
  }

  const stats = useMemo(() => {
    return [
      {
        label: "运行状态",
        value: health?.status || "loading",
        note: health?.message || "桌面主进程状态",
        icon: Activity,
      },
      {
        label: "Python 环境",
        value: pythonStatus?.ready ? "Ready" : "Not Ready",
        note: pythonStatus?.active_python_binary || "-",
        icon: TerminalSquare,
      },
      {
        label: "当前沙盒 Profile",
        value: activeProfile?.id || "local-isolated",
        note: activeProfile?.isolation || "process + temporary workspace",
        icon: ShieldCheck,
      },
      {
        label: "数据目录",
        value: "data/",
        note: liveOverview?.workspace?.app_data_root || overview?.workspace?.app_data_root || "-",
        icon: Database,
      },
    ];
  }, [activeProfile, health, liveOverview, overview, pythonStatus]);

  return (
    <PageContainer className="max-w-[1680px]">
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <>
            <Button variant="outline" size="sm" onClick={reloadDashboard} disabled={pythonBusy || bundleBusy}>
              <RefreshCcw className={cn("h-4 w-4", (pythonBusy || bundleBusy) && "animate-spin")} />
              刷新
            </Button>
            <Button size="sm" onClick={exportBundle} disabled={bundleBusy}>
              <Archive className="h-4 w-4" />
              {bundleBusy ? "导出中..." : "导出迁移包"}
            </Button>
          </>
        }
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {stats.map((item) => (
          <Card key={item.label} className="overflow-hidden border-zinc-200 shadow-sm">
            <CardHeader className="flex-row items-start justify-between space-y-0">
              <div className="space-y-1">
                <CardDescription>{item.label}</CardDescription>
                <CardTitle className="text-2xl">{item.value}</CardTitle>
              </div>
              <div className="rounded-md bg-secondary p-2 text-secondary-foreground">
                <item.icon className="h-4 w-4" />
              </div>
            </CardHeader>
            <CardContent>
              <p className="text-xs leading-5 text-muted-foreground">{item.note}</p>
            </CardContent>
          </Card>
        ))}
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <Card className="overflow-hidden border-zinc-200 shadow-sm">
          <CardHeader className="border-b border-border/70">
            <div className="flex items-center justify-between gap-3">
              <div>
                <CardTitle>受管 Python 环境</CardTitle>
              </div>
              <Badge variant={pythonStatus?.ready ? "secondary" : "outline"}>{pythonStatus?.ready ? "Ready" : "Pending"}</Badge>
            </div>
          </CardHeader>
          <CardContent className="space-y-5 p-5">
            <div className="grid gap-3 md:grid-cols-2">
              <InfoRow label="平台" value={pythonStatus ? `${pythonStatus.platform}/${pythonStatus.architecture}` : "-"} />
              <InfoRow label="受管目录" value={pythonStatus?.managed_root} />
              <InfoRow label="受管解释器" value={pythonStatus?.managed_python_binary} />
              <InfoRow label="当前启用解释器" value={pythonStatus?.active_python_binary} />
              <InfoRow label="当前来源" value={pythonStatus?.active_source} />
              <InfoRow label="初始化策略" value={pythonStatus?.strategy} />
              <InfoRow label="fallback 配置" value={pythonStatus ? `${pythonStatus.fallback_enabled ? "已启用" : "未启用"} / ${pythonStatus.fallback_configured ? "当前平台已配置" : "当前平台未配置"}` : "-"} />
              <InfoRow label="fallback 配置文件" value={pythonStatus?.fallback_config_path} />
            </div>

            <div className="space-y-3">
              <div className="text-sm font-semibold text-zinc-950">本地探测到的 Python</div>
              <div className="rounded-2xl border border-zinc-200 bg-white p-4">
                {(pythonStatus?.detected_candidates || []).length ? (
                  <div className="flex flex-wrap gap-2">
                    {pythonStatus.detected_candidates.map((item) => (
                      <Badge key={item} variant="outline">{item}</Badge>
                    ))}
                  </div>
                ) : (
                  <div className="text-sm text-muted-foreground">-</div>
                )}
              </div>
            </div>

            <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto]">
              <Input
                value={pythonBinaryInput}
                onChange={(event) => setPythonBinaryInput(event.target.value)}
                placeholder="指定 Python 路径"
                className="h-11 rounded-xl"
              />
              <Button onClick={initializeManagedPython} disabled={pythonBusy} className="h-11 px-5">
                <TerminalSquare className="h-4 w-4" />
                {pythonBusy ? "初始化中..." : "初始化内部 Python"}
              </Button>
            </div>

            <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto]">
              <Input
                value={packageInput}
                onChange={(event) => setPackageInput(event.target.value)}
                placeholder="requests pwntools"
                className="h-11 rounded-xl"
              />
              <Button variant="outline" onClick={installPackages} disabled={pythonBusy} className="h-11 px-5">
                <PackagePlus className="h-4 w-4" />
                安装依赖包
              </Button>
            </div>

            {pythonError ? <div className="rounded-xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">{pythonError}</div> : null}

            {pythonResult ? (
              <div className="space-y-3 rounded-2xl border border-zinc-200 bg-zinc-50/80 p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={pythonResult.ok ? "secondary" : "outline"}>{pythonResult.ok ? "执行成功" : "执行失败"}</Badge>
                  <Badge variant="outline">exit {pythonResult.exit_code}</Badge>
                  <Badge variant="outline">{pythonResult.duration_ms} ms</Badge>
                </div>
                <InfoRow label="命令" value={(pythonResult.command || []).join(" ")} />
                <div className="grid gap-3 xl:grid-cols-2">
                  <div className="rounded-xl border border-zinc-200 bg-white p-3">
                    <div className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">stdout</div>
                    <pre className="max-h-[220px] overflow-auto whitespace-pre-wrap break-all text-xs text-zinc-900">{pythonResult.stdout || "[stdout empty]"}</pre>
                  </div>
                  <div className="rounded-xl border border-zinc-200 bg-white p-3">
                    <div className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">stderr</div>
                    <pre className="max-h-[220px] overflow-auto whitespace-pre-wrap break-all text-xs text-zinc-900">{pythonResult.stderr || "[stderr empty]"}</pre>
                  </div>
                </div>
              </div>
            ) : null}

            <div className="space-y-3">
              <div className="text-sm font-semibold text-zinc-950">已安装依赖</div>
              <div className="max-h-[220px] overflow-auto rounded-2xl border border-zinc-200 bg-white p-4">
                {(pythonStatus?.installed_packages || []).length ? (
                  <div className="flex flex-wrap gap-2">
                    {pythonStatus.installed_packages.map((item) => (
                      <Badge key={item} variant="outline">{item}</Badge>
                    ))}
                  </div>
                ) : (
                  <div className="text-sm text-muted-foreground">-</div>
                )}
              </div>
            </div>
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card className="overflow-hidden border-zinc-200 shadow-sm">
            <CardHeader className="border-b border-border/70">
              <CardTitle>数据迁移打包</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 p-5">
              <div className="grid gap-3">
                <InfoRow label="主数据库" value={liveOverview?.workspace?.app_database_path || overview?.workspace?.app_database_path} />
                <InfoRow label="数据目录" value={liveOverview?.workspace?.app_data_root || overview?.workspace?.app_data_root} />
                <InfoRow label="运行目录" value={liveOverview?.workspace?.app_runtime_root || overview?.workspace?.app_runtime_root} />
              </div>

              <Button onClick={exportBundle} disabled={bundleBusy} className="h-11 w-full">
                <Download className="h-4 w-4" />
                {bundleBusy ? "正在导出迁移包..." : "导出迁移包"}
              </Button>

              {bundleError ? <div className="rounded-xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">{bundleError}</div> : null}

              {bundleResult ? (
                <div className="space-y-3 rounded-2xl border border-zinc-200 bg-zinc-50/80 p-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">导出完成</Badge>
                    <Badge variant="outline">{formatBytes(bundleResult.size_bytes)}</Badge>
                  </div>
                  <InfoRow label="压缩包路径" value={bundleResult.archive_path} />
                  <InfoRow label="SHA256" value={bundleResult.sha256} />
                  <InfoRow label="创建时间" value={bundleResult.created_at} />
                  <div className="rounded-xl border border-zinc-200 bg-white p-3">
                    <div className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">包含文件</div>
                    <div className="max-h-[220px] overflow-auto space-y-2">
                      {(bundleResult.files || []).map((item) => (
                        <div key={item} className="text-xs break-all text-zinc-900">{item}</div>
                      ))}
                    </div>
                  </div>
                </div>
              ) : null}
            </CardContent>
          </Card>

          <Card className="overflow-hidden border-zinc-200 shadow-sm">
            <CardHeader className="border-b border-border/70">
              <CardTitle>赛道进度</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <div className="overflow-hidden rounded-b-lg">
                <table className="w-full text-left text-sm">
                  <thead className="bg-muted/70 text-xs text-muted-foreground">
                    <tr>
                      <th className="px-4 py-3 font-medium">赛道</th>
                      <th className="px-4 py-3 font-medium">题目</th>
                      <th className="px-4 py-3 font-medium">已提交</th>
                      <th className="px-4 py-3 font-medium">WP</th>
                      <th className="px-4 py-3 font-medium">缺口</th>
                    </tr>
                  </thead>
                  <tbody>
                    {trackRows.map((row) => (
                      <tr key={row.track} className="border-t border-border">
                        <td className="px-4 py-3 font-medium">{row.track}</td>
                        <td className="px-4 py-3 text-muted-foreground">{row.total}</td>
                        <td className="px-4 py-3 text-muted-foreground">{row.submitted}</td>
                        <td className="px-4 py-3 text-muted-foreground">{row.wp}</td>
                        <td className="px-4 py-3">
                          <Badge variant="outline">{row.missing}</Badge>
                        </td>
                      </tr>
                    ))}
                    {!trackRows.length ? (
                      <tr className="border-t border-border">
                        <td className="px-4 py-6 text-center text-sm text-muted-foreground" colSpan={5}>-</td>
                      </tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </PageContainer>
  );
}
