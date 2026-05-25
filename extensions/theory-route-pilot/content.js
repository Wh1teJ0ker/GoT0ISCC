(() => {
  const shared = window.TheoryPilotShared;
  if (!shared) {
    return;
  }

  const STATE = {
    running: false,
    stopped: false,
    lastQuestionKey: "",
    completed: 0,
    overlay: null,
    collapsed: false,
    runtime: null,
    settings: null,
    bank: null
  };

  const selectors = {
    optionInputs: "input[type='radio'][name='option'], input[type='checkbox'][name='option']",
    nonce: "input[name='nonce']",
    number: "input[name='number']",
    submitForm: "form[action='/paper'], form[action='/choice'], form"
  };

  function nowText() {
    return new Date().toLocaleString("zh-CN", {hour12: false});
  }

  function renderFeedback(message, tone = "neutral") {
    const overlay = ensureOverlay();
    const node = overlay.querySelector("#theory-route-pilot-feedback");
    if (!node) {
      return;
    }
    node.textContent = message || "";
    node.style.color = tone === "error" ? "#ff9b9b" : tone === "success" ? "#8ee6b5" : "#9fb0c6";
  }

  function renderRuntimeSnapshot(runtime) {
    const overlay = ensureOverlay();
    const node = overlay.querySelector("#theory-route-pilot-runtime");
    if (!node) {
      return;
    }
    if (!runtime) {
      node.textContent = "尚无运行记录";
      node.style.color = "#8ea0b7";
      return;
    }
    node.style.color = "#eef3fa";
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

  function renderBankMeta(bank) {
    const overlay = ensureOverlay();
    const node = overlay.querySelector("#theory-route-pilot-bank-meta");
    if (!node) {
      return;
    }
    const meta = bank?.meta || {};
    if (!meta.total) {
      node.textContent = "当前未加载题库";
      return;
    }
    node.textContent = `已加载 ${meta.total} 条，来源 ${meta.source_type || "-"}，更新时间 ${meta.generated_at || meta.importedAt || "-"}`;
  }

  function fillSettingsForm(settings) {
    const overlay = ensureOverlay();
    overlay.querySelector("#theory-route-pilot-enabled").checked = Boolean(settings.enabled);
    overlay.querySelector("#theory-route-pilot-auto-start").checked = Boolean(settings.autoStart);
    overlay.querySelector("#theory-route-pilot-auto-submit").checked = Boolean(settings.autoSubmit);
    overlay.querySelector("#theory-route-pilot-ai-enabled").checked = Boolean(settings.aiEnabled);
    overlay.querySelector("#theory-route-pilot-ai-base-url").value = settings.aiBaseURL || "";
    overlay.querySelector("#theory-route-pilot-ai-api-key").value = settings.aiAPIKey || "";
    overlay.querySelector("#theory-route-pilot-ai-model").value = settings.aiModel || "";
    overlay.querySelector("#theory-route-pilot-ai-reasoning").value = settings.aiReasoningEffort || "";
    overlay.querySelector("#theory-route-pilot-local-threshold").value = settings.localConfidenceThreshold ?? "";
    overlay.querySelector("#theory-route-pilot-ai-threshold").value = settings.aiConfidenceThreshold ?? "";
    overlay.querySelector("#theory-route-pilot-fetch-cooldown").value = settings.fetchCooldownMs ?? "";
    overlay.querySelector("#theory-route-pilot-submit-cooldown").value = settings.submitCooldownMs ?? "";
    overlay.querySelector("#theory-route-pilot-question-cooldown").value = settings.questionCooldownMs ?? "";
  }

  function collectSettingsForm() {
    const overlay = ensureOverlay();
    return {
      enabled: overlay.querySelector("#theory-route-pilot-enabled").checked,
      autoStart: overlay.querySelector("#theory-route-pilot-auto-start").checked,
      autoSubmit: overlay.querySelector("#theory-route-pilot-auto-submit").checked,
      aiEnabled: overlay.querySelector("#theory-route-pilot-ai-enabled").checked,
      aiBaseURL: overlay.querySelector("#theory-route-pilot-ai-base-url").value.trim(),
      aiAPIKey: overlay.querySelector("#theory-route-pilot-ai-api-key").value.trim(),
      aiModel: overlay.querySelector("#theory-route-pilot-ai-model").value.trim(),
      aiReasoningEffort: overlay.querySelector("#theory-route-pilot-ai-reasoning").value.trim(),
      localConfidenceThreshold: Number(overlay.querySelector("#theory-route-pilot-local-threshold").value || 0),
      aiConfidenceThreshold: Number(overlay.querySelector("#theory-route-pilot-ai-threshold").value || 0),
      fetchCooldownMs: Number(overlay.querySelector("#theory-route-pilot-fetch-cooldown").value || 0),
      submitCooldownMs: Number(overlay.querySelector("#theory-route-pilot-submit-cooldown").value || 0),
      questionCooldownMs: Number(overlay.querySelector("#theory-route-pilot-question-cooldown").value || 0)
    };
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
        importedAt: nowText(),
        generated_at: nowText(),
        source_type: "sidebar-import",
        total: normalizedItems.length,
        signature: `sidebar-${normalizedItems.length}`
      }
    };
  }

  async function saveSettingsForm() {
    const next = collectSettingsForm();
    const response = await sendMessage("theory-pilot:set-settings", next);
    if (!response?.ok) {
      throw new Error(response?.error || "保存设置失败");
    }
    STATE.settings = response.data;
    fillSettingsForm(STATE.settings);
    renderFeedback("配置已保存", "success");
    if (!STATE.settings.enabled) {
      STATE.stopped = true;
      STATE.running = false;
    }
  }

  async function importBankFromSidebar() {
    const overlay = ensureOverlay();
    const file = overlay.querySelector("#theory-route-pilot-bank-file").files?.[0];
    if (!file) {
      renderFeedback("请选择一个题库 JSON 文件", "error");
      return;
    }
    const text = await file.text();
    const payload = JSON.parse(text);
    const normalized = normalizeImportedBank(payload);
    const response = await sendMessage("theory-pilot:import-bank", normalized);
    if (!response?.ok) {
      throw new Error(response?.error || "导入题库失败");
    }
    STATE.bank = normalized;
    renderBankMeta(STATE.bank);
    renderFeedback(`题库已导入 ${normalized.meta.total} 条`, "success");
  }

  async function refreshSidebarContext() {
    STATE.settings = await getSettings();
    STATE.bank = await getBank();
    fillSettingsForm(STATE.settings);
    renderBankMeta(STATE.bank);
    renderRuntimeSnapshot(STATE.runtime);
  }

  function applyOverlayLayout(root) {
    root.style.width = STATE.collapsed ? "72px" : "420px";
    root.style.padding = STATE.collapsed ? "12px 10px" : "16px";
    root.querySelector("#theory-route-pilot-body").style.display = STATE.collapsed ? "none" : "block";
    root.querySelector("#theory-route-pilot-collapse").textContent = STATE.collapsed ? "展开" : "收起";
    root.querySelector("#theory-route-pilot-title").textContent = STATE.collapsed ? "TRP" : "Theory Route Pilot";
    document.body.style.marginRight = shared.isTheoryRoute(location.href) ? (STATE.collapsed ? "72px" : "420px") : "";
  }

  function ensureOverlay() {
    if (STATE.overlay && document.body.contains(STATE.overlay)) {
      applyOverlayLayout(STATE.overlay);
      return STATE.overlay;
    }
    const root = document.createElement("div");
    root.id = "theory-route-pilot-overlay";
    root.style.position = "fixed";
    root.style.top = "0";
    root.style.right = "0";
    root.style.bottom = "0";
    root.style.zIndex = "999999";
    root.style.height = "100vh";
    root.style.overflow = "hidden auto";
    root.style.borderRadius = "0";
    root.style.borderLeft = "1px solid rgba(255,255,255,0.08)";
    root.style.background = "linear-gradient(180deg, rgba(14,18,24,0.96), rgba(11,15,20,0.98))";
    root.style.color = "#f5f7fb";
    root.style.boxShadow = "-18px 0 42px rgba(0,0,0,0.32)";
    root.style.backdropFilter = "blur(14px)";
    root.style.font = "13px/1.45 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif";
    root.innerHTML = `
      <div style="display:flex;justify-content:space-between;gap:8px;align-items:center;margin-bottom:10px;">
        <strong id="theory-route-pilot-title" style="font-size:14px;letter-spacing:0.02em;">Theory Route Pilot</strong>
        <div style="display:flex;gap:8px;align-items:center;">
          <button id="theory-route-pilot-collapse" style="border:0;background:rgba(255,255,255,0.12);color:#fff;padding:6px 10px;border-radius:10px;cursor:pointer;">收起</button>
          <button id="theory-route-pilot-stop" style="border:0;background:#ff6b6b;color:#fff;padding:6px 10px;border-radius:10px;cursor:pointer;">停止</button>
        </div>
      </div>
      <div id="theory-route-pilot-body">
        <section style="padding:12px;border-radius:14px;background:rgba(255,255,255,0.04);border:1px solid rgba(255,255,255,0.06);">
          <div id="theory-route-pilot-status" style="font-size:12px;color:#c8d2df;">等待中</div>
          <div id="theory-route-pilot-detail" style="margin-top:10px;white-space:pre-wrap;color:#eef3fa;"></div>
          <div id="theory-route-pilot-feedback" style="margin-top:10px;font-size:12px;color:#9fb0c6;"></div>
        </section>

        <section style="margin-top:12px;padding:12px;border-radius:14px;background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);">
          <div style="font-size:12px;color:#f3f7fc;margin-bottom:8px;">当前运行记录</div>
          <div id="theory-route-pilot-runtime" style="white-space:pre-wrap;color:#8ea0b7;">尚无运行记录</div>
        </section>

        <section style="margin-top:12px;padding:12px;border-radius:14px;background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);">
          <div style="font-size:12px;color:#f3f7fc;margin-bottom:10px;">插件开关</div>
          <label style="display:flex;justify-content:space-between;gap:12px;align-items:center;margin-top:8px;"><span>启用插件</span><input id="theory-route-pilot-enabled" type="checkbox"></label>
          <label style="display:flex;justify-content:space-between;gap:12px;align-items:center;margin-top:8px;"><span>理论题路由自动启动</span><input id="theory-route-pilot-auto-start" type="checkbox"></label>
          <label style="display:flex;justify-content:space-between;gap:12px;align-items:center;margin-top:8px;"><span>满足阈值时自动提交</span><input id="theory-route-pilot-auto-submit" type="checkbox"></label>
        </section>

        <section style="margin-top:12px;padding:12px;border-radius:14px;background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);">
          <div style="font-size:12px;color:#f3f7fc;margin-bottom:10px;">AI 设置</div>
          <label style="display:flex;justify-content:space-between;gap:12px;align-items:center;margin-top:8px;"><span>启用 AI 复核</span><input id="theory-route-pilot-ai-enabled" type="checkbox"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">Base URL</div><input id="theory-route-pilot-ai-base-url" type="text" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">API Key</div><input id="theory-route-pilot-ai-api-key" type="password" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">模型</div><input id="theory-route-pilot-ai-model" type="text" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">推理强度</div><input id="theory-route-pilot-ai-reasoning" type="text" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
        </section>

        <section style="margin-top:12px;padding:12px;border-radius:14px;background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);">
          <div style="font-size:12px;color:#f3f7fc;margin-bottom:10px;">阈值与节流</div>
          <label style="display:block;margin-top:8px;"><div style="color:#9fb0c6;margin-bottom:4px;">题库自动提交阈值</div><input id="theory-route-pilot-local-threshold" type="number" min="0" max="1" step="0.01" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">AI 自动提交阈值</div><input id="theory-route-pilot-ai-threshold" type="number" min="0" max="1" step="0.01" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">抓题冷却(ms)</div><input id="theory-route-pilot-fetch-cooldown" type="number" min="0" step="100" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">提交前冷却(ms)</div><input id="theory-route-pilot-submit-cooldown" type="number" min="0" step="100" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
          <label style="display:block;margin-top:10px;"><div style="color:#9fb0c6;margin-bottom:4px;">题间冷却(ms)</div><input id="theory-route-pilot-question-cooldown" type="number" min="0" step="100" style="width:100%;padding:9px 10px;border-radius:10px;border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);color:#fff;"></label>
        </section>

        <section style="margin-top:12px;padding:12px;border-radius:14px;background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);">
          <div style="font-size:12px;color:#f3f7fc;margin-bottom:10px;">题库</div>
          <div id="theory-route-pilot-bank-meta" style="color:#9fb0c6;">当前未加载题库</div>
          <input id="theory-route-pilot-bank-file" type="file" accept=".json" style="display:block;margin-top:10px;color:#9fb0c6;max-width:100%;">
          <div style="display:flex;gap:8px;flex-wrap:wrap;margin-top:10px;">
            <button id="theory-route-pilot-import-bank" style="border:0;background:rgba(255,255,255,0.1);color:#fff;padding:8px 10px;border-radius:10px;cursor:pointer;">导入题库</button>
          </div>
        </section>

        <section style="display:flex;gap:8px;flex-wrap:wrap;margin-top:12px;padding-bottom:16px;">
          <button id="theory-route-pilot-save-settings" style="border:0;background:linear-gradient(135deg,#59d89a,#2b7d5d);color:#08110d;padding:9px 12px;border-radius:12px;cursor:pointer;font-weight:700;">保存配置</button>
          <button id="theory-route-pilot-start" style="border:0;background:rgba(255,255,255,0.1);color:#fff;padding:9px 12px;border-radius:12px;cursor:pointer;">启动</button>
          <button id="theory-route-pilot-refresh" style="border:0;background:rgba(255,255,255,0.1);color:#fff;padding:9px 12px;border-radius:12px;cursor:pointer;">刷新</button>
          <button id="theory-route-pilot-clear-runtime" style="border:0;background:rgba(255,255,255,0.1);color:#fff;padding:9px 12px;border-radius:12px;cursor:pointer;">清状态</button>
        </section>
      </div>
    `;
    root.querySelector("#theory-route-pilot-collapse").addEventListener("click", () => {
      STATE.collapsed = !STATE.collapsed;
      applyOverlayLayout(root);
    });
    root.querySelector("#theory-route-pilot-stop").addEventListener("click", async () => {
      STATE.stopped = true;
      STATE.running = false;
      await pushRuntime({status: "stopped", message: "已手动停止", stoppedAt: nowText()});
      renderStatus("已手动停止");
    });
    root.querySelector("#theory-route-pilot-start").addEventListener("click", async () => {
      STATE.stopped = false;
      renderFeedback("准备启动自动答题");
      await automateOnce("manual");
    });
    root.querySelector("#theory-route-pilot-refresh").addEventListener("click", async () => {
      await refreshSidebarContext();
      renderFeedback("侧边栏已刷新", "success");
    });
    root.querySelector("#theory-route-pilot-save-settings").addEventListener("click", async () => {
      try {
        await saveSettingsForm();
      } catch (error) {
        renderFeedback(error?.message || String(error), "error");
      }
    });
    root.querySelector("#theory-route-pilot-import-bank").addEventListener("click", async () => {
      try {
        await importBankFromSidebar();
      } catch (error) {
        renderFeedback(error?.message || String(error), "error");
      }
    });
    root.querySelector("#theory-route-pilot-clear-runtime").addEventListener("click", async () => {
      const response = await sendMessage("theory-pilot:clear-runtime");
      if (!response?.ok) {
        renderFeedback(response?.error || "清理状态失败", "error");
        return;
      }
      STATE.runtime = null;
      renderRuntimeSnapshot(null);
      renderFeedback("运行状态已清空", "success");
    });
    document.body.appendChild(root);
    STATE.overlay = root;
    applyOverlayLayout(root);
    return root;
  }

  function renderStatus(message, detail = "") {
    const overlay = ensureOverlay();
    overlay.querySelector("#theory-route-pilot-status").textContent = message || "等待中";
    overlay.querySelector("#theory-route-pilot-detail").textContent = detail || "";
  }

  async function sendMessage(type, payload) {
    return chrome.runtime.sendMessage({type, payload});
  }

  async function getSettings() {
    const response = await sendMessage("theory-pilot:get-settings");
    if (!response?.ok) {
      throw new Error(response?.error || "读取设置失败");
    }
    return response.data;
  }

  async function getBank() {
    const response = await sendMessage("theory-pilot:get-bank");
    if (!response?.ok) {
      throw new Error(response?.error || "读取题库失败");
    }
    return response.data || {items: [], meta: {}};
  }

  async function pushRuntime(patch) {
    const runtime = {
      updatedAt: nowText(),
      page: location.href,
      completed: STATE.completed,
      ...patch
    };
    STATE.runtime = runtime;
    renderRuntimeSnapshot(runtime);
    await sendMessage("theory-pilot:update-runtime", runtime);
  }

  function isLoginPage() {
    const pageText = document.body?.innerText || "";
    const hasPassword = document.querySelector("input[type='password'], input[name='password']");
    return hasPassword && (pageText.includes("登录") || pageText.includes("忘记密码"));
  }

  function extractScoreText() {
    const pageText = (document.body?.innerText || "").replace(/\s+/g, " ");
    const scoreMatch = pageText.match(/(?:得分|分数|成绩)\s*(?:是|为|[:：])?\s*([0-9]+(?:\.[0-9]+)?)(?:\s*\/\s*([0-9]+(?:\.[0-9]+)?))?/);
    if (!scoreMatch) {
      return "";
    }
    return scoreMatch[2] ? `${scoreMatch[1]}/${scoreMatch[2]}` : scoreMatch[1];
  }

  function extractQuestionNumber() {
    const text = (document.body?.innerText || "").replace(/\s+/g, " ");
    const match = text.match(/第\s*([0-9]+)\s*题/);
    if (match) {
      return Number(match[1]);
    }
    return 0;
  }

  function extractTheoryQuestion() {
    const inputs = Array.from(document.querySelectorAll(selectors.optionInputs));
    const options = [];
    let title = "";
    for (const input of inputs) {
      const key = String(input.value || "").trim().toUpperCase();
      const label = input.closest("label") || input.parentElement;
      let text = "";
      if (label) {
        text = (label.innerText || "").replace(/\s+/g, " ").trim();
      }
      const cleaned = text.replace(/^[A-D][.．、]?\s*/, "").trim();
      if (!title) {
        const containerText = (label?.closest("form, .row, body")?.innerText || document.body.innerText || "").replace(/\s+/g, " ").trim();
        const optionIndex = containerText.indexOf(text);
        title = optionIndex > 0 ? containerText.slice(0, optionIndex).trim() : title;
      }
      options.push({
        key,
        content: cleaned || text,
        input_type: input.type
      });
    }
    const number = extractQuestionNumber();
    return {
      number,
      title: title.replace(/第\s*\d+\s*题/, "").trim(),
      options,
      selection_type: options.some((item) => item.input_type === "checkbox") ? "multiple" : "single"
    };
  }

  function validQuestion(question) {
    return Boolean(question?.title && Array.isArray(question.options) && question.options.length);
  }

  async function requestAIDecision(_settings, question, match) {
    const response = await sendMessage("theory-pilot:ai-decision", {
      question: {
        title: question.title,
        selection_type: question.selection_type,
        options: question.options
      },
      local_match: match,
      candidates: match.candidates || []
    });
    if (!response?.ok) {
      throw new Error(response?.error || "AI 请求失败");
    }
    return response.data;
  }

  function selectOptions(optionKeys) {
    const normalized = new Set((optionKeys || []).map((item) => String(item).trim().toUpperCase()).filter(Boolean));
    let selected = 0;
    for (const input of document.querySelectorAll(selectors.optionInputs)) {
      const checked = normalized.has(String(input.value || "").trim().toUpperCase());
      input.checked = checked;
      input.dispatchEvent(new Event("change", {bubbles: true}));
      if (checked) {
        selected += 1;
      }
    }
    return selected;
  }

  async function submitCurrentQuestion() {
    const form = document.querySelector(selectors.submitForm);
    if (!form) {
      throw new Error("未找到理论题提交表单");
    }
    const submitButton = form.querySelector("button[type='submit'], input[type='submit']");
    if (submitButton) {
      submitButton.click();
    } else {
      form.submit();
    }
  }

  async function waitForNextQuestion(previousKey, timeoutMs = 15000) {
    const startedAt = Date.now();
    while (Date.now() - startedAt < timeoutMs) {
      await shared.sleep(1200);
      const question = extractTheoryQuestion();
      const key = `${question.number}|${shared.normalizeTheoryText(question.title)}`;
      if (validQuestion(question) && key && key !== previousKey) {
        return question;
      }
      if (!validQuestion(question) && extractScoreText()) {
        return null;
      }
    }
    return undefined;
  }

  async function automateOnce(trigger = "auto") {
    const settings = await getSettings();
    const bank = await getBank();
    const autoTriggered = trigger === "auto";
    if (!settings.enabled) {
      renderFeedback("插件未启用", "error");
      return;
    }
    if (autoTriggered && !settings.autoStart) {
      renderFeedback("已关闭自动启动");
      return;
    }
    if (!settings.autoSubmit) {
      renderFeedback("已关闭自动提交", "error");
      return;
    }
    if (!shared.isTheoryRoute(location.href)) {
      renderFeedback("当前页面不是理论题路由", "error");
      return;
    }
    if (STATE.running || STATE.stopped) {
      if (STATE.stopped) {
        renderFeedback("当前已处于手动停止状态，点击启动可恢复");
      }
      return;
    }
    STATE.running = true;
    ensureOverlay();

    try {
      if (isLoginPage()) {
        renderStatus("检测到登录页", "当前页面仍未登录，插件不会代替你登录。");
        await pushRuntime({status: "login-required", message: "检测到登录页"});
        return;
      }
      await pushRuntime({status: "running", message: "理论题插件已启动"});
      renderStatus("理论题插件已启动", "按当前页面自动识别题目并节流答题。");

      while (!STATE.stopped) {
        const question = extractTheoryQuestion();
        const questionKey = `${question.number}|${shared.normalizeTheoryText(question.title)}`;
        if (!validQuestion(question)) {
          const scoreText = extractScoreText();
          const message = scoreText ? `远端显示理论题已完成，当前成绩 ${scoreText}` : "当前页面没有可提交题目";
          renderStatus(message);
          await pushRuntime({status: "completed", message, scoreText});
          break;
        }
        if (questionKey && questionKey === STATE.lastQuestionKey) {
          const message = "检测到题目未继续推进，自动停止";
          renderStatus(message, question.title);
          await pushRuntime({status: "stopped", message, question});
          break;
        }

        STATE.lastQuestionKey = questionKey;
        const match = shared.matchTheoryQuestion(question, bank.items || []);
        let decision = {
          recommended_options: match.recommended_options || [],
          recommended_texts: match.recommended_texts || [],
          confidence: Number(match.confidence || 0),
          reason: match.reason || ""
        };

        renderStatus(`第 ${question.number || "?"} 题处理中`, `${question.title}\n题库命中: ${decision.recommended_options.join(", ") || "-"}\nconfidence ${decision.confidence}`);
        await pushRuntime({
          status: "running",
          question,
          match,
          decision,
          completed: STATE.completed
        });

        if (!decision.recommended_options.length || decision.confidence < Number(settings.localConfidenceThreshold || 0.86)) {
          if (!settings.aiEnabled || !settings.aiAPIKey) {
            const message = "题库置信度不足且未配置 AI，停止自动答题";
            renderStatus(message, question.title);
            await pushRuntime({status: "stopped", message, question, match});
            break;
          }
          renderStatus(`第 ${question.number || "?"} 题 AI 复核中`, question.title);
          decision = await requestAIDecision(settings, question, match);
          await pushRuntime({
            status: "running",
            question,
            match,
            decision,
            completed: STATE.completed,
            message: `AI 已返回，confidence ${decision.confidence}`
          });
        } else if (settings.aiEnabled && settings.aiAPIKey) {
          renderStatus(`第 ${question.number || "?"} 题 AI 复核中`, "题库已命中，继续做 AI 核对。");
          decision = await requestAIDecision(settings, question, match);
          await pushRuntime({
            status: "running",
            question,
            match,
            decision,
            completed: STATE.completed,
            message: `AI 已复核，confidence ${decision.confidence}`
          });
        }

        if (!Array.isArray(decision.recommended_options) || !decision.recommended_options.length || Number(decision.confidence || 0) < Number(settings.aiConfidenceThreshold || 0.8)) {
          const message = "未达到自动提交阈值，停在当前题等待人工处理";
          renderStatus(message, `${question.title}\n${decision.reason || ""}`);
          await pushRuntime({status: "manual-needed", message, question, match, decision});
          break;
        }

        renderStatus(`第 ${question.number || "?"} 题等待提交`, `固定等待 ${Math.round((settings.submitCooldownMs || 4000) / 1000)}s 后提交`);
        await shared.sleep(Number(settings.submitCooldownMs || 4000));
        if (STATE.stopped) {
          break;
        }

        const selectedCount = selectOptions(decision.recommended_options);
        if (!selectedCount) {
          throw new Error("未能选中任何答案");
        }

        await pushRuntime({
          status: "submitting",
          message: `提交第 ${question.number || "?"} 题`,
          question,
          match,
          decision,
          completed: STATE.completed
        });
        await submitCurrentQuestion();
        STATE.completed += 1;

        renderStatus(`第 ${question.number || "?"} 题已提交`, `题间等待 ${Math.round((settings.questionCooldownMs || 5000) / 1000)}s`);
        await shared.sleep(Number(settings.questionCooldownMs || 5000));

        const nextQuestion = await waitForNextQuestion(questionKey, 15000);
        if (nextQuestion === undefined) {
          const message = "已提交但未确认题目推进，自动停止";
          renderStatus(message, question.title);
          await pushRuntime({status: "stopped", message, question, match, decision, completed: STATE.completed});
          break;
        }
        if (nextQuestion === null) {
          const scoreText = extractScoreText();
          const message = scoreText ? `理论题已完成，当前成绩 ${scoreText}` : "理论题已完成";
          renderStatus(message);
          await pushRuntime({status: "completed", message, scoreText, completed: STATE.completed});
          break;
        }

        renderStatus(`已完成 ${STATE.completed} 题`, `下一题：第 ${nextQuestion.number || "?"} 题`);
        await pushRuntime({
          status: "running",
          message: "已推进到下一题",
          question: nextQuestion,
          completed: STATE.completed
        });
        await shared.sleep(Number(settings.fetchCooldownMs || 2000));
      }
    } catch (error) {
      renderStatus("自动答题失败", error?.message || String(error));
      await pushRuntime({
        status: "error",
        message: error?.message || String(error),
        completed: STATE.completed
      });
    } finally {
      STATE.running = false;
    }
  }

  async function boot() {
    if (!shared.isTheoryRoute(location.href)) {
      document.body.style.marginRight = "";
      return;
    }
    ensureOverlay();
    await refreshSidebarContext();
    renderStatus("检测到理论题路由", location.pathname);
    await pushRuntime({status: "detected", message: `检测到理论题路由 ${location.pathname}`});
    await shared.sleep(800);
    automateOnce("auto");
  }

  const observer = new MutationObserver(() => {
    if (shared.isTheoryRoute(location.href) && !STATE.running) {
      automateOnce("auto");
    }
  });

  window.addEventListener("load", boot);
  window.addEventListener("popstate", boot);
  observer.observe(document.documentElement, {subtree: true, childList: true});
  boot();
})();
