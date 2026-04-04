# TUI Testing Guide

This document describes how to set up a local testing environment for the
Syncthing TUI and run comprehensive tests using tmux.

## TUI Layout

The TUI uses a single-page layout matching the web GUI, with three sections:

1. **Folders** — Collapsible panels for each folder (expand with Enter)
2. **This Device** — Always-expanded local device info
3. **Remote Devices** — Collapsible panels for each remote device

Navigation: `Tab`/`Shift-Tab` between sections, `j`/`k` within sections,
`Enter` to expand/collapse, `?` for help, `e` for event log overlay.
`PgUp`/`PgDn` for manual scrolling. The view auto-scrolls to keep the
focused item visible.

When adding a device, discovered/nearby devices are shown as a selectable
list. Select one with `Enter` to auto-fill the Device ID and addresses.

## Prerequisites

- Two Syncthing instances (built from source)
- tmux
- Optionally: agent-browser for web GUI comparison

## Setting Up Test Instances

Create two isolated Syncthing instances with separate home directories and
non-conflicting ports:

```bash
ST_BIN="./syncthing"
TEST_DIR="/tmp/st-tui-test"

# Clean slate
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"/{inst1,inst2}/sync-folder

# Generate configs
$ST_BIN generate --home="$TEST_DIR/inst1"
$ST_BIN generate --home="$TEST_DIR/inst2"
```

Patch configs to use different ports and set API keys:

```python
import xml.etree.ElementTree as ET

for inst, port, apikey, lport in [
    ("inst1", "8384", "testkey1", "22000"),
    ("inst2", "8385", "testkey2", "22001"),
]:
    path = f"/tmp/st-tui-test/{inst}/config.xml"
    tree = ET.parse(path)
    root = tree.getroot()

    gui = root.find("gui")
    gui.find("address").text = f"127.0.0.1:{port}"
    gui.find("apikey").text = apikey

    opts = root.find("options")
    for la in opts.findall("listenAddress"):
        opts.remove(la)
    ET.SubElement(opts, "listenAddress").text = f"tcp://:{lport}"
    ET.SubElement(opts, "listenAddress").text = f"quic://:{lport}"
    tree.write(path)
```

Start both instances:

```bash
STNOUPGRADE=1 $ST_BIN serve --home="$TEST_DIR/inst1" --no-browser &
STNOUPGRADE=1 $ST_BIN serve --home="$TEST_DIR/inst2" --no-browser &
sleep 5
```

Configure devices to know each other and add a shared folder:

```bash
DEV1=$(curl -sf -H 'X-Api-Key: testkey1' http://127.0.0.1:8384/rest/system/status | python3 -c "import json,sys; print(json.load(sys.stdin)['myID'])")
DEV2=$(curl -sf -H 'X-Api-Key: testkey2' http://127.0.0.1:8385/rest/system/status | python3 -c "import json,sys; print(json.load(sys.stdin)['myID'])")

# Add devices to each other
curl -sf -X POST -H 'X-Api-Key: testkey1' -H 'Content-Type: application/json' \
  http://127.0.0.1:8384/rest/config/devices \
  -d "{\"deviceID\": \"$DEV2\", \"name\": \"TestNode2\", \"addresses\": [\"tcp://127.0.0.1:22001\"]}"

curl -sf -X POST -H 'X-Api-Key: testkey2' -H 'Content-Type: application/json' \
  http://127.0.0.1:8385/rest/config/devices \
  -d "{\"deviceID\": \"$DEV1\", \"name\": \"TestNode1\", \"addresses\": [\"tcp://127.0.0.1:22000\"]}"

# Add shared folder
mkdir -p "$TEST_DIR/inst1/sync-folder/.stfolder" "$TEST_DIR/inst2/sync-folder/.stfolder"

curl -sf -X POST -H 'X-Api-Key: testkey1' -H 'Content-Type: application/json' \
  http://127.0.0.1:8384/rest/config/folders \
  -d "{\"id\": \"test-sync\", \"label\": \"Test Sync Folder\", \"path\": \"$TEST_DIR/inst1/sync-folder\", \"type\": \"sendreceive\", \"devices\": [{\"deviceID\": \"$DEV1\"}, {\"deviceID\": \"$DEV2\"}]}"

curl -sf -X POST -H 'X-Api-Key: testkey2' -H 'Content-Type: application/json' \
  http://127.0.0.1:8385/rest/config/folders \
  -d "{\"id\": \"test-sync\", \"label\": \"Test Sync Folder\", \"path\": \"$TEST_DIR/inst2/sync-folder\", \"type\": \"sendreceive\", \"devices\": [{\"deviceID\": \"$DEV1\"}, {\"deviceID\": \"$DEV2\"}]}"
```

## Running the TUI in tmux

```bash
mkdir -p /tmp/tmux-1000  # may be needed
tmux new-session -d -s tuitest -x 120 -y 45
tmux send-keys -t tuitest \
  "STHOMEDIR=/tmp/st-tui-test/inst1 ./syncthing tui" Enter
```

## Capturing and Sending Keys

```bash
# Capture current screen
tmux capture-pane -t tuitest -p

# Navigation
tmux send-keys -t tuitest Tab       # Next section
tmux send-keys -t tuitest Enter     # Expand/collapse
tmux send-keys -t tuitest "j"       # Move down in list
tmux send-keys -t tuitest "k"       # Move up in list
tmux send-keys -t tuitest Escape    # Close form/back
tmux send-keys -t tuitest C-c       # Quit

# Actions
tmux send-keys -t tuitest "a"       # Add folder/device
tmux send-keys -t tuitest "s"       # Scan
tmux send-keys -t tuitest "p"       # Pause/resume
tmux send-keys -t tuitest "x"       # Remove
tmux send-keys -t tuitest "S"       # Share folder
tmux send-keys -t tuitest "e"       # Toggle event log
tmux send-keys -t tuitest "?"       # Toggle help
tmux send-keys -t tuitest "R"       # Restart daemon
```

## Automated Test Pattern

```bash
PASS=0; FAIL=0
check() {
  if tmux capture-pane -t tuitest -p | grep -q "$2"; then
    echo "PASS: $1"; PASS=$((PASS+1))
  else
    echo "FAIL: $1"; FAIL=$((FAIL+1))
  fi
}

# Verify main layout
check "Folders section" "Folders"
check "This Device" "This Device"
check "Remote Devices" "Remote Devices"

# Expand folder
tmux send-keys -t tuitest Enter; sleep 0.5
check "Folder ID" "Folder ID.*test-sync"
check "Global State" "Global State.*3 files"
check "Folder Type" "Send & Receive"
check "Rescans" "Rescans"
check "Shared With" "Shared With.*TestNode2"

# Navigate to Remote Devices and expand
tmux send-keys -t tuitest Tab Tab; sleep 0.3
tmux send-keys -t tuitest Enter; sleep 0.5
check "Connection Type" "TCP LAN"
check "Compression" "Metadata Only"

echo "Results: $PASS passed, $FAIL failed"
```

## Web GUI Comparison with agent-browser

If agent-browser and Chromium are available:

```bash
AB=".cargo-home/bin/agent-browser"
$AB --ignore-https-errors open "http://127.0.0.1:8384"
sleep 2
$AB snapshot -c  # Compact snapshot

# Expand a folder in the web GUI
$AB find text "Test Sync Folder" click
sleep 1
$AB snapshot -c

# Compare specific fields
$AB eval 'document.querySelector(".panel-body table")?.innerText'
```

## Testing Sync

```bash
# Create file on instance 1
echo "test" > /tmp/st-tui-test/inst1/sync-folder/test.txt
curl -sf -X POST -H 'X-Api-Key: testkey1' \
  "http://127.0.0.1:8384/rest/db/scan?folder=test-sync"
sleep 5

# Verify synced to instance 2
cat /tmp/st-tui-test/inst2/sync-folder/test.txt

# Verify TUI shows updated file count
tmux capture-pane -t tuitest -p | grep "Global State"
```

## Feature Parity Checklist

Fields shown in expanded folder (matching web GUI):
- [x] Folder ID
- [x] Folder Path
- [x] Global State (files, dirs, bytes)
- [x] Local State (files, dirs, bytes)
- [x] Out of Sync (when applicable)
- [x] Folder Type
- [x] Rescans (interval + watcher status)
- [x] File Pull Order
- [x] Shared With
- [x] Last Scan
- [x] Last File (when available)
- [x] Errors

Fields shown in This Device (matching web GUI):
- [x] Download Rate (with total)
- [x] Upload Rate (with total)
- [x] Local State (Total)
- [x] Listeners (N/M)
- [x] Discovery (N/M)
- [x] Uptime
- [x] Identification
- [x] Version

Fields shown in expanded remote device (matching web GUI):
- [x] Download Rate (with total)
- [x] Upload Rate (with total)
- [x] Address
- [x] Connection Type (TCP LAN, etc.)
- [x] Number of Connections
- [x] Compression
- [x] Identification
- [x] Version
- [x] Last Seen
- [x] Shared Folders

## Cleanup

```bash
killall syncthing
tmux kill-session -t tuitest
rm -rf /tmp/st-tui-test
```
