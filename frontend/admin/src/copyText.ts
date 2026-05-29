/** Copy text; works on plain HTTP (e.g. headless admin at a LAN IP). */
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      /* Clipboard API blocked outside secure context — use legacy path */
    }
  }
  return copyTextLegacy(text);
}

/** Plain-HTTP fallback; Clipboard API is unavailable outside a secure context. */
const execCopy = document.execCommand.bind(document) as (command: "copy") => boolean;

function copyTextLegacy(text: string): boolean {
  const el = document.createElement("textarea");
  el.value = text;
  el.setAttribute("readonly", "");
  el.style.position = "fixed";
  el.style.left = "-9999px";
  el.style.top = "0";
  document.body.appendChild(el);
  el.focus();
  el.select();
  el.setSelectionRange(0, text.length);
  let ok = false;
  try {
    ok = execCopy("copy");
  } catch {
    ok = false;
  } finally {
    document.body.removeChild(el);
  }
  return ok;
}
