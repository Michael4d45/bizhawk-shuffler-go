# BizHawk Shuffler - Lua Plugin Management System TODO

## Overview
This document outlines the comprehensive plan for implementing a Lua plugin management system for BizHawk Shuffler. The goal is to allow dynamic loading, distribution, and management of Lua plugins that extend BizHawk functionality.

## Core Architecture Requirements

### 1. Plugin Directory Structure
- [x] Create `plugins/` directory in repository root
- [x] Define plugin subdirectory structure:
  ```
  plugins/
  ├── plugin-name/
  │   ├── plugin.lua   # Main plugin code
  │   ├── meta.json    # Plugin metadata
  │   └── README.md    # Plugin documentation
  ```

### 2. Plugin Metadata Schema
- [ ] Design `meta.json` schema for plugins:
  ```json
  {
    "name": "plugin-name",
    "version": "1.0.0", 
    "description": "Plugin description",
    "author": "Author Name",
    "bizhawk_version": ">=2.8.0",
    "dependencies": ["other-plugin"],
    "enabled": true,
    "entry_point": "plugin.lua",
    "hooks": ["on_frame", "on_save", "on_load"],
    "config_schema": { ... }
  }
  ```

### 3. Plugin Loading System
- [ ] Modify `server.lua` to support dynamic plugin loading
- [ ] Implement plugin lifecycle hooks:
  - [ ] `on_init()` - Called when plugin is loaded
  - [ ] `on_cleanup()` - Called when plugin is unloaded  
  - [ ] `on_frame()` - Called every frame
  - [ ] `on_save()` - Called before save operations
  - [ ] `on_load()` - Called after load operations
  - [ ] `on_game_start()` - Called when game starts
  - [ ] `on_game_end()` - Called when game ends

### 4. Plugin Management API Endpoints
- [ ] `GET /api/plugins` - List all plugins with status
- [ ] `POST /api/plugins/{name}/enable` - Enable a plugin
- [ ] `POST /api/plugins/{name}/disable` - Disable a plugin
- [ ] `POST /api/plugins/upload` - Upload new plugin (zip file)
- [ ] `DELETE /api/plugins/{name}` - Remove a plugin
- [ ] `GET /api/plugins/{name}/config` - Get plugin configuration
- [ ] `POST /api/plugins/{name}/config` - Update plugin configuration
- [ ] `POST /api/plugins/reload` - Reload all plugins

### 5. Web UI Plugin Management
- [ ] Add "Plugins" tab to admin interface
- [ ] Plugin list with enable/disable toggles
- [ ] Plugin upload interface
- [ ] Plugin configuration forms (dynamic based on schema)
- [ ] Plugin status indicators (loaded, error, disabled)
- [ ] Plugin dependency visualization

### 6. Client-Side Plugin Distribution
- [ ] Extend client download system for plugins
- [ ] Plugin sync mechanism (download enabled plugins)
- [ ] Plugin update detection and download
- [ ] Client-side plugin loading in BizHawk

## Implementation Details

### 7. Plugin Security & Sandboxing
- [ ] Define safe Lua environment for plugins
- [ ] Whitelist allowed Lua functions/modules
- [ ] Plugin resource limits (memory, execution time)
- [ ] Plugin isolation to prevent conflicts

### 8. Plugin Configuration System
- [ ] Dynamic configuration UI generation from schema
- [ ] Configuration validation
- [ ] Per-plugin configuration persistence
- [ ] Configuration hot-reload capability

### 9. Plugin Development Tools
- [ ] Plugin template/scaffold generator
- [ ] Plugin validation tool
- [ ] Plugin testing framework
- [ ] Documentation generator

### 10. Advanced Features
- [ ] Plugin dependency resolution
- [ ] Plugin version management
- [ ] Plugin marketplace/repository integration
- [ ] Plugin hot-reloading without restart
- [ ] Plugin event system for inter-plugin communication

## Technical Considerations

### 11. Lua Integration
- [ ] Research BizHawk Lua API limitations
- [ ] Design plugin isolation mechanisms
- [ ] Handle Lua state management for multiple plugins
- [ ] Error handling and plugin failure recovery

### 12. File Distribution
- [ ] Extend existing file serving for plugins
- [ ] Plugin packaging format (zip with validation)
- [ ] Incremental plugin updates
- [ ] Plugin integrity verification

### 13. State Management
- [ ] Plugin state persistence
- [ ] Plugin configuration in server state
- [ ] Plugin status tracking
- [ ] Rollback mechanism for plugin failures

## Phase 1: Minimal Viable Implementation (Current Sprint)
- [x] Create this comprehensive TODO document
- [ ] Basic plugin directory structure
- [ ] Simple plugin metadata types
- [ ] Basic enable/disable API endpoints
- [ ] Minimal plugin management UI
- [ ] Simple plugin loading in server.lua

## Phase 2: Core Plugin System
- [ ] Complete plugin loading infrastructure
- [ ] Plugin lifecycle management
- [ ] Configuration system
- [ ] Client-side plugin distribution

## Phase 3: Advanced Features
- [ ] Plugin marketplace integration
- [ ] Advanced security features
- [ ] Plugin development tools
- [ ] Performance optimization

## Questions for Clarification

1. **Plugin Distribution**: Should plugins be distributed to all clients automatically, or only on-demand?

2. **Plugin Security**: What level of sandboxing is needed? Should plugins have full BizHawk API access?

3. **Plugin Persistence**: Should plugin state persist across server restarts? Per-user or global?

4. **Plugin Dependencies**: How should plugin dependencies be resolved and managed?

5. **Plugin Updates**: Should plugins auto-update, or require manual approval?

6. **Plugin Conflicts**: How should conflicts between plugins be handled?

7. **Client Compatibility**: Should the client support different plugin versions than the server?

## Risk Assessment

### High Risk
- Plugin security vulnerabilities
- BizHawk compatibility issues
- Performance impact on emulation

### Medium Risk  
- Plugin dependency conflicts
- State corruption from bad plugins
- User experience complexity

### Low Risk
- UI complexity
- File management overhead
- Documentation maintenance

## Success Metrics

- [ ] Plugins can be uploaded, enabled, and disabled through web UI
- [ ] Plugins execute correctly in BizHawk without crashes
- [ ] Plugin system doesn't impact emulation performance
- [ ] Plugin configuration is persistent and reliable
- [ ] Client can automatically sync plugins from server

## Related Files to Modify

- `internal/types/types.go` - Add plugin types
- `internal/server/api_plugins.go` - New plugin management API
- `internal/server/server.go` - Plugin routes registration  
- `web/index.html` - Plugin management UI
- `server.lua` - Plugin loading system
- `internal/client/controller.go` - Client plugin sync
- `Makefile` - Plugin directory setup

---

*This document should be updated as the plugin system evolves and requirements become clearer.*