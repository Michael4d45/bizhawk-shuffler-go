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
    ["legend of zelda, the - a link to the past (usa)"] = {
        addr = 0xA2,
        size = 2,
        domain = nil,
        desc = "room id"
    },
    ["super mario bros. 3"] = {
        domain = 'WRAM',
        addr = 0x545,
        size = 1,
        desc = "world/level"
    },
    ["legend of zelda, the - ocarina of time (usa)"] = {
        -- 81 - Hyrule Field, 52 - Link's House, 85 - Kokiri Forest
        addr = 0x1C8544,
        size = 2,
        domain = "RDRAM",
        desc = "Entrance Index",

        -- Only consider entrance changes when this guard word matches.
        -- In USA 1.0 retail, 001C8540 is 0x00000000 during normal gameplay,
        -- and nonzero on title/file-select/attract modes.
        guardWord = {
            addr = 0x1C8540,
            size = 4,
            equals = 0
        }
    },
    ["1942"] = {
        addr = 0x438,
        size = 1,
        domain = "RAM",
        desc = "current level (NES RAM) — DataCrystal RAM map ($0438 = Level)"
    },
    -- 8037D188 Current Animal
    -- 0x01 - Banjo-Kazooie
    -- 0x02 - Spider
    -- 0x03 - Pumpkin
    -- 0x04 - Walrus
    -- 0x05 - Crocodile
    -- 0x06 - Bee
    -- 0x07 - Washing Machine
    -- ["banjo-kazooie (usa)"] = {
    --     addr = 0x0037D188,
    --     size = 1,
    --     domain = "RDRAM",
    --     desc = "current animal (u8). RDRAM offset of virtual 0x8037D188 — DataCrystal / Hack64"
    -- },
    -- 
    ['banjo-kazooie (usa)'] = {
        addr = 0x36A9CC,
        size = 2,
        domain = "RDRAM",
        desc = "Location change"
    },
    -- https://www.chronocompendium.com/Forums/index.php?topic=1764.0
    ["chrono trigger (usa)"] = {
        addr = 0x100,
        size = 1,
        domain = "WRAM",
        desc = "map/room index (7E:0100 used by Chrono Compendium location offsets). Save-slot world in SRAM: 0x000005F3"
    },
    ["donkey kong country 2 - diddy's kong quest (usa) (en,fr)"] = {
        addr = 0xD3, -- ?
        size = 2,
        domain = "WRAM",
        desc = "current level id (WRAM) — p4plus2 / DonkeyHacks DKC2 RAM map"
    },
    ['felix the cat'] = {
        addr = 0x305,
        size = 1,
        domain = "WRAM",
        desc = "Location change"
    },
    ['pokemon - emerald version (usa, europe)'] = {
        addr = 0x5DC0,
        size = 2,
        domain = "IWRAM",
        desc = "Location change"
    },
    -- 5044 - routes; 5040 - towns
    ['pokemon - leafgreen version (usa)'] = {
        addr = 0x500D, -- ?
        size = 2,
        domain = "IWRAM",
        desc = "map number (u8) at 0300500D"
    },
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

-- Abstracted read that handles size and endianness defaults.
-- NES/SNES/GB/GBA/PS1 are little-endian; N64 is big-endian for 16/32-bit.
-- We don't auto-detect system; rely on BizHawk's API names where possible,
-- and fall back to manual composition (little-endian) if not available.
local function safe_read(addr, size, domain, big_endian)
    if not addr then
        return nil
    end
    size = size or 1
    -- 1 byte
    if size == 1 then
        local v = try_call(memory.readbyte, addr, domain)
        if v ~= nil then
            return v
        end
        v = try_call(memory.read_u8, addr, domain)
        if v ~= nil then
            return v
        end
        return nil
    end
    -- 2 bytes
    if size == 2 then
        if big_endian then
            local v = try_call(memory.read_u16_be, addr, domain)
            if v ~= nil then
                return v
            end
            -- fallback manual BE
            local b0 = try_call(memory.readbyte, addr, domain)
            local b1 = try_call(memory.readbyte, addr + 1, domain)
            if b0 and b1 then
                return b0 * 256 + b1
            end
        else
            local v = try_call(memory.read_u16_le, addr, domain)
            if v ~= nil then
                return v
            end
            -- fallback manual LE
            local b0 = try_call(memory.readbyte, addr, domain)
            local b1 = try_call(memory.readbyte, addr + 1, domain)
            if b0 and b1 then
                return b0 + b1 * 256
            end
        end
        return nil
    end
    -- 4 bytes
    if size == 4 then
        if big_endian then
            local v = try_call(memory.read_u32_be, addr, domain)
            if v ~= nil then
                return v
            end
            -- fallback manual BE
            local b0 = try_call(memory.readbyte, addr, domain)
            local b1 = try_call(memory.readbyte, addr + 1, domain)
            local b2 = try_call(memory.readbyte, addr + 2, domain)
            local b3 = try_call(memory.readbyte, addr + 3, domain)
            if b0 and b1 and b2 and b3 then
                return (((b0 * 256) + b1) * 256 + b2) * 256 + b3
            end
        else
            local v = try_call(memory.read_u32_le, addr, domain)
            if v ~= nil then
                return v
            end
            -- fallback manual LE
            local b0 = try_call(memory.readbyte, addr, domain)
            local b1 = try_call(memory.readbyte, addr + 1, domain)
            local b2 = try_call(memory.readbyte, addr + 2, domain)
            local b3 = try_call(memory.readbyte, addr + 3, domain)
            if b0 and b1 and b2 and b3 then
                return b0 + b1 * 256 + b2 * 65536 + b3 * 16777216
            end
        end
        return nil
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
        if dn:find("rdram") then
            bestDomainCached = d
            return d
        end
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

-- Determine endianness hint based on domain (simple heuristic):
-- N64 RDRAM uses big-endian for 16/32-bit values; most others are little-endian.
local function is_big_endian_domain(domain)
    if not domain then
        return false
    end
    local d = domain:lower()
    if d:find("rdram") then
        return true
    end
    return false
end

-- Evaluate optional guardWord: { addr, size=4, equals=value }
local function guard_allows(cfg)
    if not cfg.guardWord then
        return true
    end
    local g = cfg.guardWord
    local gsize = g.size or 4
    local gdomain = g.domain or cfg.domain
    local gbe = is_big_endian_domain(gdomain)
    local gv = safe_read(g.addr, gsize, gdomain, gbe)
    if gv == nil then
        return false
    end
    if g.equals ~= nil then
        return gv == g.equals
    end
    -- if equals not specified, treat nonzero as true
    return gv ~= 0
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

-- === core polling ===
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
        console.log("Read Door: No config found for current game: \"" .. tostring(curLower) .. "\"")
        return
    end

    -- Enforce guard(s) if present
    if not guard_allows(cfg) then
        return
    end

    -- Read value with appropriate endianness hint
    local be = is_big_endian_domain(cfg.domain or choose_domain_preference())
    local val = safe_read(cfg.addr, cfg.size or 1, cfg.domain, be)
    if val == nil then
        return
    end

    console.log(("Read Door: %s read addr=0x%X size=%d domain=%s value=%s"):format(tostring(currentGame),
        cfg.addr, cfg.size or 1, tostring(cfg.domain or "best-guess"), tostring(val)))

    local key = currentGame
    local last = lastValueByGame[key]
    if last ~= val then
        if last ~= nil then
            console.log(("Read Door: %s room value changed: %s -> %s (%s)"):format(tostring(currentGame),
                tostring(last), tostring(val), tostring(cfg.desc or "")))
            SendCommand("swap", {
                ["message"] = ("Read Door: %s room value changed: %s -> %s (%s)"):format(tostring(currentGame),
                    tostring(last), tostring(val), tostring(cfg.desc or ""))
            })
        end
        lastValueByGame[key] = val
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
        bestDomainCached = nil
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
