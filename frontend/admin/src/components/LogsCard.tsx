import { Card, EmptyState } from "./ui.js";

type Props = {
  log: string[];
};

export function LogsCard({ log }: Props) {
  return (
    <Card title="Activity log" subtitle={`${log.length} recent events`}>
      {log.length === 0 ? (
        <EmptyState>Actions and connection events will appear here.</EmptyState>
      ) : (
        <div className="max-h-96 overflow-y-auto rounded-lg border border-slate-800 bg-slate-950/60 p-2 font-mono text-[11px] leading-relaxed scrollbar-thin">
          {log.map((line, i) => {
            const space = line.indexOf(" ");
            const time = space > 0 ? line.slice(0, space) : "";
            const msg = space > 0 ? line.slice(space + 1) : line;
            const failed = msg.includes("failed");
            const ok = msg.endsWith(" ok");
            return (
              <div key={i} className="flex gap-2 border-b border-slate-800/50 py-1.5 last:border-0">
                <span className="shrink-0 text-slate-600">{time}</span>
                <span
                  className={
                    failed ? "text-rose-400" : ok ? "text-emerald-400/90" : "text-slate-300"
                  }
                >
                  {msg}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
