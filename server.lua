-- BizHawk Lua: IPC command runner (server mode)
-- This variant listens for a controller (Go) to connect on localhost:55355
-- and implements the same CMD/ACK/NACK/HELLO/PING/PONG protocol as the
local socket = require("socket.core")
local HOST = "127.0.0.1"
local PORT = 55355

local ROM_DIR = "./roms"
local SAVE_DIR = "./saves"

console.log("Shuffler server starting (listening)...")

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

local function file_exists(name)
    local f = io.open(name, "r")
    if f ~= nil then
        io.close(f)
        return true
    else
        return false
    end
end

local function save_state(path)
    savestate.save(path)
end
local function load_state_if_exists(path)
    if file_exists(path) then
        savestate.load(path)
    end
end

local function load_rom(path)
    if file_exists(path) then
        client.openrom(path)
    else
        console.log("ROM not found: " .. path .. ", cannot load.")
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

local function strip_extension(filename)
    return (filename:gsub("%.[^%.]+$", ""))
end

-- Scheduler (same as reference)
local pending = {}
local function schedule(at_epoch, fn, command)
    table.insert(pending, {
        at = at_epoch,
        fn = fn,
        command = command
    })
end
local function execute_due()
    local t = now()
    local keep = {}
    for _, job in ipairs(pending) do
        if job.at <= t then
            local ok, err = pcall(job.fn)
            if not ok then
                console.log("[ERROR] Scheduled task failed: " .. tostring(err))
            end
        else
            table.insert(keep, job)
        end
    end
    pending = keep
end
local function schedule_or_now(at_epoch, fn, command)
    if at_epoch and at_epoch > (now() + 0.0005) then
        schedule(at_epoch, fn, command)
    else
        local ok, err = pcall(fn)
        if not ok then
            console.log("[ERROR] Immediate command failed: " .. tostring(err))
        end
    end
end

-- Command implementations
local current_game = nil
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
    -- fallback: if we have stored current_game as filename, use that
    if current_game then
        return canonical_game_id_from_filename(current_game)
    end
    return nil
end
local function do_swap(target_game)
    -- save current
    local cur_id = get_current_canonical_game()
    local target_id = canonical_game_id_from_filename(target_game) or canonical_game_id_from_display(target_game)

    -- save current
    local cur = gameinfo.getromname()
    cur = sanitize_filename(cur)
    if cur and cur ~= "" and cur:lower() ~= "null" then
        local path = SAVE_DIR .. "/" .. cur .. ".state"
        pcall(function()
            save_state(path)
        end)
    end

    if target_id and cur_id and target_id == cur_id then
        -- same canonical game; skip reload
        console.log("Swap skipped: target is same as current (" .. tostring(target_id) .. ")")
        return
    end

    local rom_path = ROM_DIR .. "/" .. target_game
    load_rom(rom_path)
    local disp = sanitize_filename(gameinfo.getromname())
    if not disp or disp == "" or disp:lower() == "null" then
        disp = sanitize_filename(strip_extension(target_game))
    end
    local target_save_path = SAVE_DIR .. "/" .. disp .. ".state"
    load_state_if_exists(target_save_path)
    -- update tracked current_game to the filename used for swap
    current_game = target_game
end

local function do_start(game)
    client.unpause()
    -- Avoid reloading if the canonical id matches current
    local cur_id = get_current_canonical_game()
    local target_id = canonical_game_id_from_filename(game) or canonical_game_id_from_display(game)
    if target_id and cur_id and target_id == cur_id then
        console.log("Start skipped: target is same as current (" .. tostring(target_id) .. ")")
        return
    end
    current_game = game
    local rom_path = ROM_DIR .. "/" .. game
    load_rom(rom_path)
    local disp = sanitize_filename(gameinfo.getromname())
    if not disp or disp == "" or disp:lower() == "null" then
        disp = sanitize_filename(strip_extension(game))
    end
    local save_path = SAVE_DIR .. "/" .. disp .. ".state"
    load_state_if_exists(save_path)
end

local function do_save(path)
    save_state(path)
end
local function do_load(path)
    if not path then
        error("no path")
    end
    if file_exists(path) then
        savestate.load(path)
    else
        error("file not found: " .. tostring(path))
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
    if client_socket then
        local ok, err = pcall(function()
            client_socket:send(line .. "\n")
        end)
        if not ok then
            console.log("send error: " .. tostring(err))
        end
    end
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

local function join_from(parts, start_index)
    if not parts[start_index] then
        return ""
    end
    local s = parts[start_index]
    for i = start_index + 1, #parts do
        s = s .. "|" .. parts[i]
    end
    return s
end

local last_ping = 0
local function handle_line(line)
    local parts = split_pipe(line)
    if #parts == 0 then
        return
    end
    console.log(parts)
    if parts[1] == "CMD" then
        local id, cmd = parts[2], parts[3]
        if cmd == "SWAP" then
            local at, game = tonumber(parts[4]), parts[5]
            safe_exec_and_ack(id, function()
                schedule_or_now(at, function()
                    do_swap(game)
                end, game)
            end)
        elseif cmd == "START" then
            local at, game = tonumber(parts[4]), parts[5]
            safe_exec_and_ack(id, function()
                schedule_or_now(at, function()
                    do_start(game)
                end, game)
            end)
        elseif cmd == "SAVE" then
            local path = parts[4]
            safe_exec_and_ack(id, function()
                do_save(path)
            end)
        elseif cmd == "LOAD" then
            local path = parts[4]
            safe_exec_and_ack(id, function()
                do_load(path)
            end)
        elseif cmd == "PAUSE" then
            local at = tonumber(parts[4])
            safe_exec_and_ack(id, function()
                schedule_or_now(at, do_pause, 'pause')
            end)
        elseif cmd == "RESUME" then
            local at = tonumber(parts[4])
            safe_exec_and_ack(id, function()
                schedule_or_now(at, do_resume, 'resume')
            end)
        elseif cmd == "MSG" then
            local text = join_from(parts, 4)
            safe_exec_and_ack(id, function()
                show_message(text, 3)
            end)
        elseif cmd == "SYNC" then
            local game, state, state_at = parts[4], parts[5], tonumber(parts[6] or "0")
            safe_exec_and_ack(id, function()
                if state == "running" then
                    if game and game ~= "" then
                        schedule_or_now(state_at, function()
                            do_start(game)
                        end, game)
                    end
                else
                    if game and game ~= "" then
                        schedule_or_now(state_at, function()
                            do_start(game)
                            do_pause()
                        end, game .. "_paused")
                    else
                        schedule_or_now(state_at, do_pause, 'pause')
                    end
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

-- Main loop: accept connection, then read lines non-blocking and process scheduled tasks
local next_pending_log = now() + 10.0
local next_auto_save = now() + 10.0
while true do
    if not client_socket then
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

    -- scheduled task execution and housekeeping
    execute_due()

    local t = now()
    if t >= next_auto_save then
        -- autosave current if any
        local cur = gameinfo.getromname()
        cur = sanitize_filename(cur)
        if cur and cur ~= "" and cur:lower() ~= "null" then
            local path = SAVE_DIR .. "/" .. cur .. ".state"
            pcall(function()
                save_state(path)
            end)
        end
        next_auto_save = t + 10.0
    end
    if t >= next_pending_log then
        if #pending == 0 then
            console.log("[PENDING] No scheduled games.")
        else
            console.log("[PENDING] Scheduled games:")
            for i, job in ipairs(pending) do
                local secs = math.max(0, job.at - t)
                console.log(string.format("  %s in %.1fs", job.command, secs))
            end
        end
        next_pending_log = t + 10.0
    end

    draw_messages()
    if client.ispaused() then
        emu.yield()
    else
        emu.frameadvance()
    end
end
