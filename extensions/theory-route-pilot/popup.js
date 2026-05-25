(async () => {
  const shared = window.TheoryPilotShared;

  function qs(id) {
    return document.getElementById(id);
  }

  async function send(type, payload) {
    const response = await chrome.runtime.sendMessage({type, payload});
    if (!response?.ok) {
      throw new Error(response?.error || "请求失败");
    }
    return response.data;
  }

  function setForm(settings) {
    [
      "enabled",
      "autoStart",
      "autoSubmit",
      "aiEnabled"
    ].forEach((key) => {
      qs(key).checked = Boolean(settings[key]);
    });
    [
      "aiBaseURL",
      "aiAPIKey",
      "aiModel",
      "aiReasoningEffort",
      "localConfidenceThreshold",
      "aiConfidenceThreshold",
      "fetchCooldownMs",
      "submitCooldownMs",
      "questionCooldownMs"
    ].forEach((key) => {
      qs(key).value = settings[key] ?? "";
    });
  }

  function collectForm() {
    return {
      enabled: qs("enabled").checked,
      autoStart: qs("autoStart").checked,
      autoSubmit: qs("autoSubmit").checked,
      aiEnabled: qs("aiEnabled").checked,
      aiBaseURL: qs("aiBaseURL").value.trim(),
      aiAPIKey: qs("aiAPIKey").value.trim(),
      aiModel: qs("aiModel").value.trim(),
      aiReasoningEffort: qs("aiReasoningEffort").value.trim(),
      localConfidenceThreshold: Number(qs("localConfidenceThreshold").value || 0),
      aiConfidenceThreshold: Number(qs("aiConfidenceThreshold").value || 0),
      fetchCooldownMs: Number(qs("fetchCooldownMs").value || 0),
      submitCooldownMs: Number(qs("submitCooldownMs").value || 0),
      questionCooldownMs: Number(qs("questionCooldownMs").value || 0)
    };
  }

  function renderRuntime(runtime) {
    const node = qs("runtime");
    if (!runtime) {
      node.classList.add("empty");
      node.textContent = "尚未读取运行状态";
      return;
    }
    node.classList.remove("empty");
    node.textContent = [
      `status: ${runtime.status || "-"}`,
      runtime.message ? `message: ${runtime.message}` : "",
      runtime.question?.number ? `question: 第 ${runtime.question.number} 题` : "",
      runtime.question?.title ? runtime.question.title : "",
      runtime.decision?.recommended_options?.length ? `answer: ${runtime.decision.recommended_options.join(", ")}` : "",
      runtime.decision?.confidence !== undefined ? `confidence: ${runtime.decision.confidence}` : "",
      runtime.scoreText ? `score: ${runtime.scoreText}` : "",
      runtime.completed !== undefined ? `completed: ${runtime.completed}` : "",
      runtime.updatedAt ? `updatedAt: ${runtime.updatedAt}` : ""
    ].filter(Boolean).join("\n");
  }

  function normalizeImportedBank(payload) {
    const items = Array.isArray(payload?.items) ? payload.items : (Array.isArray(payload) ? payload : []);
    const normalizedItems = items
      .map((item, index) => {
        const question = String(item.question || "").trim();
        const options = Array.isArray(item.options) ? item.options : [];
        const correctOptions = Array.isArray(item.correct_options) ? item.correct_options.map((value) => String(value).trim().toUpperCase()).filter(Boolean) : [];
        const correctTexts = Array.isArray(item.correct_texts) ? item.correct_texts.map((value) => String(value).trim()).filter(Boolean) : [];
        const normalizedQuestion = shared.normalizeTheoryText(question);
        const compactQuestion = shared.compactTheoryText(question);
        const keywords = [question, normalizedQuestion, ...options.map((option) => option.content || "")]
          .map((value) => shared.normalizeTheoryText(value))
          .filter(Boolean);
        return {
          id: item.id || `bank-${index + 1}`,
          question,
          normalized_question: normalizedQuestion,
          compact_question: compactQuestion,
          correct_options: correctOptions,
          correct_texts: correctTexts,
          keywords,
          options: options.map((option, optionIndex) => ({
            key: String(option.key || String.fromCharCode(65 + optionIndex)).trim().toUpperCase(),
            content: String(option.content || "").trim()
          }))
        };
      })
      .filter((item) => item.question && item.options.length);
    return {
      items: normalizedItems,
      meta: {
        importedAt: new Date().toLocaleString("zh-CN", {hour12: false}),
        total: normalizedItems.length
      }
    };
  }

  async function refreshAll() {
    const settings = await send("theory-pilot:get-settings");
    const runtime = await send("theory-pilot:get-runtime");
    const bank = await send("theory-pilot:get-bank");
    setForm(settings);
    renderRuntime(runtime);
    qs("bank-meta").textContent = bank?.meta?.total ? `已导入 ${bank.meta.total} 条题库，更新时间 ${bank.meta.importedAt || "-"}` : "未导入题库";
  }

  qs("save-settings").addEventListener("click", async () => {
    await send("theory-pilot:set-settings", collectForm());
    await refreshAll();
  });

  qs("refresh-runtime").addEventListener("click", refreshAll);

  qs("clear-runtime").addEventListener("click", async () => {
    await send("theory-pilot:clear-runtime");
    await refreshAll();
  });

  qs("import-bank").addEventListener("click", async () => {
    const file = qs("bank-file").files?.[0];
    if (!file) {
      return;
    }
    const text = await file.text();
    const payload = JSON.parse(text);
    const bank = normalizeImportedBank(payload);
    await send("theory-pilot:import-bank", bank);
    await refreshAll();
  });

  await refreshAll();
})();
