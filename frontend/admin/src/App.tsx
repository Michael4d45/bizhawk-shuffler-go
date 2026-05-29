import { GamesCard } from "./components/GamesCard.js";
import { LogsCard } from "./components/LogsCard.js";
import { PlayersCard } from "./components/PlayersCard.js";
import { PluginsCard } from "./components/PluginsCard.js";
import { SessionCard } from "./components/SessionCard.js";
import { Badge, Button, cn } from "./components/ui.js";
import { formatUpdatedAt, playerCounts } from "./status.js";
import {
  nextSwapDisplay,
  swapProgress,
  swapProgressUrgent,
  swapTimerActive,
} from "./swapDisplay.js";
import { useAdmin } from "./useAdmin.js";
import { useNowMs } from "./useNowMs.js";

export function App() {
  const { state, log, pushLog, wsConnected, refreshState, trigger } = useAdmin();
  const counts = playerCounts(state);
  const timerActive = swapTimerActive(state);
  const now = useNowMs(timerActive);
  const progress = swapProgress(state, now);
  const urgent = swapProgressUrgent(state, now);

  return (
    <div className="min-h-screen">
      <div
        className={cn(
          "fixed inset-x-0 top-0 z-50 h-1 origin-left shadow-[0_0_12px_rgba(16,185,129,0.5)] will-change-transform",
          urgent ? "bg-amber-400 shadow-amber-500/40" : "bg-emerald-500"
        )}
        style={{ transform: `scaleX(${progress / 100})` }}
        aria-hidden
      />

      <header className="sticky top-0 z-40 border-b border-slate-800/80 bg-slate-950/90 backdrop-blur-md">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-6">
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-emerald-500/90">
              BizShuffle
            </p>
            <h1 className="text-lg font-semibold tracking-tight text-white">Admin Console</h1>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={state?.running ? "ok" : "err"}>
              {state?.running ? "Session running" : "Session stopped"}
            </Badge>
            <Badge variant={wsConnected ? "ok" : "err"}>
              WebSocket {wsConnected ? "live" : "offline"}
            </Badge>
            <Badge variant="info">Mode {state?.mode ?? "sync"}</Badge>
            <Badge variant="neutral">Next swap {nextSwapDisplay(state, now)}</Badge>
            <Badge variant="neutral">
              Players {counts.ready}/{counts.online}/{counts.total}
            </Badge>
          </div>

          <div className="flex items-center gap-2">
            <span className="hidden text-xs text-slate-500 sm:inline">
              Updated {formatUpdatedAt(state?.updated_at)}
            </span>
            <Button variant="ghost" onClick={() => void refreshState()}>
              Refresh
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl space-y-4 px-4 py-5 sm:px-6">
        <div className="grid gap-4 lg:grid-cols-2">
          <SessionCard state={state} trigger={trigger} nowMs={now} />
          <GamesCard
            state={state}
            trigger={trigger}
            pushLog={pushLog}
            refreshState={refreshState}
          />
        </div>

        <PlayersCard
          state={state}
          trigger={trigger}
          pushLog={pushLog}
          refreshState={refreshState}
        />

        <div className="grid gap-4 lg:grid-cols-2">
          <PluginsCard trigger={trigger} pushLog={pushLog} />
          <LogsCard log={log} />
        </div>
      </main>
    </div>
  );
}
