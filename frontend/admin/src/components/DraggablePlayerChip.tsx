import type { usePlayerDragDrop } from "../hooks/usePlayerDragDrop.js";
import { cn } from "./ui.js";

type DragApi = ReturnType<typeof usePlayerDragDrop>;

function GripIcon({ className }: { className?: string }) {
  return (
    <svg
      className={cn("size-3 shrink-0 opacity-60", className)}
      fill="currentColor"
      viewBox="0 0 20 20"
    >
      <path d="M3 4h14a1 1 0 010 2H3a1 1 0 010-2zM3 8h14a1 1 0 010 2H3a1 1 0 010-2zM3 12h14a1 1 0 010 2H3a1 1 0 010-2z" />
    </svg>
  );
}

type Props = {
  name: string;
  dnd: DragApi;
  variant?: "assigned" | "unassigned";
};

export function DraggablePlayerChip({ name, dnd, variant = "unassigned" }: Props) {
  const isAssigned = variant === "assigned";
  return (
    <span
      draggable
      onDragStart={(e) => {
        e.stopPropagation();
        dnd.onDragStart(name, e);
      }}
      onDragEnd={(e) => dnd.onDragEnd(e)}
      className={cn(
        "inline-flex cursor-grab select-none items-center gap-1 rounded px-2 py-1 text-xs active:cursor-grabbing",
        isAssigned
          ? "bg-sky-600/20 text-sky-300 hover:bg-sky-600/30"
          : "bg-slate-700 text-slate-200 hover:bg-slate-600"
      )}
    >
      <GripIcon className={isAssigned ? "text-sky-400" : "text-slate-400"} />
      {name}
    </span>
  );
}
