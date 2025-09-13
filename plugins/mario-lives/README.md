# Mario Lives Tracker Plugin

A BizShuffle plugin that detects Super Mario Bros 3 and automatically maintains a minimum of 4 lives by cycling back when lives reach 1 or below.

## Features

- Automatically detects when Super Mario Bros 3 is loaded
- Monitors lives count from RAM every 300 frames (5 seconds)
- Automatically cycles lives back to 4 when they reach 1 or below
- Lightweight polling system that doesn't impact performance
- Simplified logic for reliable infinite lives functionality

## How It Works

The plugin monitors the game name to detect Super Mario Bros 3. When detected, it reads the lives value from RAM address `0x0736` every 300 frames (approximately 5 seconds at 60 FPS).

**Special Feature**: When lives drop to 1 or below, the plugin immediately writes 4 back to memory, ensuring Mario always has at least 4 lives.

## Configuration

The plugin is configured for optimal performance out of the box:

- **Poll Interval**: 300 frames (adjustable in `plugin.lua`)
- **Target Game**: "super mario bros. 3" (substring match)
- **Memory Address**: RAM 0x0736 (SMB3 lives address)
- **Cycle Threshold**: ≤ 1 life (triggers cycle to 4)
- **Cycle Target**: 4 lives (new lives count after cycling)

## Usage

1. Ensure the plugin is enabled in the server admin interface
2. Start Super Mario Bros 3 in BizHawk
3. The plugin will automatically detect the game and begin monitoring
4. Lives are automatically maintained at 4+ with seamless cycling

## Example Output

```
Mario Lives Tracker: Plugin loaded!
```

*Note: The plugin runs silently in the background with minimal console output for performance.*

## Technical Details

- **Memory Domain**: RAM
- **Address**: 0x0736
- **Data Type**: 8-bit unsigned integer
- **Polling Rate**: Every 300 frames (5 seconds)
- **Game Detection**: Substring match on ROM name
- **Memory Write**: Uses BizHawk's memory.writebyte API
- **Cycle Logic**: Simple threshold check (≤ 1 → 4 lives)

## Compatibility

- BizHawk 2.8.0+
- Super Mario Bros 3 (USA) ROM
- Works with all SMB3 revisions that use the same memory layout