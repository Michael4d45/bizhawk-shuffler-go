import type { Plugin } from "../types.js";

export function formatSettingLabel(key: string): string {
  return key
    .split(/[_-]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

/** Keys from meta.kv first, then any extra keys present in settings.kv (excluding status). */
export function orderedPluginSettingKeys(
  settings: Record<string, string>,
  settingsMeta?: Plugin["settings_meta"]
): string[] {
  const metaKeys = settingsMeta ? Object.keys(settingsMeta) : [];
  const extra = Object.keys(settings)
    .filter((k) => k !== "status" && !metaKeys.includes(k))
    .sort();
  return [...metaKeys, ...extra];
}

export function isKnownPluginSetting(key: string, settingsMeta?: Plugin["settings_meta"]): boolean {
  return Boolean(settingsMeta?.[key]);
}
