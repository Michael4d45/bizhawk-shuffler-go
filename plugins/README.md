# Plugins Directory

This directory contains Lua plugins for BizHawk Shuffler.

## Structure

All plugins are stored directly in this directory with their metadata.

## Basic Plugin Structure

Each plugin should be in its own subdirectory:

```
plugins/my-plugin/
├── plugin.lua     # Main plugin code
├── meta.kv        # Plugin metadata (read-only, simple key=value)
├── settings.kv    # Plugin settings (user-configurable, simple key=value)
└── README.md      # Plugin documentation
```

### meta.kv (Read-only Metadata)

The `meta.kv` file contains static plugin metadata that should not be modified by users. Example:

```
name = my-plugin
version = 1.0.0
description = Example plugin
author = Plugin Author
bizhawk_version = >=2.8.0
```

**Note:** The `status` field has been moved to `settings.kv` and should not be in `meta.kv`.

### settings.kv (User-configurable Settings)

The `settings.kv` file contains user-configurable settings. The `status` field is required and must be either `enabled` or `disabled`. Plugins can define additional custom settings as needed. Example:

```
status = enabled
custom_setting_1 = value1
custom_setting_2 = value2
```

Settings can be edited through the server web UI. When settings are updated, plugins implementing the `on_settings_changed` hook will be notified.

## Plugin Hooks

Plugins can implement the following hooks:

- **on_init()** - Called when the plugin is first loaded. Use this for one-time initialization.
- **on_frame()** - Called every frame during emulation. Use this for per-frame logic.
- **on_settings_changed(settings)** - Called when plugin settings are updated. Receives a table of all current settings.

### Example Plugin Structure

```lua
-- Access plugin settings and metadata
local settings = ...  -- Available via _settings table
local meta = ...      -- Available via _meta table

-- on_init hook
local function on_init()
    console.log("Plugin initialized")
    -- Access settings: settings["custom_setting_1"]
    -- Access metadata: meta["name"]
end

-- on_frame hook
local function on_frame()
    -- Called every frame
end

-- on_settings_changed hook
local function on_settings_changed(new_settings)
    console.log("Settings changed!")
    -- Update internal state based on new_settings
end

-- Export hooks
return {
    on_init = on_init,
    on_frame = on_frame,
    on_settings_changed = on_settings_changed
}
```

**Note:** Plugins can access their settings table via the `_settings` property and metadata via the `_meta` property, but the `on_settings_changed` hook is the recommended way to react to settings changes.
