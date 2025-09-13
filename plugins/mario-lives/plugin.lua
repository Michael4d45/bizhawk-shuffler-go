-- Mario Lives Tracker plugin for BizHawk Shuffler
-- Detects Super Mario Bros 3 and tracks the number of lives
-- Automatically cycles lives back to 4 when they reach 1
--
-- Usage:
--  - Automatically detects Super Mario Bros 3 games
--  - Reads lives from WRAM address 0x075A
--  - Logs lives changes to console
--  - Cycles lives back to 4 when reaching 1
--
console.log("Mario Lives Tracker: Plugin loaded!")

-- === CONFIG ===
local pollInterval = 60 -- frames between checks
local targetGame = "super mario bros. 3" -- substring to match game name

-- === internal state ===
local isTargetGame = false
local lastLives = nil
local frameCounter = 0

-- === helpers ===
local function try_call(fn, ...)
    local ok, res = pcall(fn, ...)
    if ok then
        return res
    end
    return nil
end

local function read_lives()
    -- Super Mario Bros 3 stores lives at WRAM 0x075A
    -- Value ranges from 0-99 (0 = game over)
    local lives = try_call(memory.readbyte, 0x075A, "WRAM")
    return lives
end

local function write_lives(lives)
    -- Write lives value to WRAM 0x075A
    local success = try_call(memory.writebyte, 0x075A, lives, "WRAM")
    return success ~= nil
end

local function check_game()
    local rom = gameinfo.getromname()
    if rom then
        local romLower = rom:lower()
        isTargetGame = romLower:find(targetGame, 1, true) ~= nil
        if isTargetGame then
            -- console.log("Mario Lives Tracker: Detected Super Mario Bros 3 - " .. rom)
        end
    end
end

-- === core polling ===
local function track_lives()
    if not isTargetGame then
        return
    end

    local lives = read_lives()
    if lives == nil then
        return
    end

    if lastLives ~= lives then
        if lastLives ~= nil then
            console.log(("Mario Lives Tracker: Lives changed: %d -> %d"):format(lastLives, lives))

            -- Cycle back to 4 lives when reaching 1
            if lives == 1 and lastLives > 1 then
                if write_lives(4) then
                    console.log("Mario Lives Tracker: Lives cycled back to 4!")
                    lastLives = 4 -- Update our tracking to reflect the new value
                else
                    console.log("Mario Lives Tracker: Failed to write lives to memory")
                end
            else
                lastLives = lives
            end
        else
            console.log(("Mario Lives Tracker: Initial lives: %d"):format(lives))
            lastLives = lives
        end
    end
end

-- === plugin hooks ===
local function on_init()
    console.log("Mario Lives Tracker: Plugin initialized")
    check_game()
end

local function on_frame()
    frameCounter = frameCounter + 1

    -- Check for game change
    if (frameCounter % 300) == 0 then -- Check every 5 seconds
        check_game()
    end

    if (frameCounter % pollInterval) == 0 then
        track_lives()
    end
end

local function on_game_start(game_name)
    console.log("Mario Lives Tracker: Game started - " .. tostring(game_name))
    check_game()
    lastLives = nil -- Reset lives tracking for new game
end

-- Exported functions for the plugin system
local exported = {
    on_init = on_init,
    on_frame = on_frame,
    on_game_start = on_game_start
}

return exported
