import type { Plugin } from "../types.js";
import { formatSettingLabel } from "./pluginSettings.js";
import { Button, FieldLabel, Input, Select } from "./ui.js";

type SettingMeta = NonNullable<Plugin["settings_meta"]>[string];

type Props = {
  settingKey: string;
  value: string;
  meta?: SettingMeta;
  onChange: (value: string) => void;
  onRemove?: () => void;
};

export function PluginSettingField({ settingKey, value, meta, onChange, onRemove }: Props) {
  const label = formatSettingLabel(settingKey);
  const controlId = `plugin-setting-${settingKey}`;

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-950/30 p-3">
      <div className="mb-2 flex items-start justify-between gap-2">
        <FieldLabel htmlFor={controlId}>{label}</FieldLabel>
        {onRemove ? (
          <Button variant="ghost" className="shrink-0 text-rose-400/90" onClick={onRemove}>
            Remove
          </Button>
        ) : null}
      </div>
      {meta?.type === "dropdown" && meta.options?.length ? (
        <Select id={controlId} value={value} onChange={(e) => onChange(e.target.value)}>
          {meta.options.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </Select>
      ) : meta?.type === "multiselect" && meta.options?.length ? (
        <div id={controlId} className="mt-1 space-y-1.5">
          {meta.options.map((opt) => {
            const selected = value.split(",").filter(Boolean);
            const checked = selected.includes(opt);
            return (
              <label key={opt} className="flex items-center gap-2 text-sm text-slate-300">
                <input
                  type="checkbox"
                  className="rounded border-slate-600 bg-slate-950 text-emerald-500 focus:ring-emerald-500/40"
                  checked={checked}
                  onChange={(e) => {
                    const next = new Set(selected);
                    if (e.target.checked) next.add(opt);
                    else next.delete(opt);
                    onChange([...next].join(","));
                  }}
                />
                {opt}
              </label>
            );
          })}
        </div>
      ) : (
        <Input id={controlId} value={value} onChange={(e) => onChange(e.target.value)} />
      )}
      <p className="mt-1.5 font-mono text-[10px] text-slate-600">{settingKey}</p>
    </div>
  );
}
