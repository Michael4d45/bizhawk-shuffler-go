import { createContext, useContext, type ReactNode } from "react";
import { usePlayerDragDrop } from "./hooks/usePlayerDragDrop.js";

type DragApi = ReturnType<typeof usePlayerDragDrop>;

const PlayerDragContext = createContext<DragApi | null>(null);

export function PlayerDragProvider({ children }: { children: ReactNode }) {
  const api = usePlayerDragDrop();
  return <PlayerDragContext.Provider value={api}>{children}</PlayerDragContext.Provider>;
}

export function usePlayerDrag(): DragApi {
  const ctx = useContext(PlayerDragContext);
  if (!ctx) throw new Error("usePlayerDrag requires PlayerDragProvider");
  return ctx;
}

export function useOptionalPlayerDrag(): DragApi | null {
  return useContext(PlayerDragContext);
}
