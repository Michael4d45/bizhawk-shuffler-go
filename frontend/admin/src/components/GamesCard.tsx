import { useState } from "react";
import type { AdminTrigger } from "../adminActions.js";
import { postForm } from "../api.js";
import { countPlayersCompletedGame, countPlayersOnGame } from "../gameStats.js";
import type { GameEntry, ServerState } from "../types.js";
import { CatalogModal } from "./CatalogModal.js";
import { SaveInstancesCard } from "./SaveInstancesCard.js";
import { ActionRow, Badge, Button, Card, EmptyState, FieldLabel, cn } from "./ui.js";

type Props = {
  state: ServerState | null;
  trigger: AdminTrigger;
  pushLog: (msg: string) => void;
  refreshState: () => Promise<ServerState | null>;
};

export function GamesCard({ state, trigger, pushLog, refreshState }: Props) {
  const [romFile, setRomFile] = useState<File | null>(null);
  const [catalogOpen, setCatalogOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const uploadRom = async () => {
    if (!romFile) return;
    const form = new FormData();
    form.append("file", romFile);
    const res = await postForm("/upload", form);
    pushLog(res.ok ? "ROM uploaded" : `upload failed ${res.status}`);
    setRomFile(null);
    await refreshState();
  };

  const toggleGame = async (file: string, enabled: boolean) => {
    const games = new Set(state?.games ?? []);
    if (enabled) games.add(file);
    else games.delete(file);
    await trigger("/api/games", { games: [...games] });
  };

  const enabledCount = (state?.games ?? []).length;
  const catalogCount = (state?.main_games ?? []).length;
  const isSync = state?.mode !== "save";

  return (
    <>
      <Card
        title="Games & ROMs"
        subtitle={
          isSync
            ? `${enabledCount} enabled of ${catalogCount} in catalog`
            : `${(state?.game_instances ?? []).length} save instances`
        }
        actions={
          <ActionRow>
            <Button variant="ghost" onClick={() => setExpanded((e) => !e)}>
              {expanded ? "Collapse" : "Expand"}
            </Button>
            <Button variant="ghost" onClick={() => void trigger("/api/open_roms_folder")}>
              Open ROMs
            </Button>
            {isSync ? (
              <Button variant="ghost" onClick={() => setCatalogOpen(true)}>
                Catalog
              </Button>
            ) : null}
            <Button variant="ghost" onClick={() => void trigger("/api/mode/setup")}>
              Auto setup
            </Button>
          </ActionRow>
        }
      >
        <div className="rounded-lg border border-slate-800 bg-slate-950/50 p-3">
          <FieldLabel>Upload ROM</FieldLabel>
          <ActionRow className="mt-1">
            <input
              type="file"
              className="max-w-full flex-1 text-xs text-slate-400 file:mr-3 file:rounded-md file:border-0 file:bg-slate-700 file:px-2.5 file:py-1.5 file:text-xs file:font-medium file:text-slate-200 hover:file:bg-slate-600"
              onChange={(e) => setRomFile(e.target.files?.[0] ?? null)}
            />
            <Button variant="primary" onClick={() => void uploadRom()} disabled={!romFile}>
              Upload
            </Button>
          </ActionRow>
        </div>

        <div
          className={cn(
            "mt-4 space-y-2",
            !expanded && "max-h-72 overflow-y-auto scrollbar-thin pr-1"
          )}
        >
          {isSync ? (
            (state?.main_games ?? []).length === 0 ? (
              <EmptyState>No games in catalog.</EmptyState>
            ) : (
              (state?.main_games ?? []).map((g) => {
                const enabled = (state?.games ?? []).includes(g.file);
                const onGame = countPlayersOnGame(state, g.file);
                const completed = countPlayersCompletedGame(state, g.file);
                return (
                  <div
                    key={g.file}
                    className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-800 bg-slate-950/40 px-3 py-2"
                  >
                    <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2">
                      <input
                        type="checkbox"
                        className="size-4 rounded border-slate-600 bg-slate-900 text-emerald-600"
                        checked={enabled}
                        onChange={(e) => void toggleGame(g.file, e.target.checked)}
                      />
                      <span className="truncate font-mono text-xs text-slate-200">{g.file}</span>
                      {enabled ? <Badge variant="ok">In rotation</Badge> : null}
                      {onGame > 0 ? (
                        <Badge variant="info">
                          {onGame} player{onGame > 1 ? "s" : ""}
                        </Badge>
                      ) : null}
                      {completed > 0 ? <Badge variant="warn">{completed} completed</Badge> : null}
                    </label>
                    <ActionRow>
                      <Button
                        variant="ghost"
                        onClick={() => void trigger("/api/swap_all_to_game", { game: g.file })}
                      >
                        Swap all
                      </Button>
                      <Button
                        variant="ghost"
                        onClick={() =>
                          void trigger(
                            `/api/games/${encodeURIComponent(g.file)}/mark_completed_all`
                          )
                        }
                      >
                        Mark done
                      </Button>
                    </ActionRow>
                  </div>
                );
              })
            )
          ) : (
            <SaveInstancesCard
              state={state}
              expanded={expanded}
              trigger={trigger}
              pushLog={pushLog}
              refreshState={refreshState}
            />
          )}
        </div>
      </Card>

      <CatalogModal
        open={catalogOpen}
        mainGames={[...(state?.main_games ?? [])]}
        onClose={() => setCatalogOpen(false)}
        onSave={(main_games: GameEntry[]) => void trigger("/api/games", { main_games })}
      />
    </>
  );
}
