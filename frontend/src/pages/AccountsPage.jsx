import {useEffect, useMemo, useState} from "react";
import {
  Activity,
  Database,
  KeyRound,
  Plus,
  Save,
  ShieldCheck,
  Trash2,
  UserCheck,
  UserRoundCog,
  UserX,
  Search,
} from "lucide-react";
import {
  Accounts,
  DeleteAccount,
  NetworkProxy,
  SaveAccount,
  SaveNetworkProxy,
} from "../../wailsjs/go/desktop/API";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {Badge} from "../components/ui/Badge";
import {Button} from "../components/ui/Button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "../components/ui/Card";
import {Input} from "../components/ui/Input";
import {pageMeta} from "../lib/iscc";
import {cn} from "../lib/utils";

const emptyAccount = {
  id: 0,
  name: "",
  username: "",
  password: "",
  enabled: true,
  submit_priority: 0,
};

const proxyTypes = [
  {value: "", label: "直连"},
  {value: "http", label: "HTTP"},
  {value: "https", label: "HTTPS"},
  {value: "socks4", label: "SOCKS4"},
  {value: "socks5", label: "SOCKS5"},
  {value: "script", label: "脚本代理"},
];

const emptyNetworkProxy = {
  enabled: false,
  type: "",
  host: "",
  port: 0,
  username: "",
  password: "",
  login_attempts: 6,
  login_retry_delay_seconds: 3,
};

function fieldValue(account, key) {
  return account?.[key] ?? emptyAccount[key] ?? "";
}

function accountStatus(account) {
  if (!account?.enabled) {
    return {label: "已停用", variant: "secondary"};
  }
  if (account?.runtime?.login_status === "ok") {
    return {label: "已连通", variant: "outline"};
  }
  return {label: "已启用", variant: "default"};
}

function editableAccount(account) {
  return {
    id: Number(account?.id || 0),
    name: String(account?.name || ""),
    username: String(account?.username || ""),
    password: String(account?.password || ""),
    enabled: Boolean(account?.enabled ?? true),
    submit_priority: Number(account?.submit_priority || 0),
  };
}

function normalizeProxyDraft(proxy) {
  const enabled = Boolean(proxy?.enabled);
  return {
    enabled,
    type: enabled ? String(proxy?.type || "") : "",
    host: enabled ? String(proxy?.host || "") : "",
    port: enabled ? Number(proxy?.port || 0) : 0,
    username: enabled ? String(proxy?.username || "") : "",
    password: enabled ? String(proxy?.password || "") : "",
    login_attempts: Math.max(1, Number(proxy?.login_attempts || emptyNetworkProxy.login_attempts)),
    login_retry_delay_seconds: Math.max(1, Number(proxy?.login_retry_delay_seconds || emptyNetworkProxy.login_retry_delay_seconds)),
  };
}

function passwordState(account) {
  return String(account?.password || "").trim() ? "已填写" : "未填写";
}

export function AccountsPage() {
  const meta = pageMeta.accounts;
  const [accounts, setAccounts] = useState([]);
  const [summary, setSummary] = useState(null);
  const [selectedID, setSelectedID] = useState(0);
  const [draft, setDraft] = useState(emptyAccount);
  const [networkProxy, setNetworkProxy] = useState(emptyNetworkProxy);
  const [networkProxyDraft, setNetworkProxyDraft] = useState(emptyNetworkProxy);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [savingProxy, setSavingProxy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");

  const selectedAccount = useMemo(
    () => accounts.find((item) => item.id === selectedID) || null,
    [accounts, selectedID],
  );
  const filteredAccounts = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) {
      return accounts;
    }
    return accounts.filter((item) => {
      const haystack = [
        item.name,
        item.username,
        item.runtime?.login_status,
        item.runtime?.cycle_status,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      return haystack.includes(keyword);
    });
  }, [accounts, query]);
  const selectedStatus = useMemo(() => (selectedAccount ? accountStatus(selectedAccount) : null), [selectedAccount]);
  const draftChanged = useMemo(() => {
    const base = selectedAccount ? editableAccount(selectedAccount) : editableAccount(emptyAccount);
    return JSON.stringify(editableAccount(draft)) !== JSON.stringify(base);
  }, [draft, selectedAccount]);
  const networkProxyChanged = useMemo(() => (
    JSON.stringify(normalizeProxyDraft(networkProxyDraft)) !== JSON.stringify(normalizeProxyDraft(networkProxy))
  ), [networkProxy, networkProxyDraft]);

  useEffect(() => {
    reload();
  }, []);

  useEffect(() => {
    setDraft(selectedAccount ? editableAccount(selectedAccount) : {...emptyAccount});
  }, [selectedAccount]);

  useEffect(() => {
    if (!selectedID) {
      return;
    }
    if (filteredAccounts.some((item) => item.id === selectedID)) {
      return;
    }
    setSelectedID(filteredAccounts[0]?.id || 0);
  }, [filteredAccounts, selectedID]);

  async function reload() {
    setLoading(true);
    setError("");
    try {
      const payload = await Accounts();
      const proxyPayload = await NetworkProxy();
      const nextAccounts = payload?.accounts || [];
      setAccounts(nextAccounts);
      setSummary(payload?.summary || null);
      const nextProxy = normalizeProxyDraft(proxyPayload || emptyNetworkProxy);
      setNetworkProxy(nextProxy);
      setNetworkProxyDraft(nextProxy);
      setSelectedID((current) => {
        if (current && nextAccounts.some((item) => item.id === current)) {
          return current;
        }
        return nextAccounts[0]?.id || 0;
      });
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  }

  function updateNetworkProxy(key, value) {
    setNetworkProxyDraft((current) => ({...current, [key]: value}));
  }

  async function saveNetworkProxy() {
    setSavingProxy(true);
    setMessage("");
    setError("");
    try {
      const saved = await SaveNetworkProxy(normalizeProxyDraft(networkProxyDraft));
      const nextProxy = normalizeProxyDraft(saved || emptyNetworkProxy);
      setNetworkProxy(nextProxy);
      setNetworkProxyDraft(nextProxy);
      setMessage(nextProxy.enabled ? `统一代理已保存：${nextProxy.type}://${nextProxy.host}:${nextProxy.port}` : "统一代理已关闭");
    } catch (err) {
      setError(String(err));
    } finally {
      setSavingProxy(false);
    }
  }

  function updateDraft(key, value) {
    setDraft((current) => ({...current, [key]: value}));
  }

  function createAccount() {
    setSelectedID(0);
    setDraft({...emptyAccount});
    setMessage("");
    setError("");
    setQuery("");
  }

  function resetDraft() {
    setDraft(selectedAccount ? editableAccount(selectedAccount) : {...emptyAccount});
    setMessage("");
    setError("");
  }

  async function saveAccount() {
    setSaving(true);
    setMessage("");
    setError("");
    try {
      const saved = await SaveAccount({
        ...draft,
        submit_priority: Number(draft.submit_priority || 0),
      });
      setMessage(`账号 ${saved.name} 已保存到本地 SQLite`);
      await reload();
      setSelectedID(saved.id);
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  }

  async function deleteSelected() {
    if (!selectedID) {
      return;
    }
    setSaving(true);
    setMessage("");
    setError("");
    try {
      await DeleteAccount(selectedID);
      setMessage("账号已从 SQLite 删除");
      setSelectedID(0);
      await reload();
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  }

  return (
    <PageContainer>
      <PageHeader
        eyebrow={meta.eyebrow}
        title={meta.title}
        action={
          <>
            <Button size="sm" onClick={createAccount}>
              <Plus className="h-4 w-4" />
              新增账号
            </Button>
          </>
        }
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4 2xl:grid-cols-8">
        <SummaryCard label="账号总数" value={summary?.total ?? 0} icon={UserRoundCog} />
        <SummaryCard label="已启用" value={summary?.enabled ?? 0} icon={UserCheck} />
        <SummaryCard label="已停用" value={summary?.disabled ?? 0} icon={UserX} />
        <SummaryCard label="已连通" value={summary?.login_ok ?? 0} icon={ShieldCheck} />
        <SummaryCard label="会话可用" value={summary?.session_ready ?? 0} icon={KeyRound} />
        <SummaryCard label="缺少密码" value={summary?.missing_password ?? 0} icon={Activity} />
        <SummaryCard label="运行态已迁移" value={summary?.runtime_available ?? 0} icon={Database} />
      </div>

      {(message || error) ? (
        <div className={`rounded-md border px-4 py-3 text-sm ${error ? "border-destructive/40 bg-destructive/10 text-destructive" : "border-primary/30 bg-primary/10 text-primary"}`}>
          {error || message}
        </div>
      ) : null}

      <Card>
        <CardHeader className="border-b border-border/70 bg-gradient-to-r from-cyan-50 to-white">
          <div className="flex flex-col gap-2 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <CardTitle>统一代理配置</CardTitle>
              <CardDescription className="mt-1">
                任务管理、同步资产、状态扫描、解题流程和手动提交统一使用这里的代理；不再按账号分别启动代理。
              </CardDescription>
            </div>
            <Badge variant={networkProxyDraft.enabled ? "outline" : "secondary"}>
              {networkProxyDraft.enabled ? `${networkProxyDraft.type || "proxy"}://${networkProxyDraft.host || "-"}:${networkProxyDraft.port || "-"}` : "直连"}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4 p-4">
          <label className="flex items-center gap-3 rounded-lg border border-border bg-muted/20 px-3 py-2.5 text-sm">
            <input
              type="checkbox"
              checked={Boolean(networkProxyDraft.enabled)}
              onChange={(event) => updateNetworkProxy("enabled", event.target.checked)}
            />
            <span>启用全账号统一代理</span>
          </label>
          <div className="grid gap-3 md:grid-cols-3">
            <Field label="代理类型">
              <select
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                value={networkProxyDraft.type || ""}
                onChange={(event) => updateNetworkProxy("type", event.target.value)}
              >
                {proxyTypes.filter((item) => item.value !== "script").map((item) => <option value={item.value} key={item.value}>{item.label}</option>)}
              </select>
            </Field>
            <Field label="代理主机">
              <Input className="h-9" value={networkProxyDraft.host || ""} onChange={(event) => updateNetworkProxy("host", event.target.value)} placeholder="127.0.0.1" />
            </Field>
            <Field label="代理端口">
              <Input className="h-9" type="number" value={networkProxyDraft.port || 0} onChange={(event) => updateNetworkProxy("port", Number(event.target.value || 0))} placeholder="7890" />
            </Field>
            <Field label="代理用户名">
              <Input className="h-9" value={networkProxyDraft.username || ""} onChange={(event) => updateNetworkProxy("username", event.target.value)} />
            </Field>
            <Field label="代理密码">
              <Input className="h-9" type="password" value={networkProxyDraft.password || ""} onChange={(event) => updateNetworkProxy("password", event.target.value)} />
            </Field>
            <Field label="登录重试次数">
              <Input
                className="h-9"
                type="number"
                min={1}
                max={20}
                value={networkProxyDraft.login_attempts || emptyNetworkProxy.login_attempts}
                onChange={(event) => updateNetworkProxy("login_attempts", Number(event.target.value || emptyNetworkProxy.login_attempts))}
                placeholder="6"
              />
            </Field>
            <Field label="等待倍率(秒)">
              <Input
                className="h-9"
                type="number"
                min={1}
                max={60}
                value={networkProxyDraft.login_retry_delay_seconds || emptyNetworkProxy.login_retry_delay_seconds}
                onChange={(event) => updateNetworkProxy("login_retry_delay_seconds", Number(event.target.value || emptyNetworkProxy.login_retry_delay_seconds))}
                placeholder="3"
              />
            </Field>
          </div>
          <div className="flex flex-wrap justify-end gap-3">
            <Button size="sm" variant="outline" onClick={() => setNetworkProxyDraft(networkProxy)} disabled={savingProxy || !networkProxyChanged}>
              还原统一代理
            </Button>
            <Button size="sm" onClick={saveNetworkProxy} disabled={savingProxy || !networkProxyChanged}>
              <Save className="h-4 w-4" />
              {savingProxy ? "保存中..." : "保存统一代理"}
            </Button>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[320px_minmax(0,1fr)] 2xl:grid-cols-[360px_minmax(0,1fr)]">
        <Card className="overflow-hidden xl:sticky xl:top-8 xl:h-fit">
          <CardHeader className="border-b border-border/70 bg-gradient-to-r from-slate-100 to-white">
            <div className="flex items-start justify-between gap-3">
              <div>
                <CardTitle>账号列表</CardTitle>
              </div>
              <Badge variant="secondary">{filteredAccounts.length}/{accounts.length}</Badge>
            </div>
            <div className="relative mt-3">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="搜索账号名、用户名、状态"
                className="pl-9"
              />
            </div>
            <div className="text-xs text-muted-foreground">
              {summary?.database_path ? `本地库：${summary.database_path}` : "正在读取本地 SQLite"}
            </div>
          </CardHeader>
          <CardContent className="max-h-[calc(100vh-18rem)] space-y-3 overflow-y-auto p-4">
            {loading ? <div className="text-sm text-muted-foreground">加载账号中...</div> : null}
            {!loading && accounts.length === 0 ? (
              <div className="rounded-md border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
                还没有迁移到新系统的账号，可以先导入旧库或 YAML。
              </div>
            ) : null}
            {!loading && accounts.length > 0 && filteredAccounts.length === 0 ? (
              <div className="rounded-md border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
                当前筛选条件下没有匹配账号。
              </div>
            ) : null}
            {filteredAccounts.map((account) => {
              const status = accountStatus(account);
              return (
                <button
                  key={account.id}
                  className={cn(
                    "w-full rounded-xl border px-3 py-3 text-left transition-all",
                    selectedID === account.id
                      ? "border-primary bg-primary/10 shadow-sm ring-1 ring-primary/20"
                      : "border-border bg-background hover:border-slate-300 hover:bg-accent",
                  )}
                  onClick={() => setSelectedID(account.id)}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold leading-5">{account.name}</div>
                      <div className="mt-0.5 truncate text-xs text-muted-foreground">{account.username || "未填写用户名"}</div>
                    </div>
                    <Badge variant={status.variant}>{status.label}</Badge>
                  </div>
                  <div className="mt-3 grid gap-1 text-[11px] leading-5 text-muted-foreground">
                    <span>优先级 {account.submit_priority}</span>
                    <span>密码 {passwordState(account)}</span>
                    <span>{account.updated_at ? `更新于 ${account.updated_at}` : "本地已保存"}</span>
                  </div>
                </button>
              );
            })}
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card className="overflow-hidden">
            <CardHeader className="border-b border-border/70 bg-[radial-gradient(circle_at_top_left,_rgba(2,132,199,0.12),_transparent_38%),linear-gradient(180deg,_rgba(248,250,252,1),_rgba(241,245,249,0.9))]">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle>{draft.id ? (draft.name || "未命名账号") : "新增账号草稿"}</CardTitle>
                    {selectedStatus ? <Badge variant={selectedStatus.variant}>{selectedStatus.label}</Badge> : <Badge variant="outline">未保存</Badge>}
                    {draft.enabled ? <Badge variant="outline">启用中</Badge> : <Badge variant="secondary">已停用</Badge>}
                    {draftChanged ? <Badge variant="secondary">有未保存修改</Badge> : null}
                  </div>
                  <CardDescription className="mt-2">
                    {draft.id
                      ? ""
                      : ""}
                  </CardDescription>
                </div>
                {draft.id ? (
                  <Button variant="destructive" size="sm" onClick={deleteSelected} disabled={saving}>
                    <Trash2 className="h-4 w-4" />
                    删除
                  </Button>
                ) : null}
              </div>
            </CardHeader>
            <CardContent className="p-4">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <CompactMeta label="本地 ID" value={draft.id ? String(draft.id) : "新账号"} />
                <CompactMeta label="ISCC 用户名" value={draft.username || "未填写"} />
                <CompactMeta label="提交优先级" value={String(draft.submit_priority ?? 0)} />
                <CompactMeta label="密码状态" value={passwordState(draft)} />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{draft.id ? "账号详情" : "新建账号"}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-3 md:grid-cols-2">
                <Field label="账号名称">
                  <Input className="h-9" value={fieldValue(draft, "name")} onChange={(event) => updateDraft("name", event.target.value)} />
                </Field>
                <Field label="ISCC 用户名">
                  <Input className="h-9" value={fieldValue(draft, "username")} onChange={(event) => updateDraft("username", event.target.value)} />
                </Field>
                <Field label="密码">
                  <Input className="h-9" type="password" value={fieldValue(draft, "password")} onChange={(event) => updateDraft("password", event.target.value)} />
                </Field>
                <Field label="提交优先级">
                  <Input className="h-9" type="number" value={fieldValue(draft, "submit_priority")} onChange={(event) => updateDraft("submit_priority", event.target.value)} />
                </Field>
              </div>

              <label className="flex items-center gap-3 rounded-lg border border-border bg-muted/20 px-3 py-2.5 text-sm">
                <input
                  type="checkbox"
                  checked={Boolean(draft.enabled)}
                  onChange={(event) => updateDraft("enabled", event.target.checked)}
                />
                <span>启用账号</span>
              </label>

              <div className="flex flex-wrap justify-end gap-3">
                <Button size="sm" variant="outline" onClick={resetDraft} disabled={saving || !draftChanged}>
                  还原修改
                </Button>
                <Button size="sm" onClick={saveAccount} disabled={saving || !draftChanged}>
                  <Save className="h-4 w-4" />
                  {saving ? "保存中..." : "保存到 SQLite"}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </PageContainer>
  );
}

function SummaryCard({label, value, icon: Icon}) {
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between space-y-0">
        <div>
          <CardDescription>{label}</CardDescription>
          <CardTitle className="mt-2 text-2xl">{value}</CardTitle>
        </div>
        <div className="rounded-md bg-secondary p-2 text-secondary-foreground">
          <Icon className="h-4 w-4" />
        </div>
      </CardHeader>
    </Card>
  );
}

function CompactMeta({label, value}) {
  return (
    <div className="inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-background/85 px-3 py-1.5 text-xs leading-5">
      <span className="shrink-0 font-semibold text-muted-foreground">{label}</span>
      <span className="break-all font-medium text-foreground">{value}</span>
    </div>
  );
}

function Field({label, children}) {
  return (
    <label className="space-y-1.5 text-sm">
      <span className="block text-[13px] font-medium">{label}</span>
      {children}
    </label>
  );
}
