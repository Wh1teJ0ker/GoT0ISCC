const DEFAULT_SETTINGS = {
  enabled: true,
  autoStart: true,
  autoSubmit: true,
  localConfidenceThreshold: 0.86,
  aiConfidenceThreshold: 0.8,
  loginCooldownMs: 3000,
  fetchCooldownMs: 2000,
  submitCooldownMs: 4000,
  questionCooldownMs: 5000,
  aiEnabled: false,
  aiBaseURL: "https://api.openai.com/v1",
  aiAPIKey: "",
  aiModel: "gpt-5.4",
  aiReasoningEffort: "medium",
  aiPrompt: "请结合当前题目、选项、以及本地题库候选结果，判断最可能的正确答案。只返回 JSON，字段包含 recommended_options、recommended_texts、confidence、reason。"
};

const DEFAULT_BANK_PATH = "assets/theory-bank.current.json";

async function getSettings() {
  const result = await chrome.storage.local.get(["theoryPilotSettings"]);
  return {...DEFAULT_SETTINGS, ...(result.theoryPilotSettings || {})};
}

async function setSettings(next) {
  await chrome.storage.local.set({theoryPilotSettings: {...DEFAULT_SETTINGS, ...next}});
}

async function getBank() {
  return ensureBankLoaded();
}

async function setBank(bank) {
  await chrome.storage.local.set({theoryPilotBank: bank});
}

async function loadBundledBank() {
  try {
    const response = await fetch(chrome.runtime.getURL(DEFAULT_BANK_PATH), {cache: "no-store"});
    if (!response.ok) {
      return {items: [], meta: {source: "bundled-missing"}};
    }
    const payload = await response.json();
    if (!payload || !Array.isArray(payload.items)) {
      return {items: [], meta: {source: "bundled-invalid"}};
    }
    return payload;
  } catch (_error) {
    return {items: [], meta: {source: "bundled-error"}};
  }
}

async function ensureBankLoaded() {
  const result = await chrome.storage.local.get(["theoryPilotBank"]);
  const stored = result.theoryPilotBank;
  const bundled = await loadBundledBank();
  const bundledSignature = bundled?.meta?.signature || "";
  const storedSignature = stored?.meta?.signature || "";
  if (!stored || !Array.isArray(stored.items) || (!stored.items.length && bundled.items.length) || (bundledSignature && bundledSignature !== storedSignature)) {
    await setBank(bundled);
    return bundled;
  }
  return stored;
}

async function getRuntime(tabId) {
  const result = await chrome.storage.session.get([`runtime:${tabId}`]);
  return result[`runtime:${tabId}`] || null;
}

async function setRuntime(tabId, runtime) {
  await chrome.storage.session.set({[`runtime:${tabId}`]: runtime});
}

async function clearRuntime(tabId) {
  await chrome.storage.session.remove([`runtime:${tabId}`]);
}

async function withActiveTab(handler) {
  const tabs = await chrome.tabs.query({active: true, currentWindow: true});
  const tab = tabs[0];
  if (!tab || !tab.id) {
    throw new Error("当前没有可用标签页");
  }
  return handler(tab);
}

chrome.runtime.onInstalled.addListener(async () => {
  await setSettings(await getSettings());
  await ensureBankLoaded();
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    switch (message?.type) {
      case "theory-pilot:get-settings":
        sendResponse({ok: true, data: await getSettings()});
        return;
      case "theory-pilot:set-settings":
        await setSettings(message.payload || {});
        sendResponse({ok: true, data: await getSettings()});
        return;
      case "theory-pilot:get-bank":
        sendResponse({ok: true, data: await getBank()});
        return;
      case "theory-pilot:import-bank":
        await setBank(message.payload || {items: [], meta: {}});
        sendResponse({ok: true});
        return;
      case "theory-pilot:get-runtime":
        sendResponse({ok: true, data: await withActiveTab((tab) => getRuntime(tab.id))});
        return;
      case "theory-pilot:clear-runtime":
        await withActiveTab((tab) => clearRuntime(tab.id));
        sendResponse({ok: true});
        return;
      case "theory-pilot:update-runtime":
        if (sender.tab?.id) {
          await setRuntime(sender.tab.id, message.payload || null);
        }
        sendResponse({ok: true});
        return;
      case "theory-pilot:ai-decision":
        sendResponse({ok: true, data: await requestAIDecision(message.payload || {})});
        return;
      default:
        sendResponse({ok: false, error: "unsupported"});
    }
  })().catch((error) => {
    sendResponse({ok: false, error: error?.message || String(error)});
  });
  return true;
});

async function requestAIDecision(payload) {
  const settings = await getSettings();
  if (!settings.aiEnabled || !String(settings.aiAPIKey || "").trim()) {
    throw new Error("AI 未启用或 API Key 未配置");
  }
  const response = await fetch(`${String(settings.aiBaseURL || "").replace(/\/$/, "")}/responses`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${settings.aiAPIKey}`
    },
    body: JSON.stringify({
      model: settings.aiModel,
      reasoning: {effort: settings.aiReasoningEffort || "medium"},
      input: [
        {
          role: "system",
          content: [{type: "input_text", text: settings.aiPrompt}]
        },
        {
          role: "user",
          content: [{type: "input_text", text: JSON.stringify(payload, null, 2)}]
        }
      ],
      text: {
        format: {
          type: "json_schema",
          name: "theory_answer",
          schema: {
            type: "object",
            properties: {
              recommended_options: {
                type: "array",
                items: {type: "string"}
              },
              recommended_texts: {
                type: "array",
                items: {type: "string"}
              },
              confidence: {type: "number"},
              reason: {type: "string"}
            },
            required: ["recommended_options", "recommended_texts", "confidence", "reason"],
            additionalProperties: false
          }
        }
      }
    })
  });
  if (!response.ok) {
    throw new Error(`AI 请求失败: ${response.status}`);
  }
  const result = await response.json();
  const text = (result.output || [])
    .flatMap((item) => item.content || [])
    .filter((item) => item.type === "output_text" || item.type === "text")
    .map((item) => item.text || "")
    .join("\n")
    .trim();
  if (!text) {
    throw new Error("AI 未返回可解析内容");
  }
  return JSON.parse(text);
}
