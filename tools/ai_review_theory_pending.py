#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sqlite3
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from normalize_theory_bank import call_ai, clean_text


APP_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DB_PATH = APP_ROOT / "data" / "got0iscc.db"
DEFAULT_BASE_URL = "http://localhost:18080"
DEFAULT_MODEL = "gpt-5.4"
DEFAULT_API_KEY = "sk-0845abcc8e9498836649caf90fccac8638366877050fca84a079883707eed4e5"


@dataclass
class ReviewRow:
    id: int
    question: str
    selection_type: str
    options: list[dict[str, Any]]
    answer_keys: list[str]
    answer_texts: list[str]
    review_status: str
    review_reason: str
    confidence: float
    question_hash: str


def main() -> int:
    parser = argparse.ArgumentParser(description="Run AI review for pending theory questions in SQLite.")
    parser.add_argument("--db", default=str(DEFAULT_DB_PATH), help="SQLite path")
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL, help="AI gateway base URL")
    parser.add_argument("--api-key", default=DEFAULT_API_KEY, help="AI gateway API key")
    parser.add_argument("--model", default=DEFAULT_MODEL, help="AI model")
    parser.add_argument("--reasoning-effort", default="high", help="AI reasoning effort")
    parser.add_argument("--batch-size", type=int, default=12, help="Batch size")
    parser.add_argument("--limit", type=int, default=0, help="Only review first N records")
    parser.add_argument("--timeout", type=int, default=180, help="HTTP timeout seconds")
    parser.add_argument("--dry-run", action="store_true", help="Do not write back to SQLite")
    args = parser.parse_args()

    db_path = Path(args.db).expanduser().resolve()
    conn = sqlite3.connect(str(db_path))
    conn.row_factory = sqlite3.Row

    rows = load_pending_rows(conn, args.limit)
    print(f"[theory-ai-review] pending={len(rows)} db={db_path}")
    if not rows:
        return 0

    reviewed = 0
    approved = 0
    still_pending = 0
    for start in range(0, len(rows), args.batch_size):
        batch = rows[start : start + args.batch_size]
        print(f"[theory-ai-review] batch={start // args.batch_size + 1} size={len(batch)}")
        decisions = request_review_batch(
            batch=batch,
            base_url=args.base_url,
            api_key=args.api_key,
            model=args.model,
            reasoning_effort=args.reasoning_effort,
            timeout_seconds=args.timeout,
        )
        if not args.dry_run:
            apply_decisions(conn, batch, decisions)
            conn.commit()
        reviewed += len(batch)
        approved += sum(1 for item in decisions if item["review_status"] == "approved")
        still_pending += sum(1 for item in decisions if item["review_status"] != "approved")
        print(f"[theory-ai-review] reviewed={reviewed} approved={approved} remaining_review={still_pending}")

    print(f"[theory-ai-review] done reviewed={reviewed} approved={approved} non_approved={still_pending}")
    return 0


def load_pending_rows(conn: sqlite3.Connection, limit: int) -> list[ReviewRow]:
    sql = """
    SELECT id, question, selection_type, options_json, answer_keys_json, answer_texts_json,
           review_status, review_reason, confidence, question_hash
    FROM theory_bank_questions
    WHERE review_status IN ('pending', 'captured') OR needs_review = 1
    ORDER BY updated_at DESC, id DESC
    """
    if limit > 0:
        sql += f" LIMIT {int(limit)}"

    result: list[ReviewRow] = []
    for row in conn.execute(sql):
        result.append(
            ReviewRow(
                id=int(row["id"]),
                question=clean_text(row["question"] or ""),
                selection_type=(row["selection_type"] or "single").strip() or "single",
                options=decode_json_array(row["options_json"]),
                answer_keys=decode_string_array(row["answer_keys_json"]),
                answer_texts=decode_string_array(row["answer_texts_json"]),
                review_status=(row["review_status"] or "pending").strip() or "pending",
                review_reason=clean_text(row["review_reason"] or ""),
                confidence=float(row["confidence"] or 0.0),
                question_hash=(row["question_hash"] or "").strip(),
            )
        )
    return result


def decode_json_array(raw: Any) -> list[dict[str, Any]]:
    if raw in (None, "", "null"):
        return []
    try:
        value = json.loads(raw)
    except Exception:
        return []
    return value if isinstance(value, list) else []


def decode_string_array(raw: Any) -> list[str]:
    if raw in (None, "", "null"):
        return []
    try:
        value = json.loads(raw)
    except Exception:
        return []
    if not isinstance(value, list):
        return []
    return [str(item).strip() for item in value if str(item).strip()]


def request_review_batch(
    *,
    batch: list[ReviewRow],
    base_url: str,
    api_key: str,
    model: str,
    reasoning_effort: str,
    timeout_seconds: int,
) -> list[dict[str, Any]]:
    schema = {
        "type": "object",
        "properties": {
            "items": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "id": {"type": "integer"},
                        "question": {"type": "string"},
                        "selection_type": {"type": "string", "enum": ["single", "multiple"]},
                        "answer_keys": {"type": "array", "items": {"type": "string"}},
                        "review_status": {"type": "string", "enum": ["approved", "pending", "rejected"]},
                        "review_reason": {"type": "string"},
                        "confidence": {"type": "number"},
                    },
                    "required": ["id", "question", "selection_type", "answer_keys", "review_status", "review_reason", "confidence"],
                    "additionalProperties": False,
                },
            }
        },
        "required": ["items"],
        "additionalProperties": False,
    }

    payload = {
        "items": [
            {
                "id": row.id,
                "question": row.question,
                "selection_type": row.selection_type,
                "options": [{"key": str(item.get("key", "")).strip(), "content": clean_text(str(item.get("content", "")))} for item in row.options],
                "answer_keys_hint": row.answer_keys,
                "answer_texts_hint": row.answer_texts,
                "review_status": row.review_status,
                "review_reason": row.review_reason,
            }
            for row in batch
        ]
    }

    system_prompt = (
        "You review ISCC theory question-bank records that are currently pending manual review. "
        "For each item, determine the most reliable correct answer keys only from the question and options. "
        "If the answer can be determined with strong confidence, return review_status=approved. "
        "If ambiguous or unsafe, keep review_status=pending. "
        "Never change option count or option order. Return JSON only."
    )

    raw = call_ai(
        base_url=base_url,
        api_key=api_key,
        model=model,
        reasoning_effort=reasoning_effort,
        system_prompt=system_prompt,
        user_payload=payload,
        schema=schema,
        timeout_seconds=timeout_seconds,
    )
    parsed = json.loads(raw)
    items = parsed.get("items", [])
    by_id = {int(item["id"]): item for item in items if str(item.get("id", "")).strip()}

    decisions: list[dict[str, Any]] = []
    for row in batch:
        item = by_id.get(row.id)
        if not item:
            decisions.append(
                {
                    "id": row.id,
                    "question": row.question,
                    "selection_type": row.selection_type,
                    "answer_keys": row.answer_keys,
                    "review_status": "pending",
                    "review_reason": "AI missing item",
                    "confidence": 0.0,
                }
            )
            continue
        answer_keys = [str(key).strip().upper() for key in item.get("answer_keys", []) if str(key).strip()]
        answer_keys = [key for key in answer_keys if key in option_keys(row.options)]
        selection_type = str(item.get("selection_type", row.selection_type)).strip() or row.selection_type
        if selection_type not in {"single", "multiple"}:
            selection_type = "multiple" if len(answer_keys) > 1 else "single"
        review_status = str(item.get("review_status", "pending")).strip() or "pending"
        if not answer_keys:
            review_status = "pending"
        confidence = safe_confidence(item.get("confidence", 0.0))
        review_reason = clean_text(str(item.get("review_reason", "")))
        decisions.append(
            {
                "id": row.id,
                "question": clean_text(str(item.get("question", row.question))) or row.question,
                "selection_type": selection_type,
                "answer_keys": answer_keys,
                "review_status": review_status,
                "review_reason": review_reason,
                "confidence": confidence,
            }
        )
    return decisions


def option_keys(options: list[dict[str, Any]]) -> set[str]:
    return {str(item.get("key", "")).strip().upper() for item in options if str(item.get("key", "")).strip()}


def safe_confidence(value: Any) -> float:
    try:
        number = float(value)
    except Exception:
        return 0.0
    if number < 0:
        return 0.0
    if number > 1:
        return 1.0
    return number


def apply_decisions(conn: sqlite3.Connection, batch: list[ReviewRow], decisions: list[dict[str, Any]]) -> None:
    row_map = {row.id: row for row in batch}
    for item in decisions:
        row = row_map[item["id"]]
        answer_keys = item["answer_keys"]
        answer_texts = resolve_answer_texts(row.options, answer_keys)
        review_status = item["review_status"]
        needs_review = 0 if review_status == "approved" else 1
        options = patch_option_correctness(row.options, answer_keys)
        conn.execute(
            """
            UPDATE theory_bank_questions
            SET question = ?,
                selection_type = ?,
                options_json = ?,
                answer_keys_json = ?,
                answer_texts_json = ?,
                confidence = ?,
                needs_review = ?,
                review_status = ?,
                review_reason = ?,
                updated_at = datetime('now', 'localtime')
            WHERE id = ?
            """,
            (
                item["question"],
                item["selection_type"],
                json.dumps(options, ensure_ascii=False, separators=(",", ":")),
                json.dumps(answer_keys, ensure_ascii=False, separators=(",", ":")),
                json.dumps(answer_texts, ensure_ascii=False, separators=(",", ":")),
                item["confidence"],
                needs_review,
                review_status,
                item["review_reason"],
                row.id,
            ),
        )


def patch_option_correctness(options: list[dict[str, Any]], answer_keys: list[str]) -> list[dict[str, Any]]:
    correct = set(answer_keys)
    result = []
    for item in options:
        result.append(
            {
                **item,
                "is_correct": str(item.get("key", "")).strip().upper() in correct,
            }
        )
    return result


def resolve_answer_texts(options: list[dict[str, Any]], answer_keys: list[str]) -> list[str]:
    correct = set(answer_keys)
    values = []
    for item in options:
        key = str(item.get("key", "")).strip().upper()
        if key in correct:
            values.append(clean_text(str(item.get("content", ""))))
    return values


if __name__ == "__main__":
    raise SystemExit(main())
