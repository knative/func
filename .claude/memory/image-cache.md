# Image Cache Implementation Progress

## Goal
Add `--imageCache` option to `allocate.sh` script to enable persistent registry caching across cluster deletions/recreations.

## Current State

### What's Been Implemented 

1. **[hack/common.sh](../hack/common.sh)**
   - Added `parse_args()` function to handle `--imageCache` flag
   - Modified `populate_environment()` to export `IMAGE_CACHE_DIR` when flag is used
   - Default cache location: `${HOME}/.local/share/knative-func/registry-cache`
   - Prints `IMAGE_CACHE_DIR` in initialization output when flag is used

2. **[hack/allocate.sh](../hack/allocate.sh)**
   - Moved `registry()` call to run sequentially (line 32) before parallel tasks
   - Modified `registry()` function to:
     - Create cache directory if `IMAGE_CACHE_DIR` is set
     - Mount volume with `:z` flag for SELinux compatibility
     - Added readiness check (waits for registry to be ready before continuing)
   - Currently creates ONE registry container (`func-registry`) at localhost:50000

3. **[hack/delete.sh](../hack/delete.sh)**
   - Inspects `func-registry` container to detect cache directory
   - Shows note about cache directory not being removed (if it exists)
   - Works with both Docker and Podman via `$CONTAINER_ENGINE`

### Current Problem L

**Registry pull-through cache limitation discovered:**
- The Docker registry distribution can only mirror **ONE upstream registry** at a time
- Current code tries to configure mirrors for multiple upstreams (ghcr.io, quay.io) in Kind config
- This doesn't work as a proper pull-through cache

**Evidence:**
- Registry distribution docs: https://distribution.github.io/distribution/recipes/mirror/
- "It's currently possible to mirror only one upstream registry at a time"

## Solution (Not Yet Implemented)

Based on user's existing working setup from another project (files in `.vscode/tmp/`):

### Architecture Required:
**Multiple registry containers** - one per upstream registry:
- `func-registry-ghcr` � caches ghcr.io (port 50001)
- `func-registry-quay` � caches quay.io (port 50002)
- `func-registry-gcr` � caches gcr.io (port 50003)
- Optional: `func-registry-dockerio` � caches docker.io (port 50004)

Each registry needs:
1. **Config file** specifying the upstream URL (see `.vscode/tmp/reg-*.yml`)
2. **Unique port** for each registry
3. **Separate cache directory**: `$IMAGE_CACHE_DIR/{ghcr,quay,gcr}`
4. **Connection to kind network**

### Reference Files (from user's working setup):
- `.vscode/tmp/kn8.sh` - working implementation
- `.vscode/tmp/one-node-cluster.yaml` - Kind config with multiple registry mirrors
- `.vscode/tmp/reg-ghcr.yml` - GHCR cache registry config
- `.vscode/tmp/reg-quay.yml` - Quay cache registry config
- `.vscode/tmp/reg-gcr.yml` - GCR cache registry config
- `.vscode/tmp/reg-dockercr.yml` - Docker.io cache registry config

### Implementation Plan (Next Steps):

#### 1. Create registry config templates
Add to `hack/` directory:
```yaml
# hack/registry-ghcr.yml
version: 0.1
proxy:
  remoteurl: https://ghcr.io
storage:
  filesystem:
    rootdirectory: /var/lib/registry
http:
  addr: :5000
```
(Similar for quay, gcr, etc.)

#### 2. Update `registry()` function in allocate.sh
When `IMAGE_CACHE_DIR` is set:
```bash
# Create subdirectories
mkdir -p "$IMAGE_CACHE_DIR"/{ghcr,quay,gcr}

# Start multiple registry containers
# func-registry-ghcr on port 50001
# func-registry-quay on port 50002
# func-registry-gcr on port 50003
# Each with proper config file mounted
# Each connected to kind network
```

#### 3. Update `kubernetes()` function in allocate.sh
Modify `containerdConfigPatches` (lines 93-102):
- When `IMAGE_CACHE_DIR` is set: configure mirrors for ghcr.io � registry-ghcr:5000, etc.
- When not set: keep current behavior (only localhost:50000)

#### 4. Update `delete.sh`
- Stop and remove all cache registry containers
- Detect and report all cache directories

## Usage

### Current (works for basic case):
```bash
./hack/allocate.sh --imageCache
```

### After full implementation:
```bash
# Use default cache location
./hack/allocate.sh --imageCache

# Use custom cache location
IMAGE_CACHE_DIR=/custom/path ./hack/allocate.sh --imageCache

# Delete cluster (preserves cache)
./hack/delete.sh
```

## Key Design Decisions

1. **Flag only, not required**: `--imageCache` is optional; without it, registry runs without persistence
2. **SELinux compatible**: Uses `:z` flag on volume mounts (safe on non-SELinux systems)
3. **Cross-platform**: Works with both Docker and Podman
4. **User-space cache**: Default location in `~/.local/share/` (XDG standard)
5. **Sequential registry startup**: Registry must be ready before other components start pulling images

## Notes

- User already has registry cache containers running from other projects (check `docker ps -a`)
- The registry setup must happen BEFORE parallel component installation to ensure caching works
- Readiness check is critical - registry must be serving requests before Kind starts pulling images
