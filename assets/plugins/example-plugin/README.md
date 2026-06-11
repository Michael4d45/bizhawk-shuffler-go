# Example Plugin

This is an example plugin for BizShuffle that demonstrates basic plugin functionality.

## Features

- Logs messages when loaded and initialized
- Demonstrates `on_init`, `on_frame`, and `on_settings_changed` hooks
- Shows the basic plugin structure

## Usage

1. Enable the plugin through the admin web interface
2. Start a game session to see the plugin in action
3. Check the BizHawk console for plugin messages

## Plugin Structure

- `plugin.lua` - Main plugin code with hook functions
- `meta.kv` - Plugin metadata (key=value pairs)
- `settings.kv` - User settings (`status=enabled|disabled`, optional custom keys)
- `README.md` - This documentation file

This plugin serves as a template for developing more complex plugins.
