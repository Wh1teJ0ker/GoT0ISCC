(() => {
  const THEORY_ROUTE_PATTERNS = ["/choice", "/paper"];

  function toHalfWidth(value) {
    return Array.from(String(value || ""))
      .map((char) => {
        const code = char.charCodeAt(0);
        if (code === 12288) {
          return " ";
        }
        if (code >= 65281 && code <= 65374) {
          return String.fromCharCode(code - 65248);
        }
        return char;
      })
      .join("");
  }

  function normalizeTheoryText(value) {
    const replaced = toHalfWidth(String(value || ""))
      .toLowerCase()
      .replace(/[\r\n]+/g, " ")
      .replace(/[（）（）().,，。；;：:？！?!、“”"'‘’【】\[\]《》、]/g, " ")
      .replace(/第\s*1\s*题/g, " ")
      .replace(/\s+/g, " ")
      .trim();
    return replaced;
  }

  function compactTheoryText(value) {
    return normalizeTheoryText(value).replace(/\s+/g, "");
  }

  function round(value) {
    return Math.round(Number(value || 0) * 100) / 100;
  }

  function splitAnswerOptions(value) {
    return String(value || "")
      .split(/[,，|/ ]+/)
      .map((item) => item.trim().toUpperCase())
      .filter(Boolean);
  }

  function theoryQuestionHash(question, options) {
    const normalizedOptions = (options || [])
      .map((option) => `${option.key}:${normalizeTheoryText(option.content)}`)
      .join("|");
    return `${normalizeTheoryText(question)}|${normalizedOptions}`;
  }

  function similarity(left, right) {
    const a = normalizeTheoryText(left);
    const b = normalizeTheoryText(right);
    if (!a || !b) {
      return 0;
    }
    if (a === b) {
      return 1;
    }
    if (a.includes(b) || b.includes(a)) {
      return 0.82;
    }
    const setA = new Set(a.split(/\s+/).filter(Boolean));
    const setB = new Set(b.split(/\s+/).filter(Boolean));
    if (!setA.size || !setB.size) {
      return 0;
    }
    let inter = 0;
    for (const item of setA) {
      if (setB.has(item)) {
        inter += 1;
      }
    }
    const union = new Set([...setA, ...setB]).size;
    return union ? inter / union : 0;
  }

  function alignOptions(questionOptions, expectedTexts, fallbackKeys) {
    const results = [];
    const used = new Set();
    (expectedTexts || []).forEach((expectedText, index) => {
      const fallbackKey = (fallbackKeys || [])[index] || "";
      let bestKey = fallbackKey;
      let bestScore = 0;
      for (const option of questionOptions || []) {
        if (used.has(option.key)) {
          continue;
        }
        const score = similarity(expectedText, option.content);
        if (score > bestScore) {
          bestScore = score;
          bestKey = option.key;
        }
      }
      if (bestKey) {
        used.add(bestKey);
        results.push(bestKey);
      }
    });
    return results.length ? results.sort() : [...(fallbackKeys || [])];
  }

  function scoreBankItem(normalizedQuestion, compactQuestion, item) {
    if (!item) {
      return [0, "empty"];
    }
    if (normalizedQuestion === item.normalized_question) {
      return [1, "normalized-question exact"];
    }
    if (compactQuestion && compactQuestion === item.compact_question) {
      return [0.98, "compact-question exact"];
    }
    const questionScore = similarity(normalizedQuestion, item.normalized_question);
    const keywordHits = (item.keywords || []).reduce((hits, keyword) => {
      if (!keyword) {
        return hits;
      }
      return compactQuestion.includes(compactTheoryText(keyword)) ? hits + 1 : hits;
    }, 0);
    const keywordBoost = Math.min(keywordHits * 0.02, 0.12);
    return [Math.min(questionScore + keywordBoost, 0.97), keywordHits > 0 ? `similarity + keyword(${keywordHits})` : "similarity"];
  }

  function matchTheoryQuestion(question, bankItems) {
    if (!question || !question.title || !Array.isArray(bankItems) || !bankItems.length) {
      return {
        status: "unmatched",
        method: "local-bank",
        confidence: 0,
        recommended_options: [],
        recommended_texts: [],
        candidates: []
      };
    }
    const normalizedQuestion = normalizeTheoryText(question.title);
    const compactQuestion = compactTheoryText(question.title);
    const candidates = bankItems
      .map((item) => {
        const [score, reason] = scoreBankItem(normalizedQuestion, compactQuestion, item);
        return { item, score, reason };
      })
      .sort((left, right) => right.score - left.score);
    const previews = candidates.slice(0, 3).map(({ item, score }) => ({
      question: item.question,
      recommended_options: alignOptions(question.options, item.correct_texts, item.correct_options),
      recommended_texts: item.correct_texts || [],
      confidence: round(score)
    }));
    if (!candidates.length || candidates[0].score < 0.35) {
      return {
        status: "weak_match",
        method: "local-bank",
        confidence: candidates[0] ? round(candidates[0].score) : 0,
        reason: "题库中没有足够接近的题目",
        recommended_options: [],
        recommended_texts: [],
        candidates: previews
      };
    }
    const best = candidates[0];
    return {
      status: "matched",
      method: "local-bank",
      confidence: round(best.score),
      reason: `当前答案来自本地题库相似题匹配。命中方式：${best.reason}。`,
      recommended_option: alignOptions(question.options, best.item.correct_texts, best.item.correct_options).join(","),
      recommended_options: alignOptions(question.options, best.item.correct_texts, best.item.correct_options),
      recommended_texts: best.item.correct_texts || [],
      reference_question: best.item.question,
      candidates: previews
    };
  }

  function isTheoryRoute(urlLike) {
    try {
      const url = new URL(urlLike, location.origin);
      return THEORY_ROUTE_PATTERNS.some((pattern) => url.pathname === pattern || url.pathname.startsWith(`${pattern}/`));
    } catch (_error) {
      return false;
    }
  }

  function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  window.TheoryPilotShared = {
    THEORY_ROUTE_PATTERNS,
    normalizeTheoryText,
    compactTheoryText,
    splitAnswerOptions,
    theoryQuestionHash,
    similarity,
    alignOptions,
    matchTheoryQuestion,
    isTheoryRoute,
    sleep,
    round
  };
})();
