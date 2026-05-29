import { useEffect, useState } from "react";

/**
 * Drives client-side swap countdown/progress between server state polls.
 * Uses rAF when active for a smooth progress bar; otherwise ticks once per second.
 */
export function useNowMs(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!active) {
      setNow(Date.now());
      return;
    }

    let frame = 0;
    const tick = () => {
      setNow(Date.now());
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [active]);

  return now;
}
