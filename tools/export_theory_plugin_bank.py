#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import sqlite3
import re
import sys
import zipfile
from dataclasses import dataclass
from html import unescape
from pathlib import Path
from typing import Any


APP_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_OUTPUT_PATH = APP_ROOT / "extensions" / "theory-route-pilot" / "assets" / "theory-bank.current.json"
DEFAULT_DB_PATH = APP_ROOT / "data" / "got0iscc.db"

QUESTION_START_RE = re.compile(r"^第\s*\d+\s*题")
QUESTION_PREFIX_RE = re.compile(r"^第\s*(\d+)\s*题")
EMBEDDED_ANSWER_RE = re.compile(r"_{2,}\s*([A-D]{1,4})\s*_{2,}")
TRAILING_ANSWER_RE = re.compile(r"([A-D]{1,4})\s*[。．.]?\s*$")
OPTION_MARKER_RE = re.compile(r"[A-D][\.．、]")
DOCX_TAG_RE = re.compile(r"<[^>]+>")
DOCX_PARAGRAPH_RE = re.compile(r"</w:p>")


@dataclass
class ExportItem:
    identifier: str
    question: str
    options: list[dict[str, str]]
    correct_options: list[str]
    correct_texts: list[str]


def main() -> int:
    parser = argparse.ArgumentParser(description="Export a theory bank into the Chrome plugin bank format.")
    parser.add_argument("--source", default="", help="Optional source file path")
    parser.add_argument("--db", default=str(DEFAULT_DB_PATH), help="SQLite database path")
    parser.add_argument("--prefer-db", action="store_true", help="Prefer exporting from SQLite theory_bank_questions")
    parser.add_argument("--output", default=str(DEFAULT_OUTPUT_PATH), help="Output JSON path")
    args = parser.parse_args()

    db_path = Path(args.db).expanduser().resolve()
    source_path: Path | None = None
    items: list[ExportItem]

    if args.prefer_db:
        items = load_db_items(db_path)
        source_path = db_path
    else:
        try:
            source_path = discover_source(Path(args.source).expanduser()) if args.source else discover_source(None)
            items = load_source_items(source_path)
        except SystemExit:
            if db_path.exists():
                items = load_db_items(db_path)
                source_path = db_path
            else:
                raise

    output_path = Path(args.output).expanduser().resolve()
    payload = {
        "meta": {
            "spec_version": "theory-route-pilot.v1",
            "generated_at": now_text(),
            "source_path": str(source_path),
            "source_type": detect_source_type(source_path),
            "total": len(items),
            "signature": build_signature(source_path, items),
        },
        "items": [build_plugin_item(item) for item in items],
    }

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")

    print(f"[theory-plugin-bank] source={source_path}")
    print(f"[theory-plugin-bank] total={len(items)}")
    print(f"[theory-plugin-bank] output={output_path}")
    return 0


def discover_source(preferred: Path | None) -> Path:
    if preferred and preferred.exists():
        return preferred.resolve()

    candidates = [
        APP_ROOT / "清洗后的题库.standardized.json",
        APP_ROOT / "data" / "清洗后的题库.standardized.json",
        APP_ROOT / "清洗后的题库.normalized.json",
        APP_ROOT / "data" / "清洗后的题库.normalized.json",
        APP_ROOT / "清洗后的题库.json",
        APP_ROOT / "data" / "清洗后的题库.json",
    ]
    for candidate in candidates:
        if candidate.exists():
            return candidate.resolve()

    docx_candidates = sorted((APP_ROOT / "data").glob("*题库*.docx")) + sorted(APP_ROOT.glob("*题库*.docx"))
    for candidate in docx_candidates:
        if candidate.exists():
            return candidate.resolve()

    joined = "\n".join(str(item) for item in candidates)
    raise SystemExit("未找到理论题源文件。请把题库放到以下约定路径之一，或使用 --source 指定：\n" + joined)


def detect_source_type(path: Path) -> str:
    if path.suffix.lower() == ".db":
        return "sqlite"
    return path.suffix.lower().lstrip(".") or "unknown"


def load_source_items(path: Path) -> list[ExportItem]:
    suffix = path.suffix.lower()
    if suffix == ".docx":
        return load_docx_items(path)
    if suffix == ".json":
        return load_json_items(path)
    raise SystemExit(f"不支持的题库类型: {path.suffix}")


def load_db_items(path: Path) -> list[ExportItem]:
    if not path.exists():
      raise SystemExit(f"数据库不存在: {path}")

    connection = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    try:
        cursor = connection.execute(
            """
            select
              id,
              question,
              options_json,
              answer_keys_json,
              answer_texts_json,
              review_status,
              needs_review
            from theory_bank_questions
            order by normalized_question asc, id asc
            """
        )
        result: list[ExportItem] = []
        for row in cursor.fetchall():
            identifier = f"sqlite:{row[0]}"
            question = clean_text(row[1])
            if not question:
                continue
            options_payload = parse_json_text(row[2], [])
            answer_keys = unique_keys(parse_json_text(row[3], []))
            answer_texts = [clean_text(text) for text in parse_json_text(row[4], []) if clean_text(text)]
            options = []
            for option_index, raw_option in enumerate(options_payload):
                key = clean_key(raw_option.get("key", "")) or chr(ord("A") + option_index)
                content = clean_text(raw_option.get("content", ""))
                if content:
                    options.append({"key": key, "content": content})
            if not options:
                continue
            if not answer_texts:
                option_map = {option["key"]: option["content"] for option in options}
                answer_texts = [option_map[key] for key in answer_keys if key in option_map]
            result.append(
                ExportItem(
                    identifier=identifier,
                    question=question,
                    options=options,
                    correct_options=answer_keys,
                    correct_texts=answer_texts,
                )
            )
        return result
    finally:
        connection.close()


def load_json_items(path: Path) -> list[ExportItem]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(payload, dict) and isinstance(payload.get("items"), list):
        items = payload["items"]
    elif isinstance(payload, list):
        items = payload
    else:
        raise SystemExit(f"JSON 结构无法识别: {path}")

    result: list[ExportItem] = []
    for index, item in enumerate(items, start=1):
        parsed = parse_json_item(item, index)
        if parsed is not None:
            result.append(parsed)
    return result


def parse_json_item(item: dict[str, Any], index: int) -> ExportItem | None:
    question = clean_text(item.get("question", ""))
    raw_options = item.get("options", []) or []
    options: list[dict[str, str]] = []
    for option_index, raw_option in enumerate(raw_options):
        key = clean_key(raw_option.get("key", "")) or chr(ord("A") + option_index)
        content = clean_text(raw_option.get("content", ""))
        if content:
            options.append({"key": key, "content": content})
    if not question or not options:
        return None

    correct_options = collect_correct_options(item, options)
    correct_texts = collect_correct_texts(item, options, correct_options)

    identifier = str(item.get("id") or item.get("source_id") or f"json:{index:04d}").strip()
    return ExportItem(
        identifier=identifier,
        question=question,
        options=options,
        correct_options=correct_options,
        correct_texts=correct_texts,
    )


def collect_correct_options(item: dict[str, Any], options: list[dict[str, str]]) -> list[str]:
    answer_keys = item.get("answer_keys") or item.get("correct_options") or item.get("answer_keys_hint") or []
    if answer_keys:
        return unique_keys(answer_keys)

    derived: list[str] = []
    for option_index, raw_option in enumerate(item.get("options", []) or []):
        is_correct = bool(raw_option.get("isCorrect", False) or raw_option.get("is_correct", False))
        if is_correct:
            key = clean_key(raw_option.get("key", "")) or chr(ord("A") + option_index)
            derived.append(key)
    if derived:
        return unique_keys(derived)

    answer_texts = item.get("answer_texts") or item.get("correct_texts") or []
    option_map = {normalize_theory_text(option["content"]): option["key"] for option in options}
    matched = []
    for answer_text in answer_texts:
        key = option_map.get(normalize_theory_text(answer_text))
        if key:
            matched.append(key)
    return unique_keys(matched)


def collect_correct_texts(item: dict[str, Any], options: list[dict[str, str]], correct_options: list[str]) -> list[str]:
    answer_texts = item.get("answer_texts") or item.get("correct_texts") or []
    texts = [clean_text(text) for text in answer_texts if clean_text(text)]
    if texts:
        return texts
    option_map = {option["key"]: option["content"] for option in options}
    return [option_map[key] for key in correct_options if key in option_map]


def load_docx_items(path: Path) -> list[ExportItem]:
    with zipfile.ZipFile(path) as archive:
        xml = archive.read("word/document.xml").decode("utf-8", "ignore")

    text = DOCX_PARAGRAPH_RE.sub("\n", xml)
    text = DOCX_TAG_RE.sub("", text)
    text = unescape(text).replace("\xa0", " ")
    lines = [normalize_docx_line(line) for line in text.splitlines()]
    lines = [line for line in lines if line and line != "窗体底端" and "HYPERLINK " not in line]

    blocks: list[list[str]] = []
    current: list[str] = []
    for line in lines:
        if QUESTION_START_RE.match(line):
            if current:
                blocks.append(current)
            current = [line]
        elif current:
            current.append(line)
    if current:
        blocks.append(current)

    result: list[ExportItem] = []
    for index, block in enumerate(blocks, start=1):
        parsed = parse_docx_block(block)
        if parsed is None:
            continue
        question, options, answer_keys = parsed
        option_map = {option["key"]: option["content"] for option in options}
        correct_texts = [option_map[key] for key in answer_keys if key in option_map]
        result.append(
            ExportItem(
                identifier=f"docx:{index:04d}",
                question=question,
                options=options,
                correct_options=answer_keys,
                correct_texts=correct_texts,
            )
        )
    return result


def parse_docx_block(lines: list[str]) -> tuple[str, list[dict[str, str]], list[str]] | None:
    joined = clean_text(" ".join(lines).replace("窗体底端", " "))
    if not joined:
        return None

    question_text = QUESTION_PREFIX_RE.sub("", joined, count=1).strip()
    option_start = -1
    for marker in ("A.", "A．", "A、"):
        option_start = question_text.find(marker)
        if option_start >= 0:
            break
    if option_start < 0:
        return None

    question_part = question_text[:option_start].strip()
    options_part = question_text[option_start:].strip()
    answer_keys: list[str] = []

    embedded_match = EMBEDDED_ANSWER_RE.search(question_part)
    if embedded_match:
        answer_keys = list(embedded_match.group(1).strip())
        question_part = EMBEDDED_ANSWER_RE.sub(" ", question_part).strip()
    else:
        trailing_match = TRAILING_ANSWER_RE.search(question_part)
        if trailing_match:
            answer_candidate = trailing_match.group(1).strip()
            if answer_candidate:
                answer_keys = list(answer_candidate)
                question_part = TRAILING_ANSWER_RE.sub(" ", question_part).strip()

    options = []
    markers = list(OPTION_MARKER_RE.finditer(options_part))
    for index, match in enumerate(markers):
        key = match.group(0)[0]
        start = match.end()
        end = markers[index + 1].start() if index + 1 < len(markers) else len(options_part)
        content = clean_text(options_part[start:end])
        if content:
            options.append({"key": key, "content": content})

    if len(options) < 2 or not question_part:
        return None
    return question_part, options, unique_keys(answer_keys)


def build_plugin_item(item: ExportItem) -> dict[str, Any]:
    normalized_question = normalize_theory_text(item.question)
    compact_question = compact_theory_text(item.question)
    keywords = unique_strings(
        [item.question, normalized_question]
        + [option["content"] for option in item.options]
        + item.correct_texts
    )
    return {
        "id": item.identifier,
        "question": item.question,
        "normalized_question": normalized_question,
        "compact_question": compact_question,
        "correct_options": item.correct_options,
        "correct_texts": item.correct_texts,
        "keywords": keywords,
        "options": item.options,
    }


def build_signature(path: Path, items: list[ExportItem]) -> str:
    digest = hashlib.sha1()
    digest.update(str(path.resolve()).encode("utf-8"))
    if path.exists():
        digest.update(str(path.stat().st_mtime_ns).encode("utf-8"))
        digest.update(str(path.stat().st_size).encode("utf-8"))
    digest.update(str(len(items)).encode("utf-8"))
    return digest.hexdigest()


def parse_json_text(value: Any, fallback: Any) -> Any:
    text = str(value or "").strip()
    if not text:
        return fallback
    try:
        return json.loads(text)
    except Exception:
        return fallback


def now_text() -> str:
    from datetime import datetime

    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def clean_key(value: Any) -> str:
    text = str(value or "").strip().upper()
    return text[:1] if text[:1] in {"A", "B", "C", "D", "E", "F"} else ""


def unique_keys(values: list[Any]) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for value in values:
        key = clean_key(value)
        if key and key not in seen:
            seen.add(key)
            result.append(key)
    return result


def normalize_docx_line(line: str) -> str:
    return clean_text(str(line).replace("　", " "))


def clean_text(value: Any) -> str:
    return " ".join(str(value or "").strip().split())


def normalize_theory_text(value: Any) -> str:
    text = str(value or "").strip().lower()
    text = to_half_width(text)
    text = text.replace("\n", " ").replace("\r", " ")
    text = re.sub(r"[（）（）().,，。；;：:？！?!、“”\"'‘’【】\[\]《》、]", " ", text)
    text = re.sub(r"第\s*1\s*题", " ", text)
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def compact_theory_text(value: Any) -> str:
    return normalize_theory_text(value).replace(" ", "")


def to_half_width(value: str) -> str:
    chars: list[str] = []
    for char in value:
        code = ord(char)
        if code == 12288:
            chars.append(" ")
        elif 65281 <= code <= 65374:
            chars.append(chr(code - 65248))
        else:
            chars.append(char)
    return "".join(chars)


def unique_strings(values: list[Any]) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for value in values:
        normalized = normalize_theory_text(value)
        if normalized and normalized not in seen:
            seen.add(normalized)
            result.append(normalized)
    return result


if __name__ == "__main__":
    sys.exit(main())
