import { useEffect, useRef, useState } from "react";
import type { AdminTrigger } from "../adminActions.js";
import { fetchJson } from "../api.js";
import type { Plugin } from "../types.js";
import { PluginSettingsModal } from "./PluginSettingsModal.js";
import { ActionRow, Badge, Button, Card, EmptyState } from "./ui.js";

type Props = {
  trigger: AdminTrigger;
  pushLog: (msg: string) => void;
};

function pluginBadgeVariant(status: string): "ok" | "warn" | "neutral" {
  if (status === "enabled") return "ok";
  if (status === "disabled") return "warn";
  return "neutral";
}

async function fetchPluginMap(): Promise<Record<string, Plugin>> {
  const body = await fetchJson<{ plugins: Record<string, Plugin> }>("/api/plugins");
  return body.plugins ?? {};
}

export function PluginsCard({ trigger, pushLog }: Props) {
  const [plugins, setPlugins] = useState<Record<string, Plugin>>({});
  const [loading, setLoading] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [editName, setEditName] = useState<string | null>(null);
  const pushLogRef = useRef(pushLog);
  useEffect(() => {
    pushLogRef.current = pushLog;
  });

  async function loadPlugins() {
    setLoading(true);
    try {
      setPlugins(await fetchPluginMap());
    } catch (e) {
      pushLog(String(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void fetchPluginMap()
      .then((next) => {
        if (!cancelled) setPlugins(next);
      })
      .catch((e) => {
        if (!cancelled) pushLogRef.current(String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const entries = Object.entries(plugins);

  return (
    <>
      <Card
        title="Plugins"
        subtitle={`${entries.length} installed`}
        actions={
          <ActionRow>
            <Button variant="ghost" onClick={() => setExpanded((e) => !e)}>
              {expanded ? "Collapse" : "Expand"}
            </Button>
            <Button variant="ghost" onClick={() => void loadPlugins()} disabled={loading}>
              {loading ? "Loading…" : "Refresh"}
            </Button>
            <Button variant="ghost" onClick={() => void trigger("/api/open_plugins_folder")}>
              Open folder
            </Button>
          </ActionRow>
        }
      >
        {entries.length === 0 ? (
          <EmptyState>No plugins loaded.</EmptyState>
        ) : (
          <ul
            className={
              expanded
                ? "max-h-[32rem] space-y-2 overflow-y-auto scrollbar-thin pr-1"
                : "max-h-72 space-y-2 overflow-y-auto scrollbar-thin pr-1"
            }
          >
            {entries.map(([name, p]) => (
              <li
                key={name}
                className="rounded-lg border border-slate-800 bg-slate-950/40 px-3 py-2.5"
              >
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-slate-100">{name}</span>
                      <Badge variant={pluginBadgeVariant(p.status)}>{p.status}</Badge>
                    </div>
                    {p.description ? (
                      <p className="mt-1 text-xs text-slate-500 line-clamp-2">{p.description}</p>
                    ) : null}
                    <p className="mt-1 font-mono text-[10px] text-slate-600">
                      v{p.version} · {p.author} · BizHawk {p.bizhawk_version}
                    </p>
                  </div>
                  <ActionRow>
                    <Button
                      variant={p.status === "enabled" ? "ghost" : "primary"}
                      onClick={() =>
                        void trigger(`/api/plugins/${encodeURIComponent(name)}/settings`, {
                          status: p.status === "enabled" ? "disabled" : "enabled",
                        }).then(() => loadPlugins())
                      }
                    >
                      {p.status === "enabled" ? "Disable" : "Enable"}
                    </Button>
                    <Button
                      variant="ghost"
                      onClick={() =>
                        void trigger(`/api/plugins/${encodeURIComponent(name)}/reload`)
                      }
                    >
                      Reload
                    </Button>
                    <Button variant="ghost" onClick={() => setEditName(name)}>
                      Edit
                    </Button>
                    <Button
                      variant="danger"
                      onClick={async () => {
                        await fetch(`/api/plugins/${encodeURIComponent(name)}`, {
                          method: "DELETE",
                        });
                        await loadPlugins();
                      }}
                    >
                      Delete
                    </Button>
                  </ActionRow>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <PluginSettingsModal
        open={editName !== null}
        pluginName={editName}
        onClose={() => setEditName(null)}
        onSaved={() => void loadPlugins()}
        onLog={pushLog}
        onReload={(name) => trigger(`/api/plugins/${encodeURIComponent(name)}/reload`, undefined)}
      />
    </>
  );
}
