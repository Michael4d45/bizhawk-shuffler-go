# Plugins Directory

This directory contains Lua plugins for BizHawk Shuffler.

## Structure

All plugins are stored directly in this directory with their metadata.

## Basic Plugin Structure

Each plugin should be in its own subdirectory:

```
plugins/my-plugin/
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
