import { useEffect, useRef, useState } from "react";
import { getPluginDetails, getPluginSettings, postPluginSettings } from "../api.js";
import type { Plugin } from "../types.js";
import { Modal } from "./Modal.js";
import { PluginSettingField } from "./PluginSettingField.js";
import { isKnownPluginSetting, orderedPluginSettingKeys } from "./pluginSettings.js";
import { Button, EmptyState, FieldLabel, Input, Select } from "./ui.js";

type Props = {
  open: boolean;
  pluginName: string | null;
  onClose: () => void;
  onSaved: () => void;
  onLog: (msg: string) => void;
  onReload?: (name: string) => Promise<boolean>;
};

export function PluginSettingsModal({
  open,
  pluginName,
  onClose,
  onSaved,
  onLog,
  onReload,
}: Props) {
  const [details, setDetails] = useState<Plugin | null>(null);
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");
  const onLogRef = useRef(onLog);
  useEffect(() => {
    onLogRef.current = onLog;
  });

  useEffect(() => {
    if (!open || !pluginName) {
      setDetails(null);
      setSettings({});
      setLoadError(null);
      setLoading(false);
      setShowAdvanced(false);
      setNewKey("");
      setNewValue("");
      return;
    }

    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    void (async () => {
      try {
        const [meta, kv] = await Promise.all([
          getPluginDetails(pluginName),
          getPluginSettings(pluginName),
        ]);
        if (cancelled) return;
        setDetails(meta);
        setSettings(kv);
      } catch (e) {
        if (!cancelled) {
          const msg = e instanceof Error ? e.message : String(e);
          setLoadError(msg);
          onLogRef.current(msg);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [open, pluginName]);

  async function persist(closeOnSuccess: boolean, reloadAfter = false): Promise<boolean> {
    if (!pluginName) return false;
    setSaving(true);
    try {
      const res = await postPluginSettings(pluginName, settings);
      if (!res.ok) {
        const detail = (await res.text()).trim();
        onLog(
          detail ? `failed to save plugin settings: ${detail}` : "failed to save plugin settings"
        );
        return false;
      }
      onLog("plugin settings saved");
      onSaved();
      if (reloadAfter && onReload) {
        const reloaded = await onReload(pluginName);
        if (reloaded) onLog(`plugin ${pluginName} reloaded on clients`);
      }
      if (closeOnSuccess) onClose();
      return true;
    } finally {
      setSaving(false);
    }
  }

  const settingKeys = orderedPluginSettingKeys(settings, details?.settings_meta);
  const knownKeys = settingKeys.filter((k) => isKnownPluginSetting(k, details?.settings_meta));
  const customKeys = settingKeys.filter((k) => !isKnownPluginSetting(k, details?.settings_meta));

  return (
    <Modal
      open={open}
      wide
      title={pluginName ? `Plugin settings: ${pluginName}` : "Plugin settings"}
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            Cancel
          </Button>
          {onReload ? (
            <Button
              variant="secondary"
              disabled={loading || saving || !pluginName}
              onClick={() => void persist(true, true)}
            >
              {saving ? "Saving…" : "Save & reload"}
            </Button>
          ) : null}
          <Button
            variant="primary"
            disabled={loading || saving || !pluginName}
            onClick={() => void persist(true)}
          >
            {saving ? "Saving…" : "Save settings"}
          </Button>
        </>
      }
    >
      {loading ? (
        <p className="text-sm text-slate-500">Loading plugin settings…</p>
      ) : loadError ? (
        <EmptyState>{loadError}</EmptyState>
      ) : (
        <>
          {details ? (
            <div className="mb-4 rounded-lg border border-slate-800 bg-slate-950/40 p-3 text-xs text-slate-500">
              <p className="text-sm text-slate-300">{details.description || "No description"}</p>
              <p className="mt-1">
                v{details.version} · {details.author} · BizHawk {details.bizhawk_version}
              </p>
            </div>
          ) : null}

          <p className="mb-3 text-xs text-slate-500">
            Changes are written to <span className="font-mono">settings.kv</span> on the server.
            Connected clients pick them up on plugin reload.
          </p>

          <div className="mb-4">
            <FieldLabel htmlFor="plugin-status">Status</FieldLabel>
            <Select
              id="plugin-status"
              value={settings.status ?? "disabled"}
              onChange={(e) => setSettings((s) => ({ ...s, status: e.target.value }))}
            >
              <option value="enabled">Enabled</option>
              <option value="disabled">Disabled</option>
            </Select>
          </div>

          {knownKeys.length > 0 ? (
            <div className="space-y-3">
              {knownKeys.map((key) => (
                <PluginSettingField
                  key={key}
                  settingKey={key}
                  value={settings[key] ?? ""}
                  meta={details?.settings_meta?.[key]}
                  onChange={(value) => setSettings((s) => ({ ...s, [key]: value }))}
                />
              ))}
            </div>
          ) : customKeys.length === 0 ? (
            <EmptyState>No configurable settings besides status.</EmptyState>
          ) : null}

          {customKeys.length > 0 ? (
            <div className={knownKeys.length > 0 ? "mt-4 space-y-3" : "space-y-3"}>
              {knownKeys.length > 0 ? (
                <p className="text-[11px] font-medium uppercase tracking-wide text-slate-500">
                  Other settings
                </p>
              ) : null}
              {customKeys.map((key) => (
                <PluginSettingField
                  key={key}
                  settingKey={key}
                  value={settings[key] ?? ""}
                  onChange={(value) => setSettings((s) => ({ ...s, [key]: value }))}
                  onRemove={() =>
                    setSettings((s) => {
                      const next = { ...s };
                      delete next[key];
                      return next;
                    })
                  }
                />
              ))}
            </div>
          ) : null}

          <div className="mt-4 border-t border-slate-800 pt-3">
            <Button variant="ghost" onClick={() => setShowAdvanced((v) => !v)}>
              {showAdvanced ? "Hide advanced" : "Add custom setting"}
            </Button>
            {showAdvanced ? (
              <div className="mt-3 rounded-lg border border-slate-800 bg-slate-950/40 p-3">
                <p className="mb-2 text-[11px] text-slate-500">
                  For keys not declared in <span className="font-mono">meta.kv</span>. Prefer
                  defining <span className="font-mono">setting.*.type</span> hints in meta for
                  dropdowns and multiselects.
                </p>
                <div className="flex flex-wrap gap-2">
                  <Input
                    className="min-w-[8rem] flex-1"
                    placeholder="Key"
                    value={newKey}
                    onChange={(e) => setNewKey(e.target.value)}
                  />
                  <Input
                    className="min-w-[8rem] flex-1"
                    placeholder="Value"
                    value={newValue}
                    onChange={(e) => setNewValue(e.target.value)}
                  />
                  <Button
                    variant="secondary"
                    disabled={!newKey.trim() || newKey.trim() === "status"}
                    onClick={() => {
                      const key = newKey.trim();
                      setSettings((s) => ({ ...s, [key]: newValue }));
                      setNewKey("");
                      setNewValue("");
                    }}
                  >
                    Add
                  </Button>
                </div>
              </div>
            ) : null}
          </div>
        </>
      )}
    </Modal>
  );
}
