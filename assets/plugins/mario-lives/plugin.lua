-- Mario Lives Tracker plugin for BizHawk Shuffler
-- Detects Super Mario Bros 3 and tracks the number of lives
-- Automatically cycles lives back to 4 when they reach 1
--
-- Usage:
--  - Automatically detects Super Mario Bros 3 games
--  - Reads lives from RAM address 0x0736
--  - Logs lives changes to console
--  - Cycles lives back to 4 when reaching 1
--
console.log("Mario Lives Tracker: Plugin loaded!")

-- === CONFIG ===
local pollInterval = 300 -- frames between checks
local targetGame = "super mario bros. 3" -- substring to match game name
local addressLives = 0x0736 -- RAM address for lives in SMB3
local domain = "RAM"

-- === internal state ===
local isTargetGame = false
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
    -- Value ranges from 0-99 (0 = game over)
    local lives = try_call(memory.readbyte, addressLives, domain)
    return lives
end

local function write_lives(lives)
    local success = try_call(memory.writebyte, addressLives, lives, domain)
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
    check_game()
    if not isTargetGame then
        return
    end

    local lives = read_lives()
    if lives == nil then
        return
    end
    -- Cycle back to 4 lives when reaching 1
    if lives <= 1 then
        write_lives(4)
    end
end

local function on_frame()
    frameCounter = frameCounter + 1

    if (frameCounter % pollInterval) == 0 then
        track_lives()
    end
end

-- Exported functions for the plugin system
local exported = {
    on_frame = on_frame
}

return exported
