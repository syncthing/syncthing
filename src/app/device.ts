interface Device {
    deviceID: string;
    name: string;
    state: Device.StateType;
    paused: boolean;
    connected: boolean;
    completion: number;
    used: boolean; // indicates if a folder is using the device
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

    export function getStateType(d: Device): StateType {
        // TODO
        /*
        if (typeof $scope.connections[deviceCfg.deviceID] === 'undefined') {
            return 'unknown';
        }
        */

        if (d.paused) {
            return d.used ? StateType.Paused : StateType.UnusedPaused;
        }

        if (d.connected) {
            if (d.completion === 100) {
                return d.used ? StateType.Insync : StateType.UnusedInsync;
            } else {
                return StateType.Syncing;
            }
        }

        return d.used ? StateType.Disconnected : StateType.UnusedDisconnected;
    }
}
export default Device;