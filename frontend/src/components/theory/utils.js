export function createReviewDraft(item) {
  if (!item) {
    return null;
  }
  return {
    id: item.id,
    question: item.question || "",
    selection_type: item.selection_type || (item.answer_keys?.length > 1 ? "multiple" : "single"),
    review_status: item.review_status || "approved",
    review_reason: item.review_reason || "",
    options: (item.options || []).map((option) => ({
      ...option,
      is_correct: (item.answer_keys || []).includes(option.key),
    })),
  };
}

export function collectAnswerKeys(options) {
  return (options || []).filter((item) => item.is_correct).map((item) => item.key);
}

export function collectAnswerTexts(options) {
  return (options || []).filter((item) => item.is_correct).map((item) => item.content).filter(Boolean);
}

export function questionTypeLabel(selectionType) {
  return selectionType === "multiple" ? "多选题" : "单选题";
}

export function formatConfidence(value) {
  if (value === undefined || value === null || value === "") {
    return "confidence -";
  }
  return `confidence ${value}`;
}

export function displayValue(value) {
  if (value === undefined || value === null) {
    return "-";
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    return trimmed ? trimmed : "-";
  }
  if (Array.isArray(value)) {
    return value.length ? value.join(", ") : "-";
  }
  return value;
}

export function shortPath(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "-";
  }
  const parts = text.split("/");
  return parts.length > 3 ? `.../${parts.slice(-3).join("/")}` : text;
}
