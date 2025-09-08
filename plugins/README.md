# Plugins Directory

This directory contains Lua plugins for BizHawk Shuffler.

## Structure

- `available/` - All available plugins with their metadata
- `enabled/` - Currently enabled plugins (symlinks to available/)
- `disabled/` - Explicitly disabled plugins

## Plugin Development

See `PLUGIN_TODO.md` in the repository root for comprehensive plugin development documentation and roadmap.

## Basic Plugin Structure

Each plugin should be in its own subdirectory under `available/`:

```
plugins/available/my-plugin/
├── plugin.lua     # Main plugin code
├── meta.json      # Plugin metadata
└── README.md      # Plugin documentation
```

### Example meta.json

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Example plugin",
  "author": "Plugin Author",
  "bizhawk_version": ">=2.8.0",
  "enabled": true,
  "entry_point": "plugin.lua"
}
```

## Current Status

🚧 **Under Development** - The plugin system is currently in early development phase. See `PLUGIN_TODO.md` for implementation roadmap.