# Mario Lives Tracker Plugin

A BizShuffle plugin that detects Super Mario Bros 3 and tracks the player's lives count, automatically cycling back to 4 lives when reaching 1.

## Features

- Automatically detects when Super Mario Bros 3 is loaded
- Reads the current number of lives from RAM
- Logs lives changes to the BizHawk console
- **Automatically cycles lives back to 4 when they reach 1**
- Lightweight polling system that doesn't impact performance

## How It Works

The plugin monitors the game name to detect Super Mario Bros 3. When detected, it reads the lives value from WRAM address `0x075A` every 60 frames (approximately 1 second at 60 FPS).

Lives values range from 0-99:
- 0 = Game Over
- 1-99 = Normal lives count

**Special Feature**: When lives drop to 1, the plugin automatically writes 4 back to memory, effectively giving Mario infinite lives with a minimum of 4.

## Configuration

The plugin is configured for optimal performance out of the box:

- **Poll Interval**: 60 frames (adjustable in `plugin.lua`)
- **Target Game**: "super mario bros. 3" (substring match)
- **Memory Address**: WRAM 0x075A (standard SMB3 lives address)
- **Cycle Threshold**: 1 life (triggers cycle to 4)
- **Cycle Target**: 4 lives (new lives count after cycling)

## Usage

1. Ensure the plugin is enabled in the server admin interface
2. Start Super Mario Bros 3 in BizHawk
3. The plugin will automatically detect the game and begin tracking
4. Lives changes will be logged to the console
5. When lives reach 1, they will automatically cycle back to 4

## Example Output

```
Mario Lives Tracker: Detected Super Mario Bros 3 - Super Mario Bros. 3 (USA)
Mario Lives Tracker: Initial lives: 4
Mario Lives Tracker: Lives changed: 4 -> 3
Mario Lives Tracker: Lives changed: 3 -> 2
Mario Lives Tracker: Lives changed: 2 -> 1
Mario Lives Tracker: Lives cycled back to 4!
Mario Lives Tracker: Lives changed: 4 -> 3
```

## Technical Details

- **Memory Domain**: WRAM
- **Address**: 0x075A
- **Data Type**: 8-bit unsigned integer
- **Polling Rate**: Every 60 frames
- **Game Detection**: Substring match on ROM name
- **Memory Write**: Uses BizHawk's memory.writebyte API

## Compatibility

- BizHawk 2.8.0+
- Super Mario Bros 3 (USA) ROM
- Works with all SMB3 revisions that use the same memory layout