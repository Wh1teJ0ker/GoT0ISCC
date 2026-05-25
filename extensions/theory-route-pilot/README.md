# Theory Route Pilot

独立 Chrome 插件版理论题自动化。只新增插件，不改现有 Wails 软件逻辑。

## 当前行为

- 命中 `https://iscc.isclab.org.cn/choice` 或 `https://iscc.isclab.org.cn/paper` 后自动识别为理论题页面。
- 自动读取当前页面题目和选项。
- 先用本地导入题库匹配答案。
- 如果启用了 AI，则无论题库是否命中，都会再做一次 AI 复核。
- 达到阈值后自动勾选并提交。
- 按固定冷却节奏串行处理，不会无限快速连续提交。
- 页面右上角会显示实时浮层状态。
- 插件弹窗里可以看当前运行状态、导入题库、调整阈值和 AI 配置。
- 如果扩展包内存在默认题库文件，会在扩展加载时自动同步，无需手动再次导入。

## 安装方式

1. 打开 Chrome 扩展管理页 `chrome://extensions/`
2. 打开“开发者模式”
3. 选择“加载已解压的扩展程序”
4. 选中本目录：

```text
extensions/theory-route-pilot
```

## 一次性准备好

推荐流程：

1. 把理论题源文件放到以下任一约定路径，或自行准备一个源文件路径：

```text
./清洗后的题库.standardized.json
./data/清洗后的题库.standardized.json
./清洗后的题库.normalized.json
./data/清洗后的题库.normalized.json
./清洗后的题库.json
./data/清洗后的题库.json
./data/*题库*.docx
```

2. 运行一键导出脚本：

```bash
./scripts/export_theory_plugin_bank.sh
```

也可以手动指定源文件：

```bash
./scripts/export_theory_plugin_bank.sh --source "/绝对路径/你的题库文件"
```

3. 重新加载 Chrome 扩展。

脚本会直接输出到插件当前使用的规范文件：

```text
extensions/theory-route-pilot/assets/theory-bank.current.json
```

扩展重载后会自动读取这份题库，不需要再手工导入。

## 当前使用规范

插件当前读取的是下面这种结构：

```json
{
  "meta": {
    "spec_version": "theory-route-pilot.v1",
    "generated_at": "2026-05-12 20:00:00",
    "source_path": "/abs/path/bank.json",
    "source_type": "json",
    "total": 123,
    "signature": "sha1..."
  },
  "items": [
    {
      "id": "json:0001",
      "question": "题目",
      "normalized_question": "归一化后的题目",
      "compact_question": "去空格后的题目",
      "correct_options": ["B"],
      "correct_texts": ["正确选项文本"],
      "keywords": ["题目", "选项关键词"],
      "options": [
        {"key": "A", "content": "选项A"},
        {"key": "B", "content": "选项B"}
      ]
    }
  ]
}
```

## 题库格式

当前插件仍支持手动导入 JSON，格式兼容标准化后的列表：

```json
[
  {
    "question": "题目",
    "correct_options": ["B"],
    "correct_texts": ["正确选项文本"],
    "options": [
      {"key": "A", "content": "选项A"},
      {"key": "B", "content": "选项B"}
    ]
  }
]
```

## 限制

- 这是插件版 MVP，但已经支持把 `json/docx` 源文件一键导出为插件规范。
- 当前不替代登录，不管理账号，只在已登录的理论题页面工作。
