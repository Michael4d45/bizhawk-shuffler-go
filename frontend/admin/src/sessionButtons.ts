export const SESSION_BUTTONS = [
  { label: "Start", path: "/api/start" },
  { label: "Pause", path: "/api/pause" },
  { label: "Do Swap", path: "/api/do_swap" },
  { label: "Auto Swaps", path: "/api/toggle_swaps", toggle: "swap_enabled" as const },
  {
    label: "Better Random",
    path: "/api/toggle_prevent_same_game",
    toggle: "prevent_same_game_swap" as const,
  },
  { label: "Countdown", path: "/api/toggle_countdown", toggle: "countdown_enabled" as const },
  { label: "Clear Saves", path: "/api/clear_saves" },
] as const;
