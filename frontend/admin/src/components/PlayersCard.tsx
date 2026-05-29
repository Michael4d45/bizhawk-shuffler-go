import { useState } from "react";
import type { AdminTrigger } from "../adminActions.js";
import {
  addCompletedGame,
  addCompletedInstance,
  post,
  removeCompletedGame,
  removeCompletedInstance,
} from "../api.js";
import { playerCompletionCount } from "../gameStats.js";
import { playerStatusBadge } from "../status.js";
import type { Player, ServerState } from "../types.js";
import { useOptionalPlayerDrag } from "../PlayerDragContext.js";
import { ConfigModal } from "./ConfigModal.js";
import { MessageComposerModal } from "./MessageComposerModal.js";
import { DraggablePlayerChip } from "./DraggablePlayerChip.js";
import { ActionRow, Badge, Button, Card, EmptyState, FieldLabel, Input, Select } from "./ui.js";

type Props = {
  state: ServerState | null;
  trigger: AdminTrigger;
  pushLog: (msg: string) => void;
  refreshState: () => Promise<ServerState | null>;
};

function sortedPlayers(state: ServerState | null): Array<[string, Player]> {
  if (!state?.players) return [];
  return Object.entries(state.players).sort(([a], [b]) => a.localeCompare(b));
}

export function PlayersCard({ state, trigger, pushLog, refreshState }: Props) {
  const [newPlayer, setNewPlayer] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [selections, setSelections] = useState<Record<string, string>>({});
  const [newCompleted, setNewCompleted] = useState<Record<string, string>>({});
  const [messageTarget, setMessageTarget] = useState<
    { type: "player"; player: string } | { type: "all" } | null
  >(null);
  const [configPlayer, setConfigPlayer] = useState<string | null>(null);

  const players = sortedPlayers(state);
  const isSync = state?.mode !== "save";
  const dnd = useOptionalPlayerDrag();

  const swapPlayer = async (name: string) => {
    const sel = selections[name];
    if (!sel) return;
    const body = isSync ? { player: name, game: sel } : { player: name, instance_id: sel };
    await post("/api/swap_player", body);
    await refreshState();
  };

  const openConfig = async (name: string) => {
    setConfigPlayer(name);
    await trigger("/api/check_player_config", { player: name });
  };

  return (
    <>
      <Card
        title="Players"
        subtitle={`${players.length} registered`}
        actions={
          <ActionRow>
            <Button variant="ghost" onClick={() => setMessageTarget({ type: "all" })}>
              Message all
            </Button>
            <Button
              variant="ghost"
              onClick={() => void trigger("/api/players/remove_all_completions")}
            >
              Clear completions
            </Button>
          </ActionRow>
        }
      >
        <ActionRow className="mb-4">
          <div className="min-w-[12rem] flex-1">
            <FieldLabel htmlFor="new-player">Add player</FieldLabel>
            <Input
              id="new-player"
              value={newPlayer}
              onChange={(e) => setNewPlayer(e.target.value)}
              placeholder="Player name"
              onKeyDown={(e) => {
                if (e.key !== "Enter" || !newPlayer.trim()) return;
                void trigger("/api/add_player", { player: newPlayer.trim() });
                setNewPlayer("");
              }}
            />
          </div>
          <div className="flex items-end">
            <Button
              variant="primary"
              onClick={() => {
                if (!newPlayer.trim()) return;
                void trigger("/api/add_player", { player: newPlayer.trim() });
                setNewPlayer("");
              }}
            >
              Add
            </Button>
          </div>
        </ActionRow>

        {players.length === 0 ? (
          <EmptyState>No players yet.</EmptyState>
        ) : (
          <ul className="space-y-2">
            {players.map(([name, p]) => {
              const status = playerStatusBadge(p);
              const showDone = expanded[name];
              const completions = playerCompletionCount(p);
              return (
                <li key={name} className="rounded-lg border border-slate-800 bg-slate-950/40 p-3">
                  <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        {!isSync && dnd ? (
                          <DraggablePlayerChip name={name} dnd={dnd} variant="assigned" />
                        ) : (
                          <span className="font-medium text-slate-100">{name}</span>
                        )}
                        <Badge variant={status.variant}>{status.label}</Badge>
                        <Badge variant={p.has_files ? "ok" : "warn"}>
                          {p.has_files ? "Has files" : "Missing files"}
                        </Badge>
                        {completions > 0 ? (
                          <Badge variant="neutral">{completions} completed</Badge>
                        ) : null}
                        {p.ping_ms != null ? (
                          <span className="font-mono text-[11px] text-slate-500">
                            {p.ping_ms}ms
                          </span>
                        ) : null}
                      </div>
                      <p className="mt-1 text-xs text-slate-500">
                        {p.game ?? "—"}
                        {p.instance_id ? ` · ${p.instance_id}` : ""}
                      </p>
                      <Button
                        variant="ghost"
                        className="mt-2"
                        onClick={() => setExpanded((e) => ({ ...e, [name]: !e[name] }))}
                      >
                        {showDone ? "Hide completed" : "Completed"}
                      </Button>
                      {showDone ? (
                        <div className="mt-2 space-y-2 rounded border border-slate-800 p-2 text-xs">
                          <div>
                            <p className="text-slate-500">Completed games</p>
                            <div className="mt-1 flex flex-wrap gap-1">
                              {(p.completed_games ?? []).map((g) => (
                                <span
                                  key={g}
                                  className="inline-flex items-center gap-1 rounded bg-rose-950/40 px-2 py-0.5 text-rose-300"
                                >
                                  {g}
                                  <button
                                    type="button"
                                    className="text-rose-400 hover:text-rose-200"
                                    onClick={() =>
                                      void removeCompletedGame(name, g).then(() => refreshState())
                                    }
                                  >
                                    ×
                                  </button>
                                </span>
                              ))}
                            </div>
                            <ActionRow className="mt-1">
                              <Select
                                value={newCompleted[`g-${name}`] ?? ""}
                                onChange={(e) =>
                                  setNewCompleted((c) => ({
                                    ...c,
                                    [`g-${name}`]: e.target.value,
                                  }))
                                }
                              >
                                <option value="">— game —</option>
                                {(state?.games ?? []).map((g) => (
                                  <option key={g} value={g}>
                                    {g}
                                  </option>
                                ))}
                              </Select>
                              <Button
                                variant="ghost"
                                onClick={() => {
                                  const g = newCompleted[`g-${name}`];
                                  if (!g) return;
                                  void addCompletedGame(name, g).then(() => refreshState());
                                }}
                              >
                                Add
                              </Button>
                            </ActionRow>
                          </div>
                          <div>
                            <p className="text-slate-500">Completed instances</p>
                            <div className="mt-1 flex flex-wrap gap-1">
                              {(p.completed_instances ?? []).map((inst) => (
                                <span
                                  key={inst}
                                  className="inline-flex items-center gap-1 rounded bg-rose-950/40 px-2 py-0.5 text-rose-300"
                                >
                                  {inst}
                                  <button
                                    type="button"
                                    className="text-rose-400 hover:text-rose-200"
                                    onClick={() =>
                                      void removeCompletedInstance(name, inst).then(() =>
                                        refreshState()
                                      )
                                    }
                                  >
                                    ×
                                  </button>
                                </span>
                              ))}
                            </div>
                            <ActionRow className="mt-1">
                              <Select
                                value={newCompleted[`i-${name}`] ?? ""}
                                onChange={(e) =>
                                  setNewCompleted((c) => ({
                                    ...c,
                                    [`i-${name}`]: e.target.value,
                                  }))
                                }
                              >
                                <option value="">— instance —</option>
                                {(state?.game_instances ?? []).map((i) => (
                                  <option key={i.id} value={i.id}>
                                    {i.id} — {i.game}
                                  </option>
                                ))}
                              </Select>
                              <Button
                                variant="ghost"
                                onClick={() => {
                                  const inst = newCompleted[`i-${name}`];
                                  if (!inst) return;
                                  void addCompletedInstance(name, inst).then(() => refreshState());
                                }}
                              >
                                Add
                              </Button>
                            </ActionRow>
                          </div>
                        </div>
                      ) : null}
                    </div>
                    <ActionRow className="lg:flex-col lg:items-stretch">
                      <Select
                        value={selections[name] ?? ""}
                        onChange={(e) => setSelections((s) => ({ ...s, [name]: e.target.value }))}
                      >
                        <option value="">
                          {isSync ? "— select game —" : "— select instance —"}
                        </option>
                        {isSync
                          ? (state?.games ?? []).map((g) => (
                              <option key={g} value={g}>
                                {g}
                              </option>
                            ))
                          : (state?.game_instances ?? []).map((i) => (
                              <option key={i.id} value={i.id}>
                                {i.id} — {i.game}
                              </option>
                            ))}
                      </Select>
                      <Button
                        variant="secondary"
                        disabled={!selections[name]}
                        onClick={() => void swapPlayer(name)}
                      >
                        Swap
                      </Button>
                      <Button
                        variant="ghost"
                        onClick={() => void trigger("/api/random_swap", { player: name })}
                      >
                        Random
                      </Button>
                      <Button
                        variant="ghost"
                        onClick={() => setMessageTarget({ type: "player", player: name })}
                      >
                        Message
                      </Button>
                      <Button
                        variant="ghost"
                        onClick={() => void trigger("/api/fullscreen_toggle", { player: name })}
                      >
                        Fullscreen
                      </Button>
                      <Button variant="ghost" onClick={() => void openConfig(name)}>
                        Config
                      </Button>
                      <Button
                        variant="danger"
                        onClick={() => void trigger("/api/remove_player", { player: name })}
                      >
                        Remove
                      </Button>
                    </ActionRow>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Card>

      <MessageComposerModal
        open={messageTarget !== null}
        target={messageTarget}
        onClose={() => setMessageTarget(null)}
        onSent={(msg) => pushLog(msg)}
      />
      <ConfigModal
        open={configPlayer !== null}
        playerName={configPlayer ?? ""}
        state={state}
        onClose={() => setConfigPlayer(null)}
        onLog={pushLog}
      />
    </>
  );
}
