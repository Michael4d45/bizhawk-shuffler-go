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
  {
    label: "Name hash",
    path: "/api/toggle_player_name_hash",
    toggle: "player_name_hash_assignment" as const,
  },
  { label: "Countdown", path: "/api/toggle_countdown", toggle: "countdown_enabled" as const },
  { label: "Clear Saves", path: "/api/clear_saves" },
  { label: "Reset session", path: "/api/reset" },
] as const;
