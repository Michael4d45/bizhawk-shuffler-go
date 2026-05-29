import { useEffect, useState } from "react";
import { post } from "../api.js";
import type { ServerState } from "../types.js";
import { Modal } from "./Modal.js";
import { Button, FieldLabel } from "./ui.js";

type Props = {
  open: boolean;
  playerName: string;
  state: ServerState | null;
  onClose: () => void;
  onLog: (msg: string) => void;
};

export function ConfigModal({ open, playerName, state, onClose, onLog }: Props) {
  const player = state?.players[playerName];
  const [values, setValues] = useState<Record<string, string>>({});

  useEffect(() => {
    const configKeys = state?.config_keys ?? [];
    if (!open || !player?.config_values) return;
    const next: Record<string, string> = {};
    for (const key of configKeys) {
      const val = player.config_values?.[key];
      if (val === undefined) {
        next[key] = "";
      } else if (typeof val === "object") {
        next[key] = JSON.stringify(val, null, 2);
      } else {
        next[key] = String(val);
      }
    }
    setValues(next);
  }, [open, player?.config_values, state?.config_keys, playerName]);

  const save = async () => {
    if (!playerName) return;
    const configKeys = state?.config_keys ?? [];
    const updates: Record<string, unknown> = {};
    const current = player?.config_values ?? {};
    for (const key of configKeys) {
      const raw = values[key] ?? "";
      let parsed: unknown = raw;
      try {
        parsed = JSON.parse(raw);
      } catch {
        if (raw === "true") parsed = true;
        else if (raw === "false") parsed = false;
        else if (!Number.isNaN(Number(raw)) && raw !== "") parsed = Number(raw);
      }
      if (JSON.stringify(current[key]) !== JSON.stringify(parsed)) {
        updates[key] = parsed;
      }
    }
    if (Object.keys(updates).length === 0) {
      onLog("No config changes detected");
      onClose();
      return;
    }
    if (
      !confirm(
        `Update ${playerName}'s BizHawk config? Restart BizHawk manually for changes to take effect.`
      )
    ) {
      return;
    }
    const res = await post("/api/update_player_config", {
      player: playerName,
      config: JSON.stringify(updates),
    });
    if (res.ok) {
      onLog(`Config update sent to ${playerName}`);
      onClose();
    } else {
      onLog(`Config update failed for ${playerName}`);
    }
  };

  return (
    <Modal
      open={open}
      wide
      title={`BizHawk config: ${playerName}`}
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={(state?.config_keys ?? []).length === 0}
            onClick={() => void save()}
          >
            Save config
          </Button>
        </>
      }
    >
      <p className="mb-3 text-xs text-slate-500">
        Values refresh when the player reports config after Check config. Edit predefined keys only.
      </p>
      {(state?.config_keys ?? []).length === 0 ? (
        <p className="text-sm text-slate-500">No config keys defined on the server.</p>
      ) : (
        <div className="space-y-3">
          {(state?.config_keys ?? []).map((key) => (
            <div key={key} className="rounded-lg border border-slate-800 p-3">
              <FieldLabel>{key}</FieldLabel>
              <textarea
                className="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 font-mono text-xs text-slate-100"
                rows={2}
                value={values[key] ?? ""}
                onChange={(e) =>
                  setValues((v) => ({
                    ...v,
                    [key]: e.target.value,
                  }))
                }
              />
            </div>
          ))}
        </div>
      )}
    </Modal>
  );
}
