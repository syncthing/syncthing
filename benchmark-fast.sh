#!/bin/bash
# Fast A/B/C Benchmark - Builds and benchmarks all three delete detection implementations
# Usage: ./benchmark-fast.sh [a|b|c|all]
#
# Implementations:
#   A) Original - uses osutil.IsDeleted (syscalls per file)
#   B) Cached - uses DirExistenceCache + SymlinkCache  
#   C) Zero-Syscall - uses map lookup from Phase 1 walk

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RESULTS="$SCRIPT_DIR/benchmark-results/$(date +%Y%m%d_%H%M%S).txt"
APP_BIN="/Applications/Syncthing-Optimized.app/Contents/Resources/syncthing/syncthing"
API="http://127.0.0.1:8384"
API_KEY=$(grep '<apikey>' ~/Library/Application\ Support/Syncthing/config.xml | sed 's/.*<apikey>\(.*\)<\/apikey>.*/\1/')
FOLDER_ID=$(grep -o 'id="[^"]*"' ~/Library/Application\ Support/Syncthing/config.xml | head -1 | cut -d'"' -f2)
RUNS="${RUNS:-2}"
LOG_FILE=~/Library/Application\ Support/Syncthing/syncthing.log

mkdir -p "$SCRIPT_DIR/benchmark-results"

# Temp directory for builds
BUILD_DIR="/tmp/syncthing-benchmark-builds"
mkdir -p "$BUILD_DIR"

show_profiling() {
    echo "  --- Profiling ---"
    grep -E "(Phase 1|Phase 2|internal breakdown|Scan completed)" "$LOG_FILE" | tail -8 | sed 's/^/    /'
    echo ""
}

build_implementation() {
    local name="$1"
    local restore_files="$2"
    
    echo -n "  Building $name... "
    
    if [ "$restore_files" = "original" ]; then
        # Checkout original files (no zero-syscall, no cache)
        git checkout 50fe0053e -- lib/model/folder.go lib/scanner/walk.go 2>/dev/null || true
        rm -f lib/osutil/existcache.go lib/osutil/isdeleted_cached.go lib/osutil/symlinkcache.go 2>/dev/null || true
    elif [ "$restore_files" = "cached" ]; then
        # Get cached implementation files
        git show d145d6d13:lib/model/folder.go > lib/model/folder.go
        git show d145d6d13:lib/scanner/walk.go > lib/scanner/walk.go
        git show d145d6d13:lib/osutil/existcache.go > lib/osutil/existcache.go
        git show d145d6d13:lib/osutil/isdeleted_cached.go > lib/osutil/isdeleted_cached.go
        git show d145d6d13:lib/osutil/symlinkcache.go > lib/osutil/symlinkcache.go
    fi
    # For "zerosyscall", we use current working directory as-is
    
    go build -o "$BUILD_DIR/syncthing-$name" ./cmd/syncthing 2>/dev/null
    echo "done"
}

benchmark() {
    local name="$1"
    local binary="$BUILD_DIR/syncthing-$name"
    
    echo ""
    echo "=== $name ==="
    
    # Stop app
    osascript -e 'quit app "Syncthing-Optimized"' 2>/dev/null || true
    pkill -9 -f "syncthing" 2>/dev/null || true
    sleep 3
    
    # Swap binary
    cp "$binary" "$APP_BIN"
    codesign --force --deep --sign - /Applications/Syncthing-Optimized.app 2>/dev/null
    
    # Start app
    open -a "Syncthing-Optimized"
    
    # Wait for API ready
    echo -n "  Starting..."
    for i in {1..60}; do
        if curl -s "$API/rest/system/status" -H "X-API-Key: $API_KEY" 2>/dev/null | grep -q "myID"; then
            echo " ready"
            break
        fi
        sleep 1
    done
    
    # Wait for initial idle
    echo -n "  Waiting for folder ready..."
    for i in {1..300}; do
        state=$(curl -s "$API/rest/db/status?folder=$FOLDER_ID" -H "X-API-Key: $API_KEY" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('state',''))" 2>/dev/null || echo "")
        [ "$state" = "idle" ] && break
        sleep 1
    done
    echo " done"
    
    # Run scans
    local sum=0
    for run in $(seq 1 $RUNS); do
        echo -n "  Run $run: "
        
        start_event=$(curl -s "$API/rest/events?limit=1" -H "X-API-Key: $API_KEY" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0]['id'] if d else 0)" 2>/dev/null || echo "0")
        start=$(python3 -c "import time; print(time.time())")
        
        curl -s -X POST "$API/rest/db/scan?folder=$FOLDER_ID" -H "X-API-Key: $API_KEY" >/dev/null
        
        while true; do
            events=$(curl -s "$API/rest/events?since=$start_event&limit=50" -H "X-API-Key: $API_KEY" 2>/dev/null || echo "[]")
            scan_done=$(echo "$events" | python3 -c "
import sys, json
try:
    events = json.load(sys.stdin)
    for e in events:
        if e.get('type') == 'StateChanged':
            if e.get('data', {}).get('folder') == '$FOLDER_ID':
                if e.get('data', {}).get('to') == 'idle' and e.get('data', {}).get('from') == 'scanning':
                    print('done')
                    break
except:
    pass
" 2>/dev/null || echo "")
            
            [ "$scan_done" = "done" ] && break
            
            state=$(curl -s "$API/rest/db/status?folder=$FOLDER_ID" -H "X-API-Key: $API_KEY" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('state',''))" 2>/dev/null || echo "")
            [ "$state" = "idle" ] && break
            
            sleep 0.5
        done
        
        end=$(python3 -c "import time; print(time.time())")
        dur=$(python3 -c "print(f'{$end - $start:.1f}')")
        echo "${dur}s"
        sum=$(python3 -c "print($sum + $end - $start)")
        
        show_profiling
    done
    
    avg=$(python3 -c "print(f'{$sum / $RUNS:.1f}')")
    echo "  Average: ${avg}s"
    echo "$name: ${avg}s" >> "$RESULTS"
}

# Parse argument
MODE="${1:-all}"

echo "========================================"
echo "Syncthing A/B/C Scan Benchmark"
echo "========================================"
echo "Folder: $FOLDER_ID"
echo "Mode: $MODE"
echo ""

# Save current files
cp lib/model/folder.go /tmp/folder.go.save
cp lib/scanner/walk.go /tmp/walk.go.save

# Backup current app binary
cp "$APP_BIN" "$BUILD_DIR/syncthing-backup"

run_a() {
    build_implementation "A_Original" "original"
    benchmark "A_Original"
}

run_b() {
    build_implementation "B_Cached" "cached"
    benchmark "B_Cached"
}

run_c() {
    # Restore current (zero-syscall) implementation
    cp /tmp/folder.go.save lib/model/folder.go
    cp /tmp/walk.go.save lib/scanner/walk.go
    build_implementation "C_ZeroSyscall" "current"
    benchmark "C_ZeroSyscall"
}

case "$MODE" in
    a|A|original) run_a ;;
    b|B|cached) run_b ;;
    c|C|zerosyscall|zero) run_c ;;
    all) run_a; run_b; run_c ;;
    *)
        echo "Usage: $0 [a|b|c|all]"
        echo "  a - Original (IsDeleted syscalls)"
        echo "  b - Cached (DirExistenceCache)"
        echo "  c - Zero-Syscall (map lookup)"
        echo "  all - Run all three (default)"
        exit 1
        ;;
esac

# Restore original code files
cp /tmp/folder.go.save lib/model/folder.go
cp /tmp/walk.go.save lib/scanner/walk.go

# Restore original binary
echo ""
echo "Restoring original binary..."
cp "$BUILD_DIR/syncthing-backup" "$APP_BIN"
codesign --force --deep --sign - /Applications/Syncthing-Optimized.app 2>/dev/null
open -a "Syncthing-Optimized"

# Cleanup
rm -rf "$BUILD_DIR"

echo ""
echo "========================================"
echo "RESULTS"
echo "========================================"
cat "$RESULTS" 2>/dev/null || echo "No results"
echo ""
echo "Done!"
