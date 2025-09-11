-- ReadDoor plugin for BizHawk Shuffler
-- Detects "room/scene" changes by reading a configured RAM address per-game
--
-- Usage:
--  - Add games to the `games` table (key: substring of the game name BizHawk uses,
--    value: { addr = <number>, size = 1|2|4, domain = <optional domain string>, desc = "notes" } )
--  - Or enable AUTO_PROBE_ON_START to run a one-shot probe that reports changed
--    bytes in a region (useful to find the correct address for a given game).
--
-- Notes:
--  - Addresses are ROM/core-specific. Use BizHawk's Hex Editor + the probe
--    below to find candidate addresses, or look up a RAM map (datacrystal, ROM-hacking wiki, etc).
--  - This script is conservative with memory API calls (uses pcall where appropriate).
console.log("Read Door: Plugin loaded!")

-- === CONFIG ===
local pollInterval = 60 -- frames between checks (adjust as needed)

-- Per-game config (keys are substrings matched against the game name string passed
-- to on_game_start). Leave empty and use AUTO_PROBE if you need to discover addresses.
local games = {
    -- Example format (replace with real addresses for your games):
    -- ["super metroid"] = { addr = 0x7E0F30, size = 2, domain = nil,
    --                       desc = "room id (example - replace with verified address)" },
    -- ["zelda a link to the past"] = { addr = 0x7E0ABC, size = 1, domain = nil, desc = "tilemap/room id" },
    ["legend of zelda, the - a link to the past (usa)"] = {
        addr = 0xA2,
        size = 2,
        domain = nil,
        desc = "room id"
    }
}

-- Optional: one-shot automatic probe after game start to discover changed bytes.
-- Set to true and set PROBE_PARAMS appropriately if you want the plugin to scan.
local AUTO_PROBE_ON_START = false
local PROBE_PARAMS = {
    domain = nil, -- nil => best-guess domain chosen automatically
    start = 0, -- start offset inside domain
    len = 4096, -- how many bytes to sample
    framesDelay = 20 -- how many frames to wait before taking 'after' snapshot
}

-- === internal state ===
local currentGame = nil
local frameCounter = 0
local lastValueByGame = {}
local bestDomainCached = nil
local lastRomName = nil
local lastInstanceID = nil

local probe = {
    active = false,
    domain = nil,
    start = 0,
    len = 0,
    framesToWait = 0,
    snapshot = nil
}

-- === helpers ===
local function try_call(fn, ...)
    local ok, res = pcall(fn, ...)
    if ok then
        return res
    end
    return nil
end

local function safe_read(addr, size, domain)
    if not addr then
        return nil
    end
    -- prefer memory.readbyte for byte, memory.read_u16_le / read_u32_le where available
    if size == nil or size == 1 then
        local v = try_call(memory.readbyte, addr, domain)
        if v ~= nil then
            return v
        end
        v = try_call(memory.read_u8, addr, domain)
        if v ~= nil then
            return v
        end
    elseif size == 2 then
        local v = try_call(memory.read_u16_le, addr, domain)
        if v ~= nil then
            return v
        end
        -- fallback to two bytes (LE)
        local b0 = try_call(memory.readbyte, addr, domain)
        local b1 = try_call(memory.readbyte, addr + 1, domain)
        if b0 and b1 then
            return b0 + b1 * 256
        end
    elseif size == 4 then
        local v = try_call(memory.read_u32_le, addr, domain)
        if v ~= nil then
            return v
        end
    end
    return nil
end

local function choose_domain_preference()
    if bestDomainCached then
        return bestDomainCached
    end
    local domains = memory.getmemorydomainlist()
    if not domains or #domains == 0 then
        return nil
    end
    for _, d in ipairs(domains) do
        local dn = d:lower()
        if dn:find("wram") or dn:find("main") or dn:find("ram") or dn:find("system") then
            bestDomainCached = d
            return d
        end
    end
    bestDomainCached = domains[1]
    return bestDomainCached
end

-- Start a one-shot probe: sample region now, then sample again after framesDelay frames and report changed bytes
local function start_probe(domain, startaddr, len, framesDelay)
    probe.active = true
    probe.domain = domain or choose_domain_preference()
    probe.start = startaddr or 0
    probe.len = math.max(1, math.min(len or 4096, 65536))
    probe.framesToWait = framesDelay or 20
    probe.snapshot = {}
    for i = 0, probe.len - 1 do
        local b = try_call(memory.readbyte, probe.start + i, probe.domain)
        probe.snapshot[i] = b or 0
    end
    console.log(("Read Door: probe started domain=%s start=0x%X len=%d wait=%d"):format(tostring(probe.domain),
        probe.start, probe.len, probe.framesToWait))
end

-- Called every poll interval to check for room change
local function readDoor()
    if not currentGame or not lastRomName then
        return
    end

    local cfg = nil
    local curLower = lastRomName:lower()
    for key, v in pairs(games) do
        if curLower:find(key:lower(), 1, true) then
            cfg = v;
            break
        end
    end

    if not cfg or not cfg.addr then
        return
    end

    local val = safe_read(cfg.addr, cfg.size or 1, cfg.domain)
    if val == nil then
        return
    end
    local key = currentGame
    local last = lastValueByGame[key]
    lastValueByGame[key] = val
    if last ~= val and last ~= nil then
        console.log(("Read Door: %s room value changed: %s -> %s (%s)"):format(tostring(currentGame), tostring(last),
            tostring(val), tostring(cfg.desc or "")))
        lastValueByGame[key] = val

        SendCommand("swap", {
            ["message"] = ("Read Door: %s room value changed: %s -> %s (%s)"):format(tostring(currentGame),
                tostring(last), tostring(val), tostring(cfg.desc or ""))
        })
    end
end

-- === plugin hooks ===
local function on_init()
    console.log("Read Door: Plugin initialized")
end

local function on_frame()
    frameCounter = frameCounter + 1

    -- Check for game change
    local rom = gameinfo.getromname()
    if rom and (rom ~= lastRomName or InstanceID ~= lastInstanceID) then
        console.log("Read Door: Game/Instance changed to " .. tostring(rom) .. " instance " .. tostring(InstanceID))
        lastRomName = rom
        lastInstanceID = InstanceID
        currentGame = lastRomName .. "_" .. (InstanceID or "")
        lastValueByGame[currentGame] = nil
    end

    -- probe state machine
    if probe.active then
        if probe.framesToWait > 0 then
            probe.framesToWait = probe.framesToWait - 1
        else
            local changed = {}
            for i = 0, probe.len - 1 do
                local b = try_call(memory.readbyte, probe.start + i, probe.domain) or 0
                local prev = probe.snapshot[i] or 0
                if b ~= prev then
                    changed[#changed + 1] = {
                        addr = probe.start + i,
                        old = prev,
                        new = b
                    }
                end
            end
            console.log(("Read Door: probe finished for domain=%s start=0x%X len=%d changed=%d"):format(tostring(
                probe.domain), probe.start, probe.len, #changed))
            for _, c in ipairs(changed) do
                console.log(string.format("  0x%X : %s -> %s", c.addr, tostring(c.old), tostring(c.new)))
            end
            probe.active = false
        end
    end

    if (frameCounter % pollInterval) == 0 then
        readDoor()
    end
end

local function on_game_start(game_name)
    console.log("Read Door: Game started - " .. tostring(game_name))
    currentGame = game_name .. "_" .. (InstanceID or "")
    lastValueByGame[currentGame] = nil
    bestDomainCached = nil
    lastRomName = game_name
    lastInstanceID = InstanceID
    -- optional auto-probe
    if AUTO_PROBE_ON_START then
        start_probe(PROBE_PARAMS.domain, PROBE_PARAMS.start, PROBE_PARAMS.len, PROBE_PARAMS.framesDelay)
    end
end

-- Exported helpers for manual use by whomever calls this plugin table:
local exported = {
    on_init = on_init,
    on_frame = on_frame,
    on_game_start = on_game_start
}

return exported
