import { colors } from './style';
import Folder from './folder';
import { Completion } from './completion';

interface Device {
    deviceID: string;
    name: string;
    stateType: Device.StateType;
    state: string;
    paused: boolean;
    connected: boolean;
    completion: Completion;
    used: boolean; // indicates if a folder is using the device
    folders: Folder[];
}

namespace Device {
    export enum StateType {
        Insync = 1,
        UnusedInsync,
        Unknown,
        Syncing,
        Paused,
        UnusedPaused,
        Disconnected,
        UnusedDisconnected,
    }

    export function stateTypeToString(s: StateType): string {
        switch (s) {
            case StateType.Insync:
                return 'Up to Date';
            case StateType.UnusedInsync:
                return 'Connected (Unused)';
            case StateType.Unknown:
                return 'Unknown';
            case StateType.Syncing:
                return 'Syncing';
            case StateType.Paused:
                return 'Paused';
            case StateType.UnusedPaused:
                return 'Paused (Unused)';
            case StateType.Disconnected:
                return 'Disconnected';
            case StateType.UnusedDisconnected:
                return 'Disconnected (Unused)';
        }
    }

    /**
     * stateTypeToColor looks up a hex color string based on StateType 
     * @param s StateType 
     */
    export function stateTypeToColor(s: StateType): string {
        switch (s) {
            case StateType.Insync:
                return colors.get("blue");
            case StateType.UnusedInsync:
                return colors.get("grey");
            case StateType.Unknown:
                return colors.get("grey");
            case StateType.Syncing:
                return colors.get("green");
            case StateType.Paused:
                return colors.get("grey");
            case StateType.UnusedPaused:
                return colors.get("grey");
            case StateType.Disconnected:
                return colors.get("yellow");
            case StateType.UnusedDisconnected:
                return colors.get("grey");
        }
    }

    export function getStateType(d: Device): StateType {
        // StateType Unknown is set in DeviceService
        if (d.stateType === StateType.Unknown) {
            return StateType.Unknown;
        }

        if (d.paused) {
            return d.used ? StateType.Paused : StateType.UnusedPaused;
        }

        if (d.connected) {
            if (d.completion.completion === 100) {
                return d.used ? StateType.Insync : StateType.UnusedInsync;
            } else {
                return StateType.Syncing;
            }
        }

        return d.used ? StateType.Disconnected : StateType.UnusedDisconnected;
    }

    export function recalcCompletion(d: Device) {
        if (!d || !d.completion || !d.folders) {
            return
        }
        var total = 0, needed = 0, deletes = 0, items = 0;
        d.folders.forEach(folder => {
            if (!folder || !folder.completion)
                return
            needed += folder.completion.needBytes;
            items += folder.completion.needItems;
            deletes += folder.completion.needDeletes;
        });
        if (total == 0) {
            d.completion.completion = 100;
            d.completion.needBytes = 0;
            d.completion.needItems = 0;
        } else {
            d.completion.completion = Math.floor(100 * (1 - needed / total));
            d.completion.needBytes = needed;
            d.completion.needItems = items + deletes;
        }

        if (needed == 0 && deletes > 0) {
            // We don't need any data, but we have deletes that we need
            // to do. Drop down the completion percentage to indicate
            // that we have stuff to do.
            d.completion.completion = 95;
        }
    }
}
export default Device;