-- Example BizHawk Shuffler Plugin
-- This plugin demonstrates basic plugin functionality

console.log("Example plugin loaded!")

-- Plugin initialization hook
local function on_init()
    console.log("Example plugin initialized")
end

-- Frame update hook 
local function on_frame()
    -- This would run every frame - be careful with performance!
    -- For demo purposes, do nothing
end

-- Game start hook
local function on_game_start(game_name)
    console.log("Example plugin: Game started - " .. tostring(game_name))
end

-- Export plugin hooks for the main server.lua to call
return {
    on_init = on_init,
    on_frame = on_frame,
    on_game_start = on_game_start
}