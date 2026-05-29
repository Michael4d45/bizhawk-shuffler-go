import { useEffect, useState } from "react";
import type { GameEntry } from "../types.js";
import { useFilesList } from "../hooks/useFilesList.js";
import { Modal } from "./Modal.js";
import { Button, FieldLabel, Select } from "./ui.js";

type Props = {
  open: boolean;
  mainGames: GameEntry[];
  onClose: () => void;
  onSave: (games: GameEntry[]) => void;
};

export function CatalogModal({ open, mainGames, onClose, onSave }: Props) {
  const { files, loading, refresh } = useFilesList();
  const [entries, setEntries] = useState<GameEntry[]>([]);
  const [primary, setPrimary] = useState("");
  const [extras, setExtras] = useState<string[]>([]);

  useEffect(() => {
    if (open) {
      setEntries(mainGames.map((g) => ({ ...g, extra_files: [...(g.extra_files ?? [])] })));
      void refresh();
    }
  }, [open, mainGames, refresh]);

  const usedFiles = new Set<string>();
  for (const e of entries) {
    if (e.file) usedFiles.add(e.file);
    for (const ex of e.extra_files ?? []) usedFiles.add(ex);
  }

  const availablePrimary = files.filter((f) => !usedFiles.has(f) || f === primary);
  const availableExtras = files.filter((f) => f !== primary);

  const addEntry = () => {
    if (!primary) return;
    setEntries((prev) => [
      ...prev,
      { file: primary, extra_files: extras.length ? [...extras] : undefined },
    ]);
    setPrimary("");
    setExtras([]);
  };

  return (
    <Modal
      open={open}
      wide
      title="Main games catalog"
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={() => void refresh()} disabled={loading}>
            Refresh files
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              onSave(entries);
              onClose();
            }}
          >
            Save catalog
          </Button>
        </>
      }
    >
      <p className="mb-3 text-xs text-slate-500">
        Catalog entries and extra files clients download alongside the primary ROM.
      </p>
      <div className="mb-4 max-h-48 space-y-2 overflow-y-auto scrollbar-thin">
        {entries.length === 0 ? (
          <p className="text-xs text-slate-500">No catalog entries.</p>
        ) : (
          entries.map((g, idx) => (
            <div
              key={`${g.file}-${idx}`}
              className="flex items-start justify-between gap-2 rounded-lg border border-slate-800 bg-slate-950/40 p-2"
            >
              <div>
                <p className="font-mono text-sm text-slate-200">{g.file}</p>
                <p className="text-[11px] text-slate-500">
                  {g.extra_files?.length ? `extra: ${g.extra_files.join(", ")}` : "no extra files"}
                </p>
              </div>
              <Button
                variant="danger"
                onClick={() => setEntries((prev) => prev.filter((_, i) => i !== idx))}
              >
                Remove
              </Button>
            </div>
          ))
        )}
      </div>
      <div className="space-y-2 border-t border-slate-800 pt-3">
        <div>
          <FieldLabel>Primary file</FieldLabel>
          <Select value={primary} onChange={(e) => setPrimary(e.target.value)}>
            <option value="">— select file —</option>
            {availablePrimary.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </Select>
        </div>
        <div>
          <FieldLabel>Extra files</FieldLabel>
          <select
            multiple
            className="mt-1 max-h-24 w-full rounded-lg border border-slate-700 bg-slate-950/80 px-2 py-1 text-xs text-slate-100"
            value={extras}
            onChange={(e) => setExtras(Array.from(e.target.selectedOptions, (o) => o.value))}
          >
            {availableExtras.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </select>
        </div>
        <Button variant="secondary" disabled={!primary} onClick={addEntry}>
          Add entry
        </Button>
      </div>
    </Modal>
  );
}
