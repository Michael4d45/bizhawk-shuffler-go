import { useEffect, useState } from "react";
import { fetchShareUrls, type ShareUrls } from "../api.js";
import { copyText } from "../copyText.js";
import { useToast } from "./Toast.js";
import { Button, cn, FieldLabel } from "./ui.js";

function EyeIcon({ open }: { open: boolean }) {
  if (open) {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        className="h-4 w-4"
        aria-hidden
      >
        <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
        <circle cx="12" cy="12" r="3" />
      </svg>
    );
  }
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      className="h-4 w-4"
      aria-hidden
    >
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-10-8-10-8a18.45 18.45 0 0 1 5.06-6.94" />
      <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 10 8 10 8a18.5 18.5 0 0 1-2.16 3.19" />
      <path d="M1 1l22 22" />
      <path d="M14.12 14.12a3 3 0 1 1-4.24-4.24" />
    </svg>
  );
}

type AddressRowProps = {
  label: string;
  urls: string[];
  visible: boolean;
  onCopy: (url: string) => void;
};

function AddressRow({ label, urls, visible, onCopy }: AddressRowProps) {
  if (urls.length === 0) return null;
  return (
    <div className="space-y-1.5">
      <FieldLabel>{label}</FieldLabel>
      {urls.map((url) => (
        <div key={url} className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded-lg border border-slate-800 bg-slate-950/80 px-2.5 py-1.5 font-mono text-xs text-slate-200">
            {visible ? url : "••••••••••••••••••••"}
          </code>
          <Button
            variant="secondary"
            className="shrink-0"
            disabled={!visible}
            onClick={() => onCopy(url)}
          >
            Copy
          </Button>
        </div>
      ))}
    </div>
  );
}

export function ShareAddresses() {
  const { showToast } = useToast();
  const [visible, setVisible] = useState(false);
  const [urls, setUrls] = useState<ShareUrls | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const data = await fetchShareUrls();
        if (!cancelled) setUrls(data);
      } catch {
        if (!cancelled) setUrls(null);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    const id = setInterval(() => void load(), 30_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  async function copyUrl(url: string) {
    const ok = await copyText(url);
    if (ok) showToast("Copied to clipboard");
    else showToast("Could not copy", "err");
  }

  const lanUrls = urls?.lan ?? [];
  const wanUrl = urls?.wan ? [urls.wan] : [];

  return (
    <details className="group rounded-lg border border-slate-800/80 bg-slate-950/40 open:bg-slate-950/60">
      <summary
        className={cn(
          "flex cursor-pointer list-none items-center gap-2 px-3 py-2.5",
          "[&::-webkit-details-marker]:hidden"
        )}
      >
        <span className="text-slate-500 transition group-open:rotate-90" aria-hidden>
          ▶
        </span>
        <span className="flex-1 text-[11px] font-medium uppercase tracking-wide text-slate-500">
          Share with friends
        </span>
        <Button
          variant="ghost"
          className="!px-2"
          onClick={(e) => {
            e.preventDefault();
            setVisible((v) => !v);
          }}
          aria-label={visible ? "Hide share addresses" : "Show share addresses"}
          title={visible ? "Hide addresses" : "Show addresses"}
        >
          <EyeIcon open={visible} />
        </Button>
      </summary>

      <div className="space-y-2 border-t border-slate-800/80 px-3 py-2.5">
        {loading ? (
          <p className="text-xs text-slate-500">Loading addresses…</p>
        ) : !urls ? (
          <p className="text-xs text-slate-500">Could not load share addresses.</p>
        ) : (
          <>
            {urls.local_only ? (
              <p className="text-xs text-amber-300/90">
                Server is local-only (<code className="text-amber-200">127.0.0.1</code>). Bind to{" "}
                <code className="text-amber-200">0.0.0.0</code> in Host options to share on LAN.
              </p>
            ) : null}
            <AddressRow label="LAN" urls={lanUrls} visible={visible} onCopy={copyUrl} />
            <AddressRow
              label="WAN (port forward required)"
              urls={wanUrl}
              visible={visible}
              onCopy={copyUrl}
            />
            {!urls.local_only && lanUrls.length === 0 && wanUrl.length === 0 ? (
              <p className="text-xs text-slate-500">No shareable addresses found.</p>
            ) : null}
          </>
        )}
      </div>
    </details>
  );
}
