import {useEffect, useMemo, useState} from "react";
import {
  StartTheoryAIReview,
  StartTheoryAutomation,
  SaveTheoryAISettings,
  SaveTheoryReview,
  SearchTheoryBank,
  SubmitTheoryManual,
  StopTheoryAutomation,
  StopTheoryAIReview,
  TestTheoryAISettings,
  TheoryAutomationStatus,
  TheoryAISettings,
  TheoryAIReviewStatus,
  TheoryReviewItems,
  TheoryTrack,
  TheoryTrackWithRequest,
} from "../../../wailsjs/go/desktop/API";
import {collectAnswerKeys, collectAnswerTexts, createReviewDraft} from "../../components/theory/utils";

const emptyAutomationForm = {
  max_questions: 20,
  allow_ai: true,
  stop_on_no_answer: true,
};

function buildAIAvailabilityFromSettings(payload) {
  if (!payload?.settings?.enabled) {
    return {
      ok: false,
      status: "disabled",
      model: payload?.settings?.model || "gpt-5.4",
      base_url: payload?.settings?.base_url || "",
      latency_ms: 0,
      http_status_code: 0,
      message: "AI 判题未启用",
      checked_at: "",
    };
  }
  if (!payload?.ready || !payload?.settings) {
    return {
      ok: false,
      status: "not_ready",
      model: payload?.settings?.model || "gpt-5.4",
      base_url: payload?.settings?.base_url || "",
      latency_ms: 0,
      http_status_code: 0,
      message: "AI 配置未就绪",
      checked_at: "",
    };
  }
  return null;
}

export function useTheoryWorkspace() {
  const [payload, setPayload] = useState(null);
  const [loadingCount, setLoadingCount] = useState(0);
  const [error, setError] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [searchResult, setSearchResult] = useState(null);
  const [searchError, setSearchError] = useState("");
  const [aiSettings, setAISettings] = useState(null);
  const [aiDraft, setAIDraft] = useState(null);
  const [savingAI, setSavingAI] = useState(false);
  const [aiError, setAIError] = useState("");
  const [testingAI, setTestingAI] = useState(false);
  const [aiAvailability, setAIAvailability] = useState(null);
  const [reviewState, setReviewState] = useState({summary: null, items: []});
  const [selectedReviewID, setSelectedReviewID] = useState(0);
  const [reviewDraft, setReviewDraft] = useState(null);
  const [savingReview, setSavingReview] = useState(false);
  const [reviewError, setReviewError] = useState("");
  const [automationForm, setAutomationForm] = useState(emptyAutomationForm);
  const [automationStatus, setAutomationStatus] = useState(null);
  const [runningAutomation, setRunningAutomation] = useState(false);
  const [automationResult, setAutomationResult] = useState(null);
  const [automationError, setAutomationError] = useState("");
  const [manualSubmitOptions, setManualSubmitOptions] = useState([]);
  const [manualSubmitBusy, setManualSubmitBusy] = useState(false);
  const [manualSubmitResult, setManualSubmitResult] = useState(null);
  const [manualSubmitError, setManualSubmitError] = useState("");
  const [aiReviewStatus, setAIReviewStatus] = useState(null);
  const [aiReviewForm, setAIReviewForm] = useState({
    limit: 0,
    batch_size: 12,
    timeout_seconds: 180,
    dry_run: false,
    only_pending: true,
    reasoning_effort: "high",
  });
  const [aiReviewBusy, setAIReviewBusy] = useState(false);
  const [aiReviewError, setAIReviewError] = useState("");
  const [selectedAccount, setSelectedAccount] = useState("");
  const loading = loadingCount > 0;

  function startLoading() {
    setLoadingCount((current) => current + 1);
  }

  function stopLoading() {
    setLoadingCount((current) => (current > 0 ? current - 1 : 0));
  }

  function applyTrackResult(result) {
    setPayload(result || null);
    if (result?.selected_account) {
      setSelectedAccount(result.selected_account);
    }
    if (result?.review_dashboard) {
      setReviewState({
        summary: result.review_dashboard || null,
        items: result.review_items || [],
      });
    }
  }

  function clearQuestionForAccount(account, message) {
    setPayload((current) => {
      if (!current) {
        return current;
      }
      const nextAccount = account || current.selected_account || selectedAccount;
      return {
        ...current,
        account: nextAccount,
        selected_account: nextAccount,
        question: {},
        match: {},
        ai: {},
        answer_form: {},
        cache_status: {
          ...(current.cache_status || {}),
          has_snapshot: false,
          last_remote_error: message,
          source: "remote-error",
        },
      };
    });
  }

  async function syncAIAvailability(settingsPayload, options = {}) {
    const {testAvailability = false} = options;
    const presetAvailability = buildAIAvailabilityFromSettings(settingsPayload);
    if (presetAvailability) {
      setAIAvailability(presetAvailability);
      return presetAvailability;
    }
    if (!testAvailability || !settingsPayload?.settings) {
      const unknownAvailability = {
        ok: false,
        status: "unknown",
        model: settingsPayload?.settings?.model || "gpt-5.4",
        base_url: settingsPayload?.settings?.base_url || "",
        latency_ms: 0,
        http_status_code: 0,
        message: "未检测",
        checked_at: "",
      };
      setAIAvailability(unknownAvailability);
      return unknownAvailability;
    }
    const availability = await TestTheoryAISettings(settingsPayload.settings);
    setAIAvailability(availability || null);
    return availability || null;
  }

  useEffect(() => {
    reload();
    loadAISettings({testAvailability: false});
    loadReviewItems();
    loadAIReviewStatus();
    loadAutomationStatus();
  }, []);

  useEffect(() => {
    if (!aiReviewStatus?.running && !runningAutomation) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      loadAIReviewStatus();
      loadAutomationStatus();
      loadReviewItems();
      reload();
    }, runningAutomation ? 1000 : 3000);
    return () => window.clearInterval(timer);
  }, [aiReviewStatus?.running, runningAutomation]);

  useEffect(() => {
    const title = payload?.question?.title;
    if (!title) {
      return;
    }
    setSearchQuery(title);
    runSearch(title);
  }, [payload?.question?.title]);

  useEffect(() => {
    const items = reviewState?.items || [];
    if (!items.length) {
      setSelectedReviewID(0);
      setReviewDraft(null);
      return;
    }
    const current = items.find((item) => item.id === selectedReviewID) || items[0];
    setSelectedReviewID(current.id);
    setReviewDraft(createReviewDraft(current));
  }, [reviewState]);

  const match = payload?.match;
  const question = payload?.question;
  const stats = payload?.statistics;
  const ai = payload?.ai;
  const reviewDashboard = reviewState?.summary || payload?.review_dashboard;
  const reviewItems = reviewState?.items?.length ? reviewState.items : (payload?.review_items || []);
  const selectedReview = useMemo(
    () => reviewItems.find((item) => item.id === selectedReviewID) || null,
    [reviewItems, selectedReviewID],
  );

  const recommendedOptions = useMemo(() => {
    if (match?.recommended_options?.length) {
      return match.recommended_options;
    }
    if (match?.recommended_option) {
      return [match.recommended_option];
    }
    return [];
  }, [match?.recommended_option, match?.recommended_options]);

  const aiRecommendedOptions = ai?.recommended_options || [];

  useEffect(() => {
    setManualSubmitOptions([]);
  }, [payload?.question?.number, payload?.question?.title]);

  async function reload() {
    startLoading();
    setError("");
    try {
      const result = await TheoryTrackWithRequest({
        account: selectedAccount,
        refresh: false,
      });
      applyTrackResult(result);
    } catch (err) {
      setError(String(err));
    } finally {
      stopLoading();
    }
  }

  async function refreshBankData() {
    startLoading();
    setError("");
    setReviewError("");
    setAIReviewError("");
    try {
      const [trackResult, reviewResult, aiStatusResult] = await Promise.all([
        TheoryTrackWithRequest({
          account: selectedAccount,
          refresh: false,
        }),
        TheoryReviewItems(),
        TheoryAIReviewStatus(),
      ]);
      applyTrackResult(trackResult);
      setReviewState({
        summary: reviewResult?.summary || null,
        items: reviewResult?.items || [],
      });
      setAIReviewStatus(aiStatusResult || null);
    } catch (err) {
      setError(String(err));
    } finally {
      stopLoading();
    }
  }

  async function refreshAutomationData() {
    startLoading();
    setError("");
    setReviewError("");
    setAutomationError("");
    try {
      const [trackResult, reviewResult, automationStatusResult] = await Promise.all([
        TheoryTrackWithRequest({
          account: selectedAccount,
          refresh: false,
        }),
        TheoryReviewItems(),
        TheoryAutomationStatus(),
      ]);
      applyTrackResult(trackResult);
      setReviewState({
        summary: reviewResult?.summary || null,
        items: reviewResult?.items || [],
      });
      setAutomationStatus(automationStatusResult || null);
      setAutomationResult(automationStatusResult?.result || null);
      setRunningAutomation(Boolean(automationStatusResult?.running));
    } catch (err) {
      setError(String(err));
    } finally {
      stopLoading();
    }
  }

  async function refreshAIPageData() {
    startLoading();
    setError("");
    setAIError("");
    try {
      const [trackResult, settingsResult] = await Promise.all([
        TheoryTrackWithRequest({
          account: selectedAccount,
          refresh: false,
        }),
        TheoryAISettings(),
      ]);
      applyTrackResult(trackResult);
      setAISettings(settingsResult || null);
      setAIDraft(settingsResult?.settings ? {...settingsResult.settings} : null);
      await syncAIAvailability(settingsResult, {testAvailability: true});
    } catch (err) {
      setAIError(String(err));
    } finally {
      stopLoading();
    }
  }

  async function refreshTheoryRemote(account = selectedAccount) {
    startLoading();
    setError("");
    try {
      const result = await TheoryTrackWithRequest({
        account,
        refresh: true,
      });
      applyTrackResult(result);
    } catch (err) {
      const message = String(err);
      setError(message);
      clearQuestionForAccount(account, message);
    } finally {
      stopLoading();
    }
  }

  async function loadTheoryAccount(account) {
    startLoading();
    setError("");
    try {
      const result = await TheoryTrackWithRequest({
        account,
        refresh: false,
      });
      applyTrackResult(result);
    } catch (err) {
      const message = String(err);
      setError(message);
      clearQuestionForAccount(account, message);
    } finally {
      stopLoading();
    }
  }

  async function loadReviewItems() {
    setReviewError("");
    try {
      const result = await TheoryReviewItems();
      setReviewState({
        summary: result?.summary || null,
        items: result?.items || [],
      });
    } catch (err) {
      setReviewError(String(err));
    }
  }

  async function runSearch(query = searchQuery) {
    const next = String(query || "").trim();
    setSearchQuery(next);
    if (!next) {
      setSearchResult(null);
      setSearchError("");
      return;
    }
    setSearching(true);
    setSearchError("");
    try {
      const result = await SearchTheoryBank(next);
      setSearchResult(result || null);
    } catch (err) {
      setSearchError(String(err));
    } finally {
      setSearching(false);
    }
  }

  async function loadAISettings(options = {}) {
    setAIError("");
    try {
      const result = await TheoryAISettings();
      setAISettings(result || null);
      setAIDraft(result?.settings ? {...result.settings} : null);
      await syncAIAvailability(result, options);
    } catch (err) {
      setAIError(String(err));
      setAIAvailability(null);
    }
  }

  async function loadAIReviewStatus() {
    setAIReviewError("");
    try {
      const result = await TheoryAIReviewStatus();
      setAIReviewStatus(result || null);
    } catch (err) {
      setAIReviewError(String(err));
    }
  }

  async function loadAutomationStatus() {
    setAutomationError("");
    try {
      const result = await TheoryAutomationStatus();
      setAutomationStatus(result || null);
      setAutomationResult(result?.result || null);
      setRunningAutomation(Boolean(result?.running));
      if (runningAutomation && !result?.running) {
        await reload();
      }
      return result || null;
    } catch (err) {
      setAutomationError(String(err));
      return null;
    }
  }

  async function saveAISettings() {
    if (!aiDraft) {
      return;
    }
    setSavingAI(true);
    setAIError("");
    try {
      const result = await SaveTheoryAISettings(aiDraft);
      setAISettings(result || null);
      setAIDraft(result?.settings ? {...result.settings} : null);
      await Promise.all([reload(), syncAIAvailability(result, {testAvailability: true})]);
    } catch (err) {
      setAIError(String(err));
    } finally {
      setSavingAI(false);
    }
  }

  async function testAISettings() {
    if (!aiDraft) {
      return;
    }
    setTestingAI(true);
    setAIError("");
    try {
      const result = await TestTheoryAISettings(aiDraft);
      setAIAvailability(result || null);
    } catch (err) {
      setAIError(String(err));
      setAIAvailability(null);
    } finally {
      setTestingAI(false);
    }
  }

  async function runAutomation() {
    setAutomationError("");
    try {
      const result = await StartTheoryAutomation({
        ...automationForm,
        allow_ai: true,
        stop_on_no_answer: true,
        account: selectedAccount,
      });
      setAutomationStatus(result || null);
      setRunningAutomation(Boolean(result?.running));
      setAutomationResult(result?.result || null);
      await Promise.all([loadReviewItems(), loadAutomationStatus()]);
    } catch (err) {
      setAutomationError(String(err));
    }
  }

  async function stopAutomation() {
    setAutomationError("");
    try {
      const result = await StopTheoryAutomation();
      setAutomationStatus(result || null);
      setRunningAutomation(Boolean(result?.running));
      setAutomationResult(result?.result || null);
      await Promise.all([refreshTheoryRemote(selectedAccount), loadReviewItems(), loadAutomationStatus()]);
    } catch (err) {
      setAutomationError(String(err));
    }
  }

  function toggleManualSubmitOption(option) {
    const key = String(option || "").trim().toUpperCase();
    if (!key) {
      return;
    }
    setManualSubmitOptions((current) => {
      const exists = current.includes(key);
      if (question?.selection_type !== "multiple" && !payload?.answer_form?.allows_multiple) {
        return exists ? [] : [key];
      }
      if (exists) {
        return current.filter((item) => item !== key);
      }
      return [...current, key].sort();
    });
  }

  async function submitTheoryManual() {
    setManualSubmitError("");
    setManualSubmitResult(null);
    if (!manualSubmitOptions.length) {
      setManualSubmitError("请至少选择一个答案");
      return;
    }
    setManualSubmitBusy(true);
    try {
      const result = await SubmitTheoryManual({
        account: selectedAccount,
        options: manualSubmitOptions,
      });
      setManualSubmitResult(result || null);
      if (result?.payload) {
        applyTrackResult(result.payload);
      } else {
        await reload();
      }
      setManualSubmitOptions([]);
      await Promise.all([loadReviewItems(), loadAutomationStatus()]);
    } catch (err) {
      setManualSubmitError(String(err));
    } finally {
      setManualSubmitBusy(false);
    }
  }

  async function saveReview() {
    if (!reviewDraft?.id) {
      return;
    }
    setSavingReview(true);
    setReviewError("");
    try {
      const result = await SaveTheoryReview({
        id: reviewDraft.id,
        question: reviewDraft.question,
        selection_type: reviewDraft.selection_type,
        options: (reviewDraft.options || []).map((item) => ({
          key: item.key,
          content: item.content,
          input_type: item.input_type || "",
          is_correct: Boolean(item.is_correct),
        })),
        answer_keys: collectAnswerKeys(reviewDraft.options),
        answer_texts: collectAnswerTexts(reviewDraft.options),
        review_status: reviewDraft.review_status,
        review_reason: reviewDraft.review_reason,
      });
      const nextItems = (reviewState.items || []).map((item) => (item.id === result?.id ? result : item));
      setReviewState((current) => ({...current, items: nextItems}));
      setReviewDraft(createReviewDraft(result));
      await Promise.all([reload(), loadReviewItems()]);
    } catch (err) {
      setReviewError(String(err));
    } finally {
      setSavingReview(false);
    }
  }

  async function startAIReview() {
    setAIReviewBusy(true);
    setAIReviewError("");
    try {
      const result = await StartTheoryAIReview(aiReviewForm);
      setAIReviewStatus(result || null);
      await Promise.all([loadReviewItems(), reload()]);
    } catch (err) {
      setAIReviewError(String(err));
    } finally {
      setAIReviewBusy(false);
    }
  }

  async function stopAIReview() {
    setAIReviewBusy(true);
    setAIReviewError("");
    try {
      const result = await StopTheoryAIReview();
      setAIReviewStatus(result || null);
      await Promise.all([loadReviewItems(), reload()]);
    } catch (err) {
      setAIReviewError(String(err));
    } finally {
      setAIReviewBusy(false);
    }
  }

  function updateReviewOption(index, patch) {
    setReviewDraft((current) => {
      const options = [...(current?.options || [])];
      options[index] = {...options[index], ...patch};
      return {...current, options};
    });
  }

  return {
    payload,
    loading,
    error,
    searchQuery,
    setSearchQuery,
    searching,
    searchResult,
    searchError,
    aiSettings,
    aiDraft,
    setAIDraft,
    savingAI,
    aiError,
    testingAI,
    aiAvailability,
    reviewState,
    selectedReviewID,
    setSelectedReviewID,
    reviewDraft,
    setReviewDraft,
    savingReview,
    reviewError,
    automationForm,
    setAutomationForm,
    automationStatus,
    runningAutomation,
    automationResult,
    automationError,
    manualSubmitOptions,
    manualSubmitBusy,
    manualSubmitResult,
    manualSubmitError,
    selectedAccount,
    setSelectedAccount,
    aiReviewStatus,
    aiReviewForm,
    setAIReviewForm,
    aiReviewBusy,
    aiReviewError,
    match,
    question,
    stats,
    ai,
    reviewDashboard,
    reviewItems,
    selectedReview,
    recommendedOptions,
    aiRecommendedOptions,
    reload,
    refreshBankData,
    refreshAutomationData,
    refreshAIPageData,
    loadTheoryAccount,
    refreshTheoryRemote,
    runSearch,
    loadAISettings,
    loadAIReviewStatus,
    loadAutomationStatus,
    loadReviewItems,
    saveAISettings,
    testAISettings,
    startAIReview,
    stopAIReview,
    runAutomation,
    stopAutomation,
    toggleManualSubmitOption,
    submitTheoryManual,
    saveReview,
    updateReviewOption,
  };
}
