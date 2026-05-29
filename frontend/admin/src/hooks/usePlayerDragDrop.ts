import { useState, type DragEvent } from "react";

function setDragOpacity(el: EventTarget | null, on: boolean) {
  if (el instanceof HTMLElement) {
    el.classList.toggle("opacity-50", on);
  }
}

export function usePlayerDragDrop() {
  const [draggedPlayer, setDraggedPlayer] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);

  function onDragStart(player: string, e: DragEvent) {
    setDraggedPlayer(player);
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", player);
    setDragOpacity(e.currentTarget, true);
  }

  function onDragEnd(e?: DragEvent) {
    setDraggedPlayer(null);
    setDropTarget(null);
    if (e) setDragOpacity(e.currentTarget, false);
  }

  function onDragOver(targetId: string, e: DragEvent) {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setDropTarget(targetId);
  }

  function onDragLeave(e: DragEvent) {
    const current = e.currentTarget;
    const related = e.relatedTarget;
    if (current instanceof HTMLElement && related instanceof Node && current.contains(related)) {
      return;
    }
    setDropTarget(null);
  }

  return {
    draggedPlayer,
    dropTarget,
    onDragStart,
    onDragEnd,
    onDragOver,
    onDragLeave,
  };
}
