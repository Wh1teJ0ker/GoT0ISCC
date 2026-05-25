#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import sys
import time
import zipfile
from dataclasses import dataclass
from datetime import datetime
from html import unescape
from pathlib import Path
from typing import Any

import requests
import yaml


APP_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CONFIG_PATH = APP_ROOT / "data" / "theory-bank-normalizer.yaml"

QUESTION_START_RE = re.compile(r"^第\s*\d+\s*题")
QUESTION_PREFIX_RE = re.compile(r"^第\s*(\d+)\s*题")
EMBEDDED_ANSWER_RE = re.compile(r"_{2,}\s*([A-D]{1,4})\s*_{2,}")
TRAILING_ANSWER_RE = re.compile(r"([A-D]{1,4})\s*[。．.]?\s*$")
OPTION_MARKER_RE = re.compile(r"[A-D][\.．、]")
DOCX_TAG_RE = re.compile(r"<[^>]+>")
DOCX_PARAGRAPH_RE = re.compile(r"</w:p>")


@dataclass
class SourceEntry:
    source_id: str
    source_path: str
    question: str
    options: list[dict[str, Any]]
    answer_keys_hint: list[str]
    question_type_hint: str
    raw_text: str


def main() -> int:
    parser = argparse.ArgumentParser(description="Normalize theory bank entries with a local AI gateway.")
    parser.add_argument("--config", default=str(DEFAULT_CONFIG_PATH), help="YAML config path")
    parser.add_argument("--limit", type=int, default=0, help="Only process the first N entries")
    parser.add_argument("--batch-size", type=int, default=0, help="Override batch size from config")
    parser.add_argument("--overwrite", action="store_true", help="Ignore previous normalized output and rebuild")
    args = parser.parse_args()

    config_path = Path(args.config).expanduser().resolve()
    config = load_yaml(config_path)

    source_paths = [resolve_path(config_path, item) for item in config.get("sources", [])]
    if not source_paths:
        raise SystemExit("No sources configured")

    output_cfg = config.get("output", {})
    output_path = resolve_path(config_path, output_cfg.get("path", "./data/清洗后的题库.standardized.json"))
    meta_path = resolve_path(config_path, output_cfg.get("meta_path", "./data/清洗后的题库.standardized.meta.json"))

    provider = config.get("provider", {})
    batch_size = args.batch_size or int(config.get("normalizer", {}).get("batch_size", 6))
    timeout_seconds = int(provider.get("timeout_seconds", 120))

    entries = load_entries(source_paths)
    if args.limit > 0:
        entries = entries[: args.limit]

    existing = {} if args.overwrite else load_existing(output_path)
    pending = [entry for entry in entries if entry.source_id not in existing]

    print(f"[bank-normalizer] sources={len(source_paths)} total={len(entries)} pending={len(pending)}")

    for index in range(0, len(pending), batch_size):
        batch = pending[index : index + batch_size]
        print(f"[bank-normalizer] batch {index // batch_size + 1} size={len(batch)}")
        standardized = request_standardized_batch(provider, batch, timeout_seconds=timeout_seconds)
        for item in standardized:
            existing[item["source_id"]] = item
        write_outputs(output_path, meta_path, source_paths, entries, existing, provider)

    if not pending:
        write_outputs(output_path, meta_path, source_paths, entries, existing, provider)

    review_count = sum(1 for item in existing.values() if item.get("needs_review"))
    print(f"[bank-normalizer] done normalized={len(existing)} review={review_count}")
    print(f"[bank-normalizer] output={output_path}")
    print(f"[bank-normalizer] meta={meta_path}")
    return 0


def load_yaml(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise SystemExit(f"Config not found: {path}")
    return yaml.safe_load(path.read_text(encoding="utf-8")) or {}


def resolve_path(config_path: Path, value: str) -> Path:
    candidate = Path(value).expanduser()
    if candidate.is_absolute():
        return candidate
    return (config_path.parent.parent / candidate).resolve()


def load_existing(path: Path) -> dict[str, dict[str, Any]]:
    if not path.exists():
        return {}
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, list):
        return {}
    result: dict[str, dict[str, Any]] = {}
    for item in payload:
        source_id = str(item.get("source_id", "")).strip()
        if source_id:
            result[source_id] = item
    return result


def load_entries(paths: list[Path]) -> list[SourceEntry]:
    entries: list[SourceEntry] = []
    for path in paths:
        suffix = path.suffix.lower()
        if suffix == ".json":
            entries.extend(load_json_entries(path))
        elif suffix == ".docx":
            entries.extend(load_docx_entries(path))
    return entries


def load_json_entries(path: Path) -> list[SourceEntry]:
    raw_items = json.loads(path.read_text(encoding="utf-8"))
    entries: list[SourceEntry] = []
    for index, item in enumerate(raw_items, start=1):
        raw_options = item.get("options", []) or []
        options = []
        answer_keys = []
        for option_index, raw_option in enumerate(raw_options):
            key = chr(ord("A") + option_index)
            content = clean_text(str(raw_option.get("content", "")))
            is_correct = bool(raw_option.get("isCorrect", False))
            options.append({"key": key, "content": content, "isCorrect": is_correct})
            if is_correct:
                answer_keys.append(key)
        entries.append(
            SourceEntry(
                source_id=f"json:{index:04d}",
                source_path=str(path),
                question=clean_text(str(item.get("question", ""))),
                options=options,
                answer_keys_hint=answer_keys,
                question_type_hint="multiple" if len(answer_keys) > 1 else "single",
                raw_text=json.dumps(item, ensure_ascii=False),
            )
        )
    return entries


def load_docx_entries(path: Path) -> list[SourceEntry]:
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

    entries: list[SourceEntry] = []
    for index, block in enumerate(blocks, start=1):
        parsed = parse_docx_block(block)
        if parsed is None:
            continue
        question, options, answer_keys, raw_text = parsed
        entries.append(
            SourceEntry(
                source_id=f"docx:{index:04d}",
                source_path=str(path),
                question=question,
                options=options,
                answer_keys_hint=answer_keys,
                question_type_hint="multiple" if len(answer_keys) > 1 else ("single" if answer_keys else "unknown"),
                raw_text=raw_text,
            )
        )
    return entries


def parse_docx_block(lines: list[str]) -> tuple[str, list[dict[str, Any]], list[str], str] | None:
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
    for idx, match in enumerate(markers):
        key = match.group(0)[0]
        start = match.end()
        end = markers[idx + 1].start() if idx + 1 < len(markers) else len(options_part)
        content = clean_text(options_part[start:end])
        if not content:
            continue
        options.append({"key": key, "content": content, "isCorrect": key in answer_keys})

    if len(options) < 2 or not question_part:
        return None
    return question_part, options, answer_keys, joined


def normalize_docx_line(line: str) -> str:
    return clean_text(line.replace("　", " "))


def clean_text(value: str) -> str:
    return " ".join(str(value).strip().split())


def request_standardized_batch(provider: dict[str, Any], batch: list[SourceEntry], timeout_seconds: int) -> list[dict[str, Any]]:
    base_url = str(provider.get("base_url", "")).strip()
    api_key = str(provider.get("api_key", "")).strip()
    model = str(provider.get("model", "gpt-5.4")).strip() or "gpt-5.4"
    reasoning_effort = str(provider.get("reasoning_effort", "medium")).strip() or "medium"

    if not base_url or not api_key:
        raise SystemExit("provider.base_url / provider.api_key is required")

    payload = {
        "items": [
            {
                "source_id": entry.source_id,
                "source_path": entry.source_path,
                "question": entry.question,
                "options": [{"key": item["key"], "content": item["content"]} for item in entry.options],
                "answer_keys_hint": entry.answer_keys_hint,
                "question_type_hint": entry.question_type_hint,
                "raw_text": entry.raw_text,
            }
            for entry in batch
        ]
    }

    system_prompt = (
        "You standardize ISCC theory question-bank entries. "
        "Return clean JSON only. Keep the meaning of the question and options. "
        "Do not invent answers. If the source answer is missing or ambiguous, leave answer_keys empty and set needs_review=true. "
        "Preserve option order and count. Remove obvious noise such as duplicated numbering, footers, stray hyperlinks, and OCR residue."
    )

    schema = {
        "type": "object",
        "properties": {
            "items": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "source_id": {"type": "string"},
                        "question": {"type": "string"},
                        "question_type": {"type": "string", "enum": ["single", "multiple", "unknown"]},
                        "options": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {
                                    "key": {"type": "string"},
                                    "content": {"type": "string"},
                                },
                                "required": ["key", "content"],
                                "additionalProperties": False,
                            },
                        },
                        "answer_keys": {"type": "array", "items": {"type": "string"}},
                        "needs_review": {"type": "boolean"},
                        "review_reason": {"type": "string"},
                        "confidence": {"type": "number"},
                    },
                    "required": [
                        "source_id",
                        "question",
                        "question_type",
                        "options",
                        "answer_keys",
                        "needs_review",
                        "review_reason",
                        "confidence",
                    ],
                    "additionalProperties": False,
                },
            }
        },
        "required": ["items"],
        "additionalProperties": False,
    }

    raw_result = call_ai(
        base_url=base_url,
        api_key=api_key,
        model=model,
        reasoning_effort=reasoning_effort,
        system_prompt=system_prompt,
        user_payload=payload,
        schema=schema,
        timeout_seconds=timeout_seconds,
    )
    normalized_items = json.loads(raw_result).get("items", [])
    batch_map = {entry.source_id: entry for entry in batch}
    result = []
    for item in normalized_items:
        source_id = str(item.get("source_id", "")).strip()
        entry = batch_map.get(source_id)
        if not entry:
            continue
        result.append(finalize_item(entry, item))

    missing = [entry for entry in batch if entry.source_id not in {item["source_id"] for item in result}]
    for entry in missing:
        result.append(
            finalize_item(
                entry,
                {
                    "source_id": entry.source_id,
                    "question": entry.question,
                    "question_type": entry.question_type_hint,
                    "options": [{"key": item["key"], "content": item["content"]} for item in entry.options],
                    "answer_keys": entry.answer_keys_hint,
                    "needs_review": True,
                    "review_reason": "AI response missing this entry",
                    "confidence": 0.0,
                },
            )
        )
    result.sort(key=lambda item: item["source_id"])
    return result


def call_ai(
    *,
    base_url: str,
    api_key: str,
    model: str,
    reasoning_effort: str,
    system_prompt: str,
    user_payload: dict[str, Any],
    schema: dict[str, Any],
    timeout_seconds: int,
) -> str:
    session = requests.Session()
    endpoints = build_endpoint_candidates(base_url)
    errors: list[str] = []

    for endpoint_type, url in endpoints:
        try:
            if endpoint_type == "responses":
                response = session.post(
                    url,
                    timeout=timeout_seconds,
                    headers={
                        "Authorization": f"Bearer {api_key}",
                        "Content-Type": "application/json",
                    },
                    json={
                        "model": model,
                        "reasoning": {"effort": reasoning_effort},
                        "input": [
                            {
                                "role": "system",
                                "content": [{"type": "input_text", "text": system_prompt}],
                            },
                            {
                                "role": "user",
                                "content": [{"type": "input_text", "text": json.dumps(user_payload, ensure_ascii=False)}],
                            },
                        ],
                        "text": {
                            "format": {
                                "type": "json_schema",
                                "name": "theory_bank_batch",
                                "schema": schema,
                            }
                        },
                    },
                )
                if response.status_code >= 400:
                    raise RuntimeError(f"{response.status_code} {response.text[:300]}")
                data = response.json()
                text_parts = []
                for output in data.get("output", []):
                    for content in output.get("content", []):
                        if content.get("type") in {"output_text", "text"} and content.get("text"):
                            text_parts.append(content["text"])
                text = "\n".join(text_parts).strip()
                if text:
                    return text
                raise RuntimeError("empty responses output")

            response = session.post(
                url,
                timeout=timeout_seconds,
                headers={
                    "Authorization": f"Bearer {api_key}",
                    "Content-Type": "application/json",
                },
                json={
                    "model": model,
                    "messages": [
                        {"role": "system", "content": system_prompt},
                        {"role": "user", "content": json.dumps(user_payload, ensure_ascii=False)},
                    ],
                    "response_format": {"type": "json_object"},
                },
            )
            if response.status_code >= 400:
                raise RuntimeError(f"{response.status_code} {response.text[:300]}")
            data = response.json()
            choices = data.get("choices", [])
            if choices:
                content = choices[0].get("message", {}).get("content", "")
                if content:
                    return content.strip()
            raise RuntimeError("empty chat.completions output")
        except Exception as exc:  # noqa: BLE001
            errors.append(f"{url} -> {exc}")
            continue

    raise RuntimeError("AI endpoint failed:\n" + "\n".join(errors))


def build_endpoint_candidates(base_url: str) -> list[tuple[str, str]]:
    base = base_url.rstrip("/")
    roots = [base]
    if not base.endswith("/v1"):
        roots.insert(0, f"{base}/v1")

    seen = set()
    candidates = []
    for root in roots:
        for endpoint_type, suffix in (
            ("responses", "/responses"),
            ("chat", "/chat/completions"),
        ):
            url = f"{root}{suffix}"
            if url in seen:
                continue
            seen.add(url)
            candidates.append((endpoint_type, url))
    return candidates


def finalize_item(entry: SourceEntry, ai_item: dict[str, Any]) -> dict[str, Any]:
    ai_options = ai_item.get("options", []) or []
    ai_option_map = {str(item.get("key", "")).strip(): clean_text(str(item.get("content", ""))) for item in ai_options}
    answer_keys = [str(item).strip() for item in (ai_item.get("answer_keys", []) or []) if str(item).strip()]
    if not answer_keys:
        answer_keys = list(entry.answer_keys_hint)

    final_options = []
    for source_option in entry.options:
        key = str(source_option.get("key", "")).strip()
        content = ai_option_map.get(key) or clean_text(str(source_option.get("content", "")))
        final_options.append(
            {
                "content": content,
                "isCorrect": key in answer_keys,
            }
        )

    question = clean_text(str(ai_item.get("question", ""))) or entry.question
    question_type = str(ai_item.get("question_type", "")).strip() or entry.question_type_hint or "unknown"
    needs_review = bool(ai_item.get("needs_review", False))
    review_reason = clean_text(str(ai_item.get("review_reason", "")))
    confidence = ai_item.get("confidence", 0)
    try:
        confidence = float(confidence)
    except Exception:  # noqa: BLE001
        confidence = 0.0
    confidence = max(0.0, min(1.0, confidence))

    if len(final_options) != len(entry.options):
        needs_review = True
        review_reason = review_reason or "AI changed option count"
        final_options = [{"content": item["content"], "isCorrect": item["key"] in answer_keys} for item in entry.options]

    if not answer_keys:
        needs_review = True
        review_reason = review_reason or "No reliable answer extracted"

    if question_type not in {"single", "multiple", "unknown"}:
        question_type = "unknown"
    if question_type == "unknown":
        if len(answer_keys) == 1:
            question_type = "single"
        elif len(answer_keys) > 1:
            question_type = "multiple"

    answer_texts = []
    for index, option in enumerate(final_options):
        key = chr(ord("A") + index)
        if key in answer_keys:
            answer_texts.append(option["content"])

    return {
        "source_id": entry.source_id,
        "source_path": entry.source_path,
        "question": question,
        "options": final_options,
        "question_type": question_type,
        "answer_keys": answer_keys,
        "answer_texts": answer_texts,
        "needs_review": needs_review,
        "review_reason": review_reason,
        "confidence": confidence,
        "raw_text": entry.raw_text,
    }


def write_outputs(
    output_path: Path,
    meta_path: Path,
    source_paths: list[Path],
    all_entries: list[SourceEntry],
    normalized: dict[str, dict[str, Any]],
    provider: dict[str, Any],
) -> None:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    meta_path.parent.mkdir(parents=True, exist_ok=True)

    ordered = []
    for entry in all_entries:
        item = normalized.get(entry.source_id)
        if item:
            ordered.append(item)

    output_path.write_text(json.dumps(ordered, ensure_ascii=False, indent=2), encoding="utf-8")
    meta_payload = {
        "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "sources": [str(path) for path in source_paths],
        "model": str(provider.get("model", "gpt-5.4")),
        "base_url": str(provider.get("base_url", "")),
        "total_input": len(all_entries),
        "total_output": len(ordered),
        "needs_review": sum(1 for item in ordered if item.get("needs_review")),
    }
    meta_path.write_text(json.dumps(meta_payload, ensure_ascii=False, indent=2), encoding="utf-8")


if __name__ == "__main__":
    raise SystemExit(main())
