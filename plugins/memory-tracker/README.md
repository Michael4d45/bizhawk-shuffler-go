# Memory Tracker Plugin

This plugin tracks multiple memory address types (door, health, etc.) per game with configurable monitoring.

## Features

- **Multiple Type Support**: Track different memory types (door, health, etc.) simultaneously per game
- **Configurable Types**: Enable/disable specific types via settings
- **Per-Game Configuration**: Each game can have different address configurations for each type
- **Guard Words**: Optional guard conditions to prevent false positives
- **Flexible Monitoring**: Supports different memory domains, sizes, and endianness

## Configuration

### Settings (settings.kv)

- `status`: Enable/disable the plugin (`enabled` or `disabled`)
- `command_type`: Command to send when memory changes (`swap` or `swap_me`)
- `enabled_types`: Comma-separated list of types to monitor (e.g., `door,health`)

### Game Configuration

Games are configured in the `games` table within `plugin.lua`. Each game can have multiple types:

```lua
games["game name"] = {
    door = {
        addr = 0x1C8544,
        size = 2,
        domain = "RDRAM",
        desc = "Entrance Index",
        guardWord = {  -- Optional
            addr = 0x1C8540,
            size = 4,
            equals = 0
        }
    },
    health = {
        addr = 0x11A601,
        size = 2,
        domain = "RDRAM",
        desc = "Health value"
    }
}
```

### Configuration Options

- `addr`: Memory address (hexadecimal number)
- `size`: Size in bytes (1, 2, or 4)
- `domain`: Memory domain name (optional, auto-detected if not specified)
- `desc`: Description for logging
- `guardWord`: Optional guard condition (object with `addr`, `size`, and `equals`)
- `equals`: Optional value check (only trigger when value equals this)

## Usage

1. Enable the plugin through the admin web interface
2. Configure enabled types in settings (e.g., `enabled_types = door,health`)
3. Add game configurations to `plugin.lua` as needed
4. Start a game session to see the plugin in action
5. Check the BizHawk console for plugin messages

## Examples

### Tracking Door Changes (Ocarina of Time)

```lua
["legend of zelda, the - ocarina of time (usa)"] = {
    door = {
        addr = 0x1C8544,
        size = 2,
        domain = "RDRAM",
        desc = "Entrance Index"
    }
}
```

### Tracking Health (Ocarina of Time)

```lua
["legend of zelda, the - ocarina of time (usa)"] = {
    health = {
        addr = 0x11A601,
        size = 2,
        domain = "RDRAM",
        desc = "Health value"
    }
}
```

### Tracking Multiple Types

```lua
["legend of zelda, the - ocarina of time (usa)"] = {
    door = { addr = 0x1C8544, size = 2, domain = "RDRAM", desc = "Entrance" },
    health = { addr = 0x11A601, size = 2, domain = "RDRAM", desc = "Health" }
}
```

## Finding Memory Addresses

- Use BizHawk's Hex Editor to inspect memory
- Consult RAM maps (DataCrystal, ROM-hacking wiki, etc.)
- Use memory watch tools to find changing values
- Enable the probe feature (AUTO_PROBE_ON_START) to discover changed bytes

## Plugin Structure

- `plugin.lua` - Main plugin code with hook functions
- `meta.kv` - Plugin metadata and configuration options
- `settings.kv` - User settings (status, command_type, enabled_types)
- `README.md` - This documentation file

