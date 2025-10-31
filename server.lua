-- BizHawk Lua: IPC command runner (server mode)
-- This variant listens for a controller (Go) to connect on localhost:55355
-- and implements the same CMD/ACK/NACK/HELLO/PING/PONG protocol as the
local socket = require("socket.core")
local HOST = "127.0.0.1"
local PORT = 55355
-- read port from lua_server_port.txt if it exists
do
    local f = io.open("lua_server_port.txt", "r")
    if f then
        local line = f:read("*l")
        f:close()
        local p = tonumber(line)
        if p and p > 0 and p < 65536 then
            PORT = p
        end
    end
end

local ROM_DIR = "./roms"
local SAVE_DIR = "./saves"
local PLUGIN_DIR = "./plugins"

console.log("Shuffler server starting (listening)...")

-- Simple plugin loading system (basic implementation)
local loaded_plugins = {}

local function file_exists(name)
    local f = io.open(name, "r")
    if f ~= nil then
        io.close(f)
        return true
    else
        return false
    end
end

local available_hooks = {"on_init", "on_frame", "on_settings_changed"}

-- Plugin hook functions
local function call_plugin_hook(hook_name, ...)
    for plugin_name, plugin_data in pairs(loaded_plugins) do
        local hook_func = plugin_data[hook_name]
        if hook_func then
            local ok, err = pcall(hook_func, ...)
            if not ok then
                console.log("Plugin " .. plugin_name .. " " .. hook_name .. " error: " .. tostring(err))
            end
        end
    end
end

-- Load a single plugin by name (used by load_plugins and reload_plugin_settings)
local function load_single_plugin(plugin_name, settings)
    local plugin_path = PLUGIN_DIR .. "/" .. plugin_name
    local plugin_lua_path = plugin_path .. "/plugin.lua"

    if not file_exists(plugin_lua_path) then
        console.log("Plugin " .. plugin_name .. " missing required files")
        return false
    end

    console.log("Found plugin: " .. plugin_name)

    -- Parse meta.kv if present (read-only metadata)
    local meta = {}
    local meta_path = plugin_path .. "/meta.kv"
    local mf = io.open(meta_path, "r")
    if mf then
        for line in mf:lines() do
            local l = line:match("^%s*(.-)%s*$")
            if l ~= "" then
                local eq = l:find("=")
                if eq then
                    local key = l:sub(1, eq - 1):gsub("^%s*(.-)%s*$", "%1"):lower()
                    local val = l:sub(eq + 1):gsub("^%s*(.-)%s*$", "%1")
                    meta[key] = val
                end
            end
        end
        mf:close()
    end

    -- Load the plugin Lua file
    local plugin_file = plugin_path .. "/plugin.lua"
    local plugin_ok, plugin_module = pcall(dofile, plugin_file)

    if plugin_ok and plugin_module then
        local is_valid = true
        for _, hook in ipairs(available_hooks) do
            if plugin_module[hook] and type(plugin_module[hook]) ~= "function" then
                console.log("Plugin " .. plugin_name .. " has invalid hook '" .. hook .. "'; must be a function")
                is_valid = false
            end
        end
        for k, v in pairs(plugin_module) do
            if type(v) == "function" then
                local is_hook = false
                for _, hook in ipairs(available_hooks) do
                    if k == hook then
                        is_hook = true
                        break
                    end
                end
                if not is_hook then
                    console.log("Plugin " .. plugin_name .. " has unknown function '" .. k .. "'; ignoring")
                end
            end
        end

        if not is_valid then
            console.log("Plugin " .. plugin_name .. " is invalid and will not be loaded")
            return false
        else
            -- Store settings with the plugin module so plugins can access them
            plugin_module._settings = settings
            plugin_module._meta = meta
            plugin_module._initialized = false -- Track initialization state
            loaded_plugins[plugin_name] = plugin_module

            -- Call on_init hook if available (only once per enablement)
            if plugin_module.on_init and not plugin_module._initialized then
                local init_ok, init_err = pcall(plugin_module.on_init)
                if init_ok then
                    plugin_module._initialized = true
                    console.log("Plugin " .. plugin_name .. " initialized successfully")
                else
                    console.log("Plugin " .. plugin_name .. " init error: " .. tostring(init_err))
                end
            end
            console.log("Plugin " .. plugin_name .. " loaded successfully")
            return true
        end
    else
        console.log("Failed to load plugin " .. plugin_name .. ": " .. tostring(plugin_module))
        return false
    end
end

-- Fully reload a plugin: reload plugin.lua file and settings
local function reload_plugin_file(plugin_name)
    console.log("Fully reloading plugin: " .. plugin_name)

    local plugin_path = PLUGIN_DIR .. "/" .. plugin_name
    local settings_path = plugin_path .. "/settings.kv"

    -- Load current settings first
    local settings = {}
    local sf = io.open(settings_path, "r")
    if sf then
        for line in sf:lines() do
            local l = line:match("^%s*(.-)%s*$")
            if l ~= "" then
                local eq = l:find("=")
                if eq then
                    local key = l:sub(1, eq - 1):gsub("^%s*(.-)%s*$", "%1")
                    local val = l:sub(eq + 1):gsub("^%s*(.-)%s*$", "%1")
                    settings[key] = val
                end
            end
        end
        sf:close()
    else
        console.log("Settings file not found for plugin: " .. plugin_name .. ", defaulting to disabled")
        settings["status"] = "disabled"
    end

    local plugin_status = (settings["status"] or "disabled"):lower()

    -- Only reload if plugin is enabled
    if plugin_status ~= "enabled" then
        console.log("Plugin " .. plugin_name .. " is disabled, skipping file reload")
        return
    end

    -- Get old plugin data if it exists
    local old_plugin_data = loaded_plugins[plugin_name]
    local was_initialized = false
    if old_plugin_data then
        was_initialized = old_plugin_data._initialized or false
        -- Remove old plugin to force reload
        loaded_plugins[plugin_name] = nil
    end

    -- Reload the plugin file
    if load_single_plugin(plugin_name, settings) then
        local plugin_data = loaded_plugins[plugin_name]
        if plugin_data then
            -- If it was previously initialized, call on_init again (reload means full restart)
            if plugin_data.on_init then
                local init_ok, init_err = pcall(plugin_data.on_init)
                if init_ok then
                    plugin_data._initialized = true
                    console.log("Plugin " .. plugin_name .. " re-initialized successfully")
                else
                    console.log("Plugin " .. plugin_name .. " init error: " .. tostring(init_err))
                end
            end

            -- Call on_settings_changed hook if available
            if plugin_data.on_settings_changed then
                local ok, err = pcall(plugin_data.on_settings_changed, settings)
                if not ok then
                    console.log("Plugin " .. plugin_name .. " on_settings_changed error: " .. tostring(err))
                else
                    console.log("Plugin " .. plugin_name .. " reloaded completely")
                end
            else
                console.log("Plugin " .. plugin_name .. " reloaded completely")
            end
        end
    else
        console.log("Failed to reload plugin " .. plugin_name)
    end
end

-- Reload plugin settings from settings.kv and notify plugin via hook
local function reload_plugin_settings(plugin_name)
    console.log("Reloading settings for plugin: " .. plugin_name)
    local plugin_data = loaded_plugins[plugin_name]

    local plugin_path = PLUGIN_DIR .. "/" .. plugin_name
    local settings_path = plugin_path .. "/settings.kv"
    console.log("Reading settings from: " .. settings_path)
    local settings = {}

    local sf = io.open(settings_path, "r")
    if sf then
        console.log("Settings file found for plugin: " .. plugin_name)
        local setting_count = 0
        for line in sf:lines() do
            local l = line:match("^%s*(.-)%s*$")
            if l ~= "" then
                local eq = l:find("=")
                if eq then
                    local key = l:sub(1, eq - 1):gsub("^%s*(.-)%s*$", "%1")
                    local val = l:sub(eq + 1):gsub("^%s*(.-)%s*$", "%1")
                    settings[key] = val
                    setting_count = setting_count + 1
                    console.log("  Setting: " .. key .. " = " .. val)
                end
            end
        end
        sf:close()
        console.log("Loaded " .. tostring(setting_count) .. " settings for plugin: " .. plugin_name)
    else
        console.log("Settings file not found for plugin: " .. plugin_name .. ", defaulting to disabled")
        settings["status"] = "disabled"
    end

    local old_status = "disabled"
    if plugin_data then
        old_status = (plugin_data._settings["status"] or "disabled"):lower()
    end
    local new_status = (settings["status"] or "disabled"):lower()

    -- If plugin is not loaded and status is enabled, load it now
    if not plugin_data and new_status == "enabled" then
        console.log("Plugin " .. plugin_name .. " transitioning from disabled to enabled, loading plugin")
        if load_single_plugin(plugin_name, settings) then
            plugin_data = loaded_plugins[plugin_name]
            -- on_init already called in load_single_plugin, so we're done
            -- But still call on_settings_changed if available
            if plugin_data and plugin_data.on_settings_changed then
                local ok, err = pcall(plugin_data.on_settings_changed, settings)
                if not ok then
                    console.log("Plugin " .. plugin_name .. " on_settings_changed error: " .. tostring(err))
                else
                    console.log("Plugin " .. plugin_name .. " settings reloaded and hook called")
                end
            end
        end
        return
    end

    -- If plugin is not loaded and still disabled, nothing to do
    if not plugin_data then
        console.log("Plugin " .. plugin_name .. " not loaded and disabled, nothing to do")
        return
    end

    -- If status changed from enabled to disabled, unload the plugin
    if old_status == "enabled" and new_status ~= "enabled" then
        console.log("Plugin " .. plugin_name .. " disabled, removing from loaded plugins")
        plugin_data._initialized = false -- Reset initialization flag
        loaded_plugins[plugin_name] = nil
        return
    end

    -- If status changed from disabled to enabled and plugin was already loaded, ensure it's initialized
    if old_status ~= "enabled" and new_status == "enabled" then
        console.log("Plugin " .. plugin_name .. " enabled, checking initialization")
        if plugin_data.on_init and not plugin_data._initialized then
            local init_ok, init_err = pcall(plugin_data.on_init)
            if init_ok then
                plugin_data._initialized = true
                console.log("Plugin " .. plugin_name .. " initialized successfully")
            else
                console.log("Plugin " .. plugin_name .. " init error: " .. tostring(init_err))
            end
        end
    end

    -- Update stored settings
    plugin_data._settings = settings

    -- Call on_settings_changed hook if available (only if still enabled)
    if new_status == "enabled" then
        if plugin_data.on_settings_changed then
            local ok, err = pcall(plugin_data.on_settings_changed, settings)
            if not ok then
                console.log("Plugin " .. plugin_name .. " on_settings_changed error: " .. tostring(err))
            else
                console.log("Plugin " .. plugin_name .. " settings reloaded and hook called")
            end
        else
            console.log("Plugin " .. plugin_name .. " settings reloaded (no on_settings_changed hook)")
        end
    end
end

local function now()
    return socket.gettime()
end

local messages = {}
local function show_message(text, duration, x, y, fontsize, fg, bg)
    duration = tonumber(duration) or 3.0
    table.insert(messages, {
        text = text or "",
        expires = now() + duration,
        x = x or 10,
        y = y or 10,
        fontsize = fontsize or 12,
        fg = fg or 0xFFFFFFFF,
        bg = bg or 0xFF000000
    })
end

local function draw_messages()
    gui.clearGraphics()
    if #messages == 0 then
        return
    end
    gui.use_surface("client")
    local t = now()
    local keep = {}
    local yoff = 0
    for _, m in ipairs(messages) do
        if t < m.expires then
            gui.drawText(m.x, m.y + yoff, m.text, m.fg, m.bg, m.fontsize)
            table.insert(keep, m)
            yoff = yoff + (m.fontsize + 4)
        end
    end
    messages = keep
end

local function save_state(path)
    if not path then
        console.log("No valid save path; skipping save")
        return
    end
    console.log("Saving state to: " .. tostring(path))
    local ok, err = pcall(function()
        savestate.save(path)
    end)
    if not ok then
        console.log("Failed to save state to '" .. tostring(path) .. "': " .. tostring(err))
        console.log("Save operation failed, but continuing...")
    end
end

local function sanitize_filename(name)
    if not name then
        return nil
    end
    name = name:gsub("[/\\:%*?\"<>|]", "_")
    name = name:gsub("%s+$", "")
    return name
end

local function get_save_path()
    local cur = gameinfo.getromname()
    cur = sanitize_filename(cur)
    local name = ""
    if cur and cur ~= "" and cur:lower() ~= "null" then
        name = cur
    end
    if InstanceID and InstanceID ~= "" then
        name = InstanceID
    end

    if not name or name == "" or name:lower() == "null" then
        return nil
    end

    return SAVE_DIR .. "/" .. name .. ".state"
end

local function is_valid_zip(path)
    local f = io.open(path, "rb")
    if not f then
        return false
    end
    local size = f:seek("end")
    if size < 22 then -- Minimum ZIP footer length
        f:close()
        return false
    end
    local chunk_size = math.min(65536, size)
    f:seek("end", -chunk_size)
    local tail = f:read(chunk_size)
    f:close()
    if not tail then
        return false
    end
    return tail:find("PK\005\006", 1, true) ~= nil
end

local function load_state_if_exists()
    local path = get_save_path()
    if path and file_exists(path) then
        console.log("Loading state from: " .. tostring(path))
        if is_valid_zip(path) then
            local ok, err = pcall(function()
                savestate.load(path)
            end)
            if not ok then
                console.log("Failed to load state from '" .. tostring(path) .. "': " .. tostring(err))
                os.remove(path)
            end
        else
            console.log("Invalid ZIP structure; deleting save: " .. tostring(path))
            os.remove(path)
        end
    end
end

local function load_rom(game)
    local path = ROM_DIR .. "/" .. game
    client.closerom()
    if file_exists(path) then
        client.openrom(path)
        local ok, err = pcall(load_state_if_exists)
        if not ok then
            console.log("Error loading state: " .. tostring(err))
        end
        return true
    else
        console.log("ROM not found: " .. path .. ", cannot load.")
        return false
    end
end

local function strip_extension(filename)
    return (filename:gsub("%.[^%.]+$", ""))
end

-- Command implementations
InstanceID = nil
-- Compute a canonical id for a game based on display name or filename
local function canonical_game_id_from_display(name)
    if not name then
        return nil
    end
    name = sanitize_filename(name)
    if not name or name == "" then
        return nil
    end
    return name:lower()
end

local function canonical_game_id_from_filename(filename)
    if not filename then
        return nil
    end
    local base = strip_extension(filename)
    base = sanitize_filename(base)
    if not base or base == "" then
        return nil
    end
    return base:lower()
end

local function get_current_canonical_game()
    local disp = gameinfo.getromname()
    local id = canonical_game_id_from_display(disp)
    if id and id ~= "" then
        return id
    end
    return nil
end

local function do_save()
    save_state(get_save_path())
end

local function do_swap(target_game, instance, skip_check)
    console.log("Starting swap to game: " .. tostring(target_game) .. " with instance: " .. tostring(instance))

    -- Wrap the entire swap operation in error handling
    local swap_ok, swap_err = pcall(function()
        local cur_id = get_current_canonical_game()
        local target_id = canonical_game_id_from_filename(target_game) or canonical_game_id_from_display(target_game)
        local old_save_path = get_save_path()

        InstanceID = instance

        local new_save_path = get_save_path()
        if (target_id and cur_id and target_id == cur_id and old_save_path == new_save_path) and not skip_check then
            -- same canonical game; skip reload
            console.log("Swap skipped: target is same as current (" .. tostring(target_id) .. ")")
            return
        end
        load_rom(target_game)
    end)

    if not swap_ok then
        console.log("Swap operation failed: " .. tostring(swap_err))
        console.log("Attempting to continue with current game state...")
    else
        console.log("Swap completed successfully")
    end
end

local function do_pause()
    client.pause();
    console.log("[INFO] Paused")
end

local function do_resume()
    client.unpause();
    console.log("[INFO] Resumed")
end

-- Networking: listen for controller connections (Go process will connect)
-- Some BizHawk builds expose socket.core but do not provide a top-level bind function.
-- Create a TCP socket and bind/listen using the tcp() object when available.
local server = nil
do
    local ok, s = pcall(function()
        return socket.bind
    end)
    if ok and s then
        server = assert(socket.bind(HOST, PORT))
    else
        local c = socket.tcp()
        if c then
            local bind_ok, bind_err = pcall(function()
                return c:bind(HOST, PORT)
            end)
            if not bind_ok then
                local listen_ok, listen_err = pcall(function()
                    return c:listen(PORT)
                end)
                if not listen_ok then
                    error("socket bind/listen not available: " .. tostring(bind_err or listen_err))
                end
            else
                pcall(function()
                    c:listen()
                end)
            end
            server = c
        else
            error("socket.tcp() returned nil; socket API unavailable")
        end
    end
end
server:settimeout(0) -- non-blocking accept
console.log("Listening on " .. HOST .. ":" .. tostring(PORT))

local client_socket = nil

local function send_line(line)
    console.log("Sending: " .. tostring(line))
    if client_socket then
        local ok, err = pcall(function()
            client_socket:send(line .. "\n")
        end)
        if not ok then
            console.log("send error: " .. tostring(err))
        end
    end
end

local function escape(s)
    return (s:gsub("\\", "\\\\"):gsub("|", "\\|"):gsub(";", "\\;"):gsub("=", "\\="))
end

local function serialize_payload_escaped(payload)
    if payload == nil then
        return ""
    end
    local parts = {}
    for k, v in pairs(payload) do
        local t = type(v)
        if t == "boolean" then
            v = v and "true" or "false"
        elseif t ~= "number" and t ~= "string" then
            v = tostring(v)
        end
        table.insert(parts, escape(tostring(k)) .. "=" .. escape(tostring(v)))
    end
    return table.concat(parts, ";")
end

function SendCommand(cmd, payload)
    -- Choose one of the serializers:
    -- local payload_str = serialize_payload(payload)
    local payload_str = serialize_payload_escaped(payload)

    local cmd_str = "CMD|" .. tostring(cmd) .. "|" .. payload_str
    send_line(cmd_str)
end

-- send HELLO to controller side when ready
local function send_hello()
    send_line("HELLO")
end

local function safe_exec_and_ack(id, fn)
    local ok, err = pcall(fn)
    if ok then
        send_line("ACK|" .. id)
    else
        send_line("NACK|" .. id .. "|" .. tostring(err))
    end
end

local function split_pipe(s)
    -- Split on '|' and preserve empty fields. The previous implementation
    -- used "([^|]+)" which skipped empty segments (consecutive pipes),
    -- causing argument positions to shift when fields are empty.
    local parts = {}
    local last = 1
    while true do
        local startpos, endpos = string.find(s, "|", last, true)
        if not startpos then
            table.insert(parts, string.sub(s, last))
            break
        end
        table.insert(parts, string.sub(s, last, startpos - 1))
        last = endpos + 1
    end
    return parts
end

local function handle_line(line)
    local parts = split_pipe(line)
    if #parts == 0 then
        return
    end
    console.log(parts)
    if parts[1] == "CMD" then
        local id, cmd = parts[2], parts[3]
        if cmd == "SAVE" then
            safe_exec_and_ack(id, function()
                do_save()
            end)
        elseif cmd == "LOAD" then
            safe_exec_and_ack(id, function()
                do_swap(parts[4], parts[5], true)
            end)
        elseif cmd == "SWAP" then
            safe_exec_and_ack(id, function()
                do_swap(parts[4], parts[5], false)
            end)
        elseif cmd == "PAUSE" then
            safe_exec_and_ack(id, function()
                do_pause()
            end)
        elseif cmd == "RESUME" then
            safe_exec_and_ack(id, function()
                do_resume()
            end)
        elseif cmd == "MSG" then
            safe_exec_and_ack(id, function()
                show_message(parts[4], tonumber(parts[5]), tonumber(parts[6]), tonumber(parts[7]), tonumber(parts[8]),
                    parts[9], parts[10])
            end)
        elseif cmd == "PLUGIN_SETTINGS" then
            safe_exec_and_ack(id, function()
                local plugin_name = parts[4]
                if plugin_name and plugin_name ~= "" then
                    reload_plugin_settings(plugin_name)
                else
                    console.log("PLUGIN_SETTINGS command missing plugin name")
                end
            end)
        elseif cmd == "PLUGIN_RELOAD" then
            safe_exec_and_ack(id, function()
                local plugin_name = parts[4]
                if plugin_name and plugin_name ~= "" then
                    reload_plugin_file(plugin_name)
                else
                    console.log("PLUGIN_RELOAD command missing plugin name")
                end
            end)
        else
            send_line("NACK|" .. id .. "|Unknown command: " .. tostring(cmd))
        end
    elseif parts[1] == "PING" then
        -- reply PONG|<timestamp>
        if parts[2] then
            send_line("PONG|" .. parts[2])
        else
            send_line("PONG|" .. tostring(math.floor(now())))
        end
    end
end

local function load_plugins()
    console.log("Scanning plugins directory...")

    -- Get list of plugin directories
    local plugin_dirs = {}
    local plugin_dir_handle = io.popen('dir /b "' .. PLUGIN_DIR .. '" 2>nul')
    if plugin_dir_handle then
        for line in plugin_dir_handle:lines() do
            if line ~= "" then
                table.insert(plugin_dirs, line)
            end
        end
        plugin_dir_handle:close()
    end

    -- Load each plugin
    for _, plugin_name in ipairs(plugin_dirs) do
        local plugin_path = PLUGIN_DIR .. "/" .. plugin_name
        local settings_path = plugin_path .. "/settings.kv"

        -- Parse settings.kv to get status and other settings
        console.log("Loading settings for plugin: " .. plugin_name)
        local settings = {}
        console.log("Reading settings from: " .. settings_path)
        local sf = io.open(settings_path, "r")
        if sf then
            console.log("Settings file found for plugin: " .. plugin_name)
            local setting_count = 0
            for line in sf:lines() do
                local l = line:match("^%s*(.-)%s*$")
                if l ~= "" then
                    local eq = l:find("=")
                    if eq then
                        local key = l:sub(1, eq - 1):gsub("^%s*(.-)%s*$", "%1")
                        local val = l:sub(eq + 1):gsub("^%s*(.-)%s*$", "%1")
                        settings[key] = val
                        setting_count = setting_count + 1
                        console.log("  Setting: " .. key .. " = " .. val)
                    end
                end
            end
            sf:close()
            console.log("Loaded " .. tostring(setting_count) .. " settings for plugin: " .. plugin_name)
        else
            -- Default to disabled if settings.kv doesn't exist
            console.log("Settings file not found for plugin: " .. plugin_name .. ", defaulting to disabled")
            settings["status"] = "disabled"
        end

        -- Check if plugin is enabled
        local plugin_status = (settings["status"] or "disabled"):lower()
        if plugin_status ~= "enabled" then
            console.log("Plugin " .. plugin_name .. " is disabled (status=" .. plugin_status .. "), skipping")
        else
            -- Load the plugin using the shared function
            load_single_plugin(plugin_name, settings)
        end
    end

    console.log("Plugin loading complete. Loaded " .. tostring(#loaded_plugins) .. " plugins")
end

-- Initialize plugin system
load_plugins()

-- Main loop: accept connection, then read lines non-blocking and process scheduled tasks
local next_auto_save = now() + 10.0
while true do
    if not client_socket then
        console.log("Waiting for controller to connect...")
        local c = server:accept()
        if c then
            console.log("Controller connected")
            client_socket = c
            client_socket:settimeout(0)
            send_hello()
        end
    else
        -- read lines
        local line, err = client_socket:receive("*l")
        if line then
            handle_line(line)
        else
            if err == "timeout" then
                -- nothing to read
            elseif err == "closed" then
                console.log("Controller disconnected")
                client_socket:close()
                client_socket = nil
            else
                -- other errors
                console.log("socket recv err: " .. tostring(err))
            end
        end
    end

    local t = now()
    if t >= next_auto_save then
        -- autosave current if any
        pcall(function()
            do_save()
        end)
        next_auto_save = t + 10.0
    end

    draw_messages()

    -- Call plugin frame hook
    call_plugin_hook("on_frame")

    if client.ispaused() then
        emu.yield()
    else
        emu.frameadvance()
    end
end
