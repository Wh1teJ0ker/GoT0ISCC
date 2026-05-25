import {useEffect, useMemo, useState} from "react";
import {HashRouter, Navigate, Route, Routes} from "react-router-dom";
import {Health, Overview, SandboxProfiles} from "../wailsjs/go/desktop/API";
import {MainLayout} from "./components/layout/MainLayout";
import {AccountsPage} from "./pages/AccountsPage";
import {ArenaPage} from "./pages/ArenaPage";
import {CombatPage} from "./pages/CombatPage";
import {Dashboard} from "./pages/Dashboard";
import {LogsPage} from "./pages/LogsPage";
import {PracticePage} from "./pages/PracticePage";
import {TasksPage} from "./pages/TasksPage";
import {TheoryAIPage} from "./pages/theory/TheoryAIPage";
import {TheoryAutomationPage} from "./pages/theory/TheoryAutomationPage";
import {TheoryBankPage} from "./pages/theory/TheoryBankPage";
import {WriteupsPage} from "./pages/WriteupsPage";
import {callWails} from "./lib/wails";

function App() {
  const [health, setHealth] = useState(null);
  const [overview, setOverview] = useState(null);
  const [profiles, setProfiles] = useState([]);

  useEffect(() => {
    Promise.all([
      callWails(() => Health(), {
        status: "unknown",
        message: "",
      }),
      callWails(() => Overview(), null),
      callWails(() => SandboxProfiles(), []),
    ])
      .then(([nextHealth, nextOverview, nextProfiles]) => {
        setHealth(nextHealth);
        setOverview(nextOverview);
        setProfiles(nextProfiles || []);
      })
      .catch((err) => {
        setHealth({
          status: "error",
          message: String(err),
        });
      });
  }, []);

  const activeProfile = useMemo(() => profiles[0] || null, [profiles]);

  return (
    <HashRouter>
      <Routes>
        <Route element={<MainLayout health={health} />}>
          <Route
            index
            element={<Dashboard overview={overview} health={health} activeProfile={activeProfile} />}
          />
          <Route path="accounts" element={<AccountsPage />} />
          <Route path="writeups" element={<WriteupsPage />} />
          <Route path="tasks" element={<TasksPage />} />
          <Route path="practice" element={<PracticePage />} />
          <Route path="arena" element={<ArenaPage />} />
          <Route path="theory" element={<Navigate to="/theory/automation" replace />} />
          <Route path="theory/automation" element={<TheoryAutomationPage />} />
          <Route path="theory/bank" element={<TheoryBankPage />} />
          <Route path="theory/ai" element={<TheoryAIPage />} />
          <Route path="combat" element={<CombatPage />} />
          <Route path="logs" element={<LogsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </HashRouter>
  );
}

export default App;
