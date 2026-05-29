import { useEffect, useState } from "react";
import type { AdminTrigger } from "../adminActions.js";
import { intervalError, intervalValid } from "../intervalUtils.js";
import { SESSION_BUTTONS } from "../sessionButtons.js";
import { intervalDisplay, nextSwapDisplay } from "../swapDisplay.js";
import type { ServerState } from "../types.js";
import { ShareAddresses } from "./ShareAddresses.js";
import { ActionRow, Badge, Button, Card, Divider, FieldLabel, Input, Select } from "./ui.js";

type Props = {
  state: ServerState | null;
  trigger: AdminTrigger;
  nowMs: number;
};

const PRIMARY_PATHS = new Set(["/api/start", "/api/pause", "/api/do_swap"]);

export function SessionCard({ state, trigger, nowMs }: Props) {
  const [intervalMin, setIntervalMin] = useState(5);
  const [intervalMax, setIntervalMax] = useState(10);

  useEffect(() => {
    if (state?.min_interval_secs) setIntervalMin(state.min_interval_secs);
    if (state?.max_interval_secs) setIntervalMax(state.max_interval_secs);
  }, [state?.min_interval_secs, state?.max_interval_secs]);

  const draft = { min: intervalMin, max: intervalMax };
  const err = intervalError(draft);
  const valid = intervalValid(draft);

  const primary = SESSION_BUTTONS.filter((b) => PRIMARY_PATHS.has(b.path));
  const toggles = SESSION_BUTTONS.filter((b) => "toggle" in b);
  const other = SESSION_BUTTONS.filter((b) => !PRIMARY_PATHS.has(b.path) && !("toggle" in b));

  return (
    <Card
      title="Session control"
      subtitle={`Swap timer · ${nextSwapDisplay(state, nowMs)} until next swap`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={state?.running ? "ok" : "err"}>
          {state?.running ? "Running" : "Stopped"}
        </Badge>
        {state?.swap_enabled === false ? <Badge variant="warn">Auto swaps off</Badge> : null}
        {state?.countdown_enabled ? <Badge variant="info">Countdown on</Badge> : null}
      </div>

      <Divider />

      <ShareAddresses />

      <Divider />

      <div className="space-y-2">
        <FieldLabel htmlFor="session-mode">Game mode</FieldLabel>
        <Select
          id="session-mode"
          value={state?.mode ?? "sync"}
          onChange={(e) => void trigger("/api/mode", { mode: e.target.value })}
        >
          <option value="sync">Sync swap (all same game)</option>
          <option value="save">Save swap (per-player saves)</option>
        </Select>
      </div>

      <Divider />

      <p className="mb-2 text-[11px] font-medium uppercase tracking-wide text-slate-500">
        Primary actions
      </p>
      <ActionRow>
        {primary.map((btn) => (
          <Button
            key={btn.path}
            variant={btn.path === "/api/start" ? "primary" : "secondary"}
            onClick={() => void trigger(btn.path)}
          >
            {btn.label}
          </Button>
        ))}
        <Button variant="ghost" onClick={() => void trigger("/api/mode/setup")}>
          Auto setup
        </Button>
      </ActionRow>

      <p className="mb-2 mt-4 text-[11px] font-medium uppercase tracking-wide text-slate-500">
        Toggles
      </p>
      <ActionRow>
        {toggles.map((btn) => {
          const on = state && "toggle" in btn && btn.toggle ? state[btn.toggle] : false;
          return (
            <Button
              key={btn.path}
              variant={on ? "primary" : "ghost"}
              onClick={() => void trigger(btn.path)}
            >
              {btn.label}
              <span className="ml-1.5 opacity-70">{on ? "On" : "Off"}</span>
            </Button>
          );
        })}
      </ActionRow>

      {other.length > 0 ? (
        <>
          <p className="mb-2 mt-4 text-[11px] font-medium uppercase tracking-wide text-slate-500">
            Maintenance
          </p>
          <ActionRow>
            {other.map((btn) => (
              <Button key={btn.path} variant="danger" onClick={() => void trigger(btn.path)}>
                {btn.label}
              </Button>
            ))}
          </ActionRow>
        </>
      ) : null}

      <Divider />

      <p className="mb-1 text-[11px] font-medium uppercase tracking-wide text-slate-500">
        Swap interval (seconds)
      </p>
      <p className="mb-2 font-mono text-xs text-slate-500">Current: {intervalDisplay(state)}</p>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-[1fr_1fr_auto]">
        <div>
          <FieldLabel htmlFor="interval-min">Min</FieldLabel>
          <Input
            id="interval-min"
            type="number"
            min={1}
            value={intervalMin}
            onChange={(e) => setIntervalMin(+e.target.value)}
          />
        </div>
        <div>
          <FieldLabel htmlFor="interval-max">Max</FieldLabel>
          <Input
            id="interval-max"
            type="number"
            min={1}
            value={intervalMax}
            onChange={(e) => setIntervalMax(+e.target.value)}
          />
        </div>
        <div className="flex items-end">
          <Button
            variant="primary"
            className="w-full"
            disabled={!valid}
            onClick={() =>
              void trigger("/api/interval", {
                min_interval_secs: intervalMin,
                max_interval_secs: intervalMax,
              })
            }
          >
            Save
          </Button>
        </div>
      </div>
      {err ? <p className="mt-2 text-xs text-rose-400">{err}</p> : null}
    </Card>
  );
}
