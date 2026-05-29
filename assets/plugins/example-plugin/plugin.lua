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

-- Settings changed hook
local function on_settings_changed(settings)
    console.log("Settings changed!")
    -- Update internal state based on new_settings
end

-- Export plugin hooks for the main server.lua to call
return {
    on_init = on_init,
    on_frame = on_frame,
    on_settings_changed = on_settings_changed,
}