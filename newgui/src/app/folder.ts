import Device from './device';
import { colors } from './style';
import { Completion } from './completion';

interface Folder {
    id: string;
    label: string;
    devices: Device[];
    status: Folder.Status;
    stateType: Folder.StateType;
    state: string;
    paused: boolean;
    completion: Completion;
    path: string;
}

namespace Folder {
    export enum StateType {
        Paused = 1,
        Unknown,
        Unshared,
        WaitingToScan,
        Stopped,
        Scanning,
        Idle,
        LocalAdditions,
        WaitingToSync,
        PreparingToSync,
        Syncing,
        OutOfSync,
        FailedItems,
    }

    /**
     * stateTypeToString returns a string representation of
     * the StateType enum
     * @param s StateType
     */
    export function stateTypeToString(s: StateType): string {
        switch (s) {
            case StateType.Paused:
                return 'Paused';
            case StateType.Unknown:
                return 'Unknown';
            case StateType.Unshared:
                return 'Unshared';
            case StateType.WaitingToSync:
                return 'Waiting to Sync';
            case StateType.Stopped:
                return 'Stopped';
            case StateType.Scanning:
                return 'Scanning';
            case StateType.Idle:
                return 'Up to Date';
            case StateType.LocalAdditions:
                return 'Local Additions';
            case StateType.WaitingToScan:
                return 'Waiting to Scan';
            case StateType.PreparingToSync:
                return 'Preparing to Sync';
            case StateType.Syncing:
                return 'Syncing';
            case StateType.OutOfSync:
                return 'Out of Sync';
            case StateType.FailedItems:
                return 'Failed Items';
        }
    }

    /**
     * stateTypeToColor looks up a hex color string based on StateType 
     * @param s StateType 
     */
    export function stateTypeToColor(s: StateType): string {
        switch (s) {
            case StateType.Paused:
                return colors.get("grey");
            case StateType.Unknown:
                return colors.get("grey");
            case StateType.Unshared:
                return colors.get("grey");
            case StateType.WaitingToSync:
                return colors.get("yellow");
            case StateType.Stopped:
                return colors.get("grey");
            case StateType.Scanning:
                return colors.get("grey");
            case StateType.Idle:
                return colors.get("blue");
            case StateType.LocalAdditions:
                return colors.get("grey");
            case StateType.WaitingToScan:
                return colors.get("grey");
            case StateType.PreparingToSync:
                return colors.get("grey");
            case StateType.Syncing:
                return colors.get("green");
            case StateType.OutOfSync:
                return colors.get("grey");
            case StateType.FailedItems:
                return colors.get("red");
        }
    }

    /**
     * getStateType looks at a folder and determines the correct
     * StateType to return
     * 
     * Possible state values from API
     * "idle", "scanning", "scan-waiting", "sync-waiting", "sync-preparing"
     * "syncing", "error", "unknown"
     * 
     * @param f Folder
     */
    export function getStateType(f: Folder): StateType {
        if (f.paused) {
            return StateType.Paused;
        }

        if (!f.status || (Object.keys(f.status).length === 0)) {
            return StateType.Unknown;
        }

        const fs: Folder.Status = f.status;
        const state: string = fs.state;

        // Match API string to StateType
        switch (state) {
            case "idle":
                return StateType.Idle;
            case "scanning":
                return StateType.Scanning;
            case "scan-waiting":
                return StateType.WaitingToScan;
            case "sync-waiting":
                return StateType.WaitingToSync;
            case "sync-preparing":
                return StateType.PreparingToSync;
            case "syncing":
                return StateType.Syncing;
            case "error":
                // legacy, the state is called "stopped" in the gui
                return StateType.Stopped;
            case "unknown":
                return StateType.Unknown;
        }

        if (fs.needTotalItems > 0) {
            return StateType.OutOfSync;
        }
        if (fs.pullErrors > 0) {
            return StateType.FailedItems;
        }
        if (fs.receiveOnlyTotalItems > 0) {
            return StateType.LocalAdditions;
        }
        if (f.devices.length <= 1) {
            return StateType.Unshared;
        }

        return StateType.Unknown;
    }


    export interface Status {
        globalBytes: number;
        globalDeleted: number;
        globalDirectories: number;
        globalFiles: number;
        globalSymlinks: number;
        globalTotalItems: number;
        ignorePatterns: boolean;
        inSyncBytes: number;
        inSyncFiles: number;
        invalid: string;
        localBytes: number;
        localDeleted: number;
        localDirectories: number;
        localFiles: number;
        localSymlinks: number;
        needBytes: number;
        needDeletes: number;
        needDirectories: number;
        needFiles: number;
        needSymlinks: number;
        needTotalItems: number;
        pullErrors: number;
        receiveOnlyChangedBytes: number;
        receiveOnlyChangedDeletes: number;
        receiveOnlyChangedDirectories: number;
        receiveOnlyChangedFiles: number;
        receiveOnlyChangedSymlinks: number;
        receiveOnlyTotalItems: number;
        sequence: number;
        state: string;
        stateChanged: string;
        version: number;
    }
}
export default Folder;