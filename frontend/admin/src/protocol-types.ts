/** Shared admin types (mirrors Go/TS protocol package). */

export type CommandName =
  | "hello"
  | "ack"
  | "nack"
  | "games_update_ack"
  | "status_update"
  | "lua_command"
  | "config_response"
  | "hello_admin"
  | "ping"
  | "start"
  | "pause"
  | "swap"
  | "message"
  | "games_update"
  | "clear_saves"
  | "request_save"
  | "plugin_reload"
  | "fullscreen_toggle"
  | "check_config"
  | "update_config"
  | "state_update";

export interface Command {
  cmd: CommandName;
  id: string;
  payload?: unknown;
}

export interface GameEntry {
  file: string;
  extra_files?: string[];
}

export interface Player {
  name: string;
  has_files: boolean;
  connected: boolean;
  bizhawk_ready: boolean;
  game?: string;
  instance_id?: string;
  ping_ms?: number;
  completed_games?: string[];
  completed_instances?: string[];
  config_values?: Record<string, unknown>;
}

export type FileState = "none" | "pending" | "ready";

export interface GameSwapInstance {
  id: string;
  game: string;
  file_state: FileState;
  pending_player?: string;
}

export type PluginStatus = "disabled" | "enabled" | "loading" | "error";

export interface Plugin {
  name: string;
  version: string;
  bizhawk_version: string;
  description: string;
  author: string;
  status: PluginStatus;
  settings_meta?: Record<string, { type: string; options?: string[] }>;
}

export interface ServerState {
  running: boolean;
  swap_enabled: boolean;
  mode?: "sync" | "save";
  host?: string;
  port?: number;
  next_swap_at?: number;
  min_interval_secs?: number;
  max_interval_secs?: number;
  main_games?: GameEntry[];
  plugins?: Record<string, Plugin>;
  players: Record<string, Player>;
  updated_at: string;
  games?: string[];
  game_instances?: GameSwapInstance[];
  prevent_same_game_swap: boolean;
  countdown_enabled: boolean;
  swap_seed?: number;
  config_keys?: string[];
}
