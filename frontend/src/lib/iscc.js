import {
  Activity,
  BarChart3,
  BookOpenCheck,
  ClipboardList,
  FileText,
  FlaskConical,
  LayoutDashboard,
  ListChecks,
  ScrollText,
  Shield,
  Swords,
  TerminalSquare,
  UserRoundCog,
} from "lucide-react";

export const starterCode = `import json
import platform
import sys
from pathlib import Path

def load_context():
    path = Path("context/challenge.json")
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        return {"error": str(exc)}

context = load_context()
raw_payload = context.get("payload")

payload = {
    "python": sys.version.split()[0],
    "platform": platform.platform(),
    "cwd": str(Path.cwd()),
    "message": "iscc sandbox ready",
    "scope": context.get("scope"),
    "context_name": context.get("context_name"),
    "payload_type": type(raw_payload).__name__,
}

if isinstance(raw_payload, dict):
    payload["context_fields"] = sorted(raw_payload.keys())

print(json.dumps(payload, ensure_ascii=False, indent=2))
`;

export const navItems = [
  {key: "dashboard", label: "仪表盘", path: "/", icon: LayoutDashboard, desc: "总览与提交状态"},
  {key: "accounts", label: "账号管理", path: "/accounts", icon: UserRoundCog, desc: "账号、Cookie、WP资料"},
  {key: "tasks", label: "任务管理", path: "/tasks", icon: ClipboardList, desc: "队列、执行、调度"},
  {key: "writeups", label: "WP 管理", path: "/writeups", icon: FileText, desc: "远端提交监控、缺交清单"},
  {key: "practice", label: "练武题", path: "/practice", icon: FlaskConical, desc: "练武赛道与待完成题"},
  {key: "arena", label: "擂台题", path: "/arena", icon: Swords, desc: "擂台提交与战况"},
  {
    key: "theory",
    label: "理论题",
    path: "/theory",
    icon: BookOpenCheck,
    desc: "理论题进度追踪",
    children: [
      {key: "theory-automation", label: "自动答题与进度", path: "/theory/automation"},
      {key: "theory-bank", label: "题库管理", path: "/theory/bank"},
      {key: "theory-ai", label: "AI 设置", path: "/theory/ai"},
    ],
  },
  {key: "combat", label: "实战题", path: "/combat", icon: Shield, desc: "实战题执行与产物"},
  {key: "logs", label: "日志管理", path: "/logs", icon: ScrollText, desc: "日志、运行轨迹、审计"},
];

export const pageMeta = {
  dashboard: {
    eyebrow: "Overview",
    title: "ISCC 控制总览",
  },
  accounts: {
    eyebrow: "Accounts",
    title: "账号管理",
  },
  tasks: {
    eyebrow: "Tasks",
    title: "任务管理",
  },
  writeups: {
    eyebrow: "Writeups",
    title: "WP 管理",
  },
  practice: {
    eyebrow: "Practice",
    title: "练武题",
  },
  arena: {
    eyebrow: "Arena",
    title: "擂台题",
  },
  theory: {
    eyebrow: "Theory",
    title: "理论题",
  },
  theoryAutomation: {
    eyebrow: "Theory / Automation",
    title: "理论题 / 自动答题与进度",
  },
  theoryBank: {
    eyebrow: "Theory / Bank",
    title: "理论题 / 题库管理",
  },
  theoryAI: {
    eyebrow: "Theory / AI",
    title: "理论题 / AI 设置",
  },
  combat: {
    eyebrow: "Combat",
    title: "实战题",
  },
  logs: {
    eyebrow: "Logs",
    title: "日志管理",
  },
};

export const dashboardStats = [
  {label: "当前账号提交", value: "运行态已接", note: "账号运行态、远端提交数和会话状态已迁移到 SQLite", icon: ListChecks},
  {label: "WP 提交情况", value: "-", note: "-", icon: FileText},
  {label: "还缺少的题", value: "-", note: "-", icon: BarChart3},
  {label: "沙盒执行器", value: "Ready", note: "Python 代码沙盒已纳入新版架构", icon: TerminalSquare},
];

export const dashboardRows = [
  {track: "练武题", total: "-", submitted: "-", wp: "-", missing: "-", status: "planned"},
  {track: "擂台题", total: "-", submitted: "-", wp: "-", missing: "-", status: "planned"},
  {track: "理论题", total: "-", submitted: "-", wp: "-", missing: "-", status: "planned"},
  {track: "实战题", total: "-", submitted: "-", wp: "-", missing: "-", status: "planned"},
];

export const moduleFeatures = [
  "查看当前 ISCC 账号的题目提交状态",
  "查看当前账号的 WP 提交情况",
  "识别还缺少哪些题、哪些 WP 没交",
  "按练武题 / 擂台题 / 理论题 / 实战题分栏管理",
];

export const activityItems = [
  {icon: Activity, title: "系统运行", value: "Wails dev ready"},
  {icon: TerminalSquare, title: "Python 沙盒", value: "local-isolated"},
  {icon: FileText, title: "WP 管理", value: "本地规范检查 + 上传入口"},
];
