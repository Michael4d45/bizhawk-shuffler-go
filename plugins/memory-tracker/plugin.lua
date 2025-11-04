-- Memory Tracker plugin for BizHawk Shuffler
-- Tracks multiple memory address types (door, health, etc.) per game
-- with configurable monitoring per type
--
-- Usage:
--  - Configure enabled types in settings.kv (e.g., enabled_types = door,health)
--  - Add games to the `games` table with structure:
--    games["game name"] = {
--        door = { addr = <number>, size = 1|2|4, domain = <optional>, desc = "notes" },
--        health = { addr = <number>, size = 1|2|4, domain = <optional>, desc = "notes" },
--        ...
--    }
--  - Each type can have its own address configuration, guardWord, equals check, etc.
--
-- Notes:
--  - Addresses are ROM/core-specific. Use BizHawk's Hex Editor to find addresses.
--  - Multiple types can be tracked simultaneously per game.
--  - Only enabled types (from settings) will be monitored.
console.log("Memory Tracker: Plugin loaded!")

-- === CONFIG ===
local pollInterval = 60 -- frames between checks (adjust as needed)

-- Per-game config with support for multiple types per game.
-- Structure: games["game name"][type] = { addr, size, domain, desc, guardWord, equals, ... }
local games = {
    ["legend of zelda, the - a link to the past (usa)"] = {
        door = {
            addr = 0xA2,
            size = 2,
            domain = nil,
            desc = "room id"
        }
    },
    ["super mario bros. 3"] = {
        door = {
            domain = 'WRAM',
            addr = 0x545,
            size = 1,
            desc = "world/level"
        }
    },
    ["legend of zelda, the - ocarina of time (usa)"] = {
        door = {
            -- 81 - Hyrule Field, 52 - Link's House, 85 - Kokiri Forest
            addr = 0x1C8544,
            size = 2,
            domain = "RDRAM",
            desc = "Entrance Index",
            -- Only consider entrance changes when this guard word matches.
            guardWord = {
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
    },
    ["1942"] = {
        door = {
            addr = 0x438,
            size = 1,
            domain = "RAM",
            desc = "current level (NES RAM) — DataCrystal RAM map ($0438 = Level)"
        }
    },
    ['banjo-kazooie (usa)'] = {
        door = {
            addr = 0x36A9CC,
            size = 2,
            domain = "RDRAM",
            desc = "Location change"
        }
    },
    ["chrono trigger (usa)"] = {
        door = {
            addr = 0x100,
            size = 1,
            domain = "WRAM",
            desc = "map/room index (7E:0100 used by Chrono Compendium location offsets). Save-slot world in SRAM: 0x000005F3"
        }
    },
    ["donkey kong country 2 - diddy's kong quest (usa) (en,fr)"] = {
        door = {
            addr = 0x94, -- (level?) other candidates: 96 (level?), D3 (sublevel), F, 1C
            size = 2,
            domain = "WRAM",
            desc = "current level id (WRAM) — p4plus2 / DonkeyHacks DKC2 RAM map"
        }
    },
    ['felix the cat'] = {
        door = {
            addr = 0x305,
            size = 1,
            domain = "WRAM",
            desc = "Location change"
        }
    },
    ['pokemon - emerald version (usa, europe)'] = {
        door = {
            addr = 0x5DC0,
            size = 2,
            domain = "IWRAM",
            desc = "Location change"
        }
    },
    ['pokemon - leafgreen version (usa)'] = {
        door = {
            addr = 0x5044,
            size = 2,
            domain = "IWRAM",
            desc = "map number (u8) at 03005044"
        }
    },
    ['harry potter and the sorcerer\'s stone (usa, europe) (en,fr,de,es,it,pt,nl,sv,no,da,fi)'] = {
        door = {
            addr = 0x66D2, -- 66D0 stores the last room, 66D2 the current room
            size = 1,
            domain = "WRAM",
            desc = "current room id"
        }
    },
    ['mega man battle network (usa)'] = {
        door = {
            addr = 0x24A,
            size = 1,
            domain = "EWRAM",
            desc = "current room id"
        }
    },
    ['kirby - nightmare in dream land (usa)'] = {
        door = {
            addr = 0xAFF4,
            size = 1,
            domain = "EWRAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the - phantom hourglass (usa) (en,fr,es)'] = {
        door = {
            addr = 0x1B2F36,
            equals = 0x1,
            size = 1,
            domain = "Main RAM",
            desc = "transition screen"
        }
    },
    ['legend of zelda, the - spirit tracks (usa) (en,fr,es) (rev 1)'] = {
        door = {
            addr = 0x0B5298,
            equals = 0x1,
            size = 1,
            domain = "Main RAM",
            desc = "transition screen"
        }
    },
    ['legend of zelda, the - spirit tracks (usa) (video)'] = {
        door = {
            addr = 0x7D80,
            size = 1,
            domain = "ARM7 WRAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the - twilight princess - zelda gallery (usa) (kiosk) (e3 2005)'] = {
        door = {
            addr = 0xAF30,
            size = 1,
            domain = "ARM7 WRAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the (gc)'] = {
        door = {
            addr = 0xEC,
            size = 1,
            domain = "RAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the - link\'s awakening dx (usa)'] = {
        door = {
            addr = 0x1404,
            size = 2,
            domain = "WRAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the - oracle of seasons (usa)'] = {
        door = {
            addr = 0xC63,
            size = 4,
            domain = "WRAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the - oracle of ages (usa)'] = {
        door = {
            addr = 0xC34,
            size = 4,
            domain = "WRAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the - the minish cap (usa)'] = {
        door = {
            addr = 0x17650,
            size = 4,
            domain = "WRAM",
            desc = "current room id"
        }
    },
    ['legend of zelda, the - majora\'s mask (usa)'] = {
        door = {
            addr = 0x1F342B,
            equals = 0x1,
            size = 4,
            domain = "RDRAM",
            desc = "transition screen"
        }
    }
}

-- Optional: one-shot automatic probe after game start to discover changed bytes.
local AUTO_PROBE_ON_START = false
local PROBE_PARAMS = {
    domain = nil,
    start = 0,
    len = 4096,
    framesDelay = 20
}

-- === internal state ===
local currentGame = nil
local frameCounter = 0
-- Track last values per game and type: lastValueByGame[gameKey][type] = value
local lastValueByGame = {}
local bestDomainCached = nil
local lastRomName = nil
local lastInstanceID = nil
-- Module-level settings storage (will be populated by server or on_settings_changed)
-- Note: Not local so it can be accessed/modified by server.lua's plugin_module._settings
_settings = _settings or {}
-- Forward reference to exported table for syncing settings
local exported = nil

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
    return gv ~= 0
end

-- Parse enabled types from settings
local function get_enabled_types()
    local enabled = {}

    -- Try to sync _settings from exported table if not populated
    -- (server sets plugin_module._settings after load, but might not call on_settings_changed)
    if (not _settings or not _settings["enabled_types"]) and exported and exported._settings then
        _settings = exported._settings
    end

    if not _settings or not _settings["enabled_types"] then
        -- Default to "door" for backward compatibility
        enabled["door"] = true
        console.log("Memory Tracker: No enabled_types setting, defaulting to door")
        return enabled
    end

    local types_str = _settings["enabled_types"] or ""

    -- Use csv_to_array helper function
    local types_array = csv_to_array(types_str)
    for _, type_str in ipairs(types_array) do
        enabled[type_str:lower()] = true
    end

    -- If no types found, default to door
    if not next(enabled) then
        enabled["door"] = true
        console.log("Memory Tracker: No types parsed, defaulting to door")
    end

    return enabled
end

local function do_send(last, val, cfg, type_name)
    console.log(("Memory Tracker: %s %s value changed: %s -> %s (%s)"):format(tostring(currentGame),
        tostring(type_name), tostring(last), tostring(val), tostring(cfg.desc or "")))

    local command_type = _settings and _settings["command_type"] or "swap_me"

    SendCommand(command_type, {
        ["message"] = ("Memory Tracker: %s %s value changed: %s -> %s (%s)"):format(tostring(currentGame),
            tostring(type_name), tostring(last), tostring(val), tostring(cfg.desc or ""))
    })
end

-- === core polling ===
local function trackMemory()
    if not currentGame or not lastRomName then
        return
    end

    local gameConfig = nil
    local curLower = lastRomName:lower()
    for key, v in pairs(games) do
        if curLower:find(key:lower(), 1, true) then
            gameConfig = v
            break
        end
    end

    if not gameConfig then
        return
    end

    local enabledTypes = get_enabled_types()

    -- Track each enabled type for this game
    for type_name, type_cfg in pairs(gameConfig) do
        if enabledTypes[type_name:lower()] and type_cfg.addr then
            -- Enforce guard(s) if present
            if not guard_allows(type_cfg) then
                goto continue
            end

            -- Read value with appropriate endianness hint
            local be = is_big_endian_domain(type_cfg.domain or choose_domain_preference())
            local val = safe_read(type_cfg.addr, type_cfg.size or 1, type_cfg.domain, be)
            if val == nil then
                goto continue
            end

            -- Log read operations at debug level (commented out for production)
            -- console.log(("Memory Tracker: %s %s read addr=0x%X size=%d domain=%s value=%s"):format(
            --     tostring(currentGame), tostring(type_name), type_cfg.addr, type_cfg.size or 1,
            --     tostring(type_cfg.domain or "best-guess"), tostring(val)))

            local key = currentGame
            if not lastValueByGame[key] then
                lastValueByGame[key] = {}
            end
            local last = lastValueByGame[key][type_name]

            if last ~= val then
                if type_cfg.equals then -- only trigger if equals specified value
                    if val ~= type_cfg.equals then
                        goto continue
                    elseif last ~= type_cfg.equals then
                        do_send(last, val, type_cfg, type_name)
                    end
                else -- check for any change
                    if last ~= nil then
                        do_send(last, val, type_cfg, type_name)
                    end
                end
                lastValueByGame[key][type_name] = val
            end
        end
        ::continue::
    end
end

-- === plugin hooks ===
local function on_init()
    console.log("Memory Tracker: Plugin initialized")
end

local function on_settings_changed(settings)
    -- Update _settings with new settings
    if settings then
        _settings = settings
        -- Log enabled types summary
        local enabledTypes = get_enabled_types()
        local typesList = {}
        for k, v in pairs(enabledTypes) do
            table.insert(typesList, k)
        end
        console.log("Memory Tracker: Settings updated, enabled types: " .. table.concat(typesList, ", "))
    else
        console.log("Memory Tracker: on_settings_changed called with nil settings!")
    end
end

local function on_frame()
    frameCounter = frameCounter + 1

    -- Sync _settings from exported table if needed (server sets plugin_module._settings after load)
    if not _settings or not _settings["enabled_types"] then
        if exported and exported._settings then
            _settings = exported._settings
        end
    end
    local rom = gameinfo.getromname()
    if rom and (rom ~= lastRomName or InstanceID ~= lastInstanceID) then
        console.log("Memory Tracker: Game/Instance changed to " .. tostring(rom) .. " instance " .. tostring(InstanceID))
        lastRomName = rom
        lastInstanceID = InstanceID
        currentGame = lastRomName .. "_" .. (InstanceID or "")
        lastValueByGame[currentGame] = {}
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
            console.log(("Memory Tracker: probe finished for domain=%s start=0x%X len=%d changed=%d"):format(tostring(
                probe.domain), probe.start, probe.len, #changed))
            for _, c in ipairs(changed) do
                console.log(string.format("  0x%X : %s -> %s", c.addr, tostring(c.old), tostring(c.new)))
            end
            probe.active = false
        end
    end

    if (frameCounter % pollInterval) == 0 then
        trackMemory()
    end
end

-- Exported helpers for manual use
exported = {
    on_init = on_init,
    on_frame = on_frame,
    on_settings_changed = on_settings_changed
}

return exported

