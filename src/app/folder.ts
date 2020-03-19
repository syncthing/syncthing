import { Device } from './device';

interface Folder {
    id: string;
    label: string;
    devices: Device[];
    status: Folder.Status;
    paused: boolean;
}

namespace Folder {
    export enum stateType {
    }

    export function statusToString(f: Folder): string {
        const fs: Folder.Status = f.status;
        const state: string = fs.state;

        if (f.paused) {
            return 'paused';
        }

        if (!f.status || (Object.keys(f.status).length === 0)) {
            return 'unknown';
        }

        if (state === 'error') {
            return 'stopped'; // legacy, the state is called "stopped" in the GUI
        }

        if (state !== 'idle') {
            return state;
        }

        const needTotalItems = fs.needDeletes + fs.needDirectories +
            fs.needFiles + fs.needSymlinks;
        const receiveOnlyTotalItems = fs.receiveOnlyChangedDeletes + fs.receiveOnlyChangedDirectories +
            fs.receiveOnlyChangedFiles + fs.receiveOnlyChangedSymlinks;

        if (needTotalItems > 0) {
            return 'outofsync';
        }
        if (f.status.pullErrors > 0) {
            return 'faileditems';
        }
        if (receiveOnlyTotalItems > 0) {
            return 'localadditions';
        }
        if (f.devices.length <= 1) {
            return 'unshared';
        }

        return state;
    }

    export interface Status {
        globalBytes: number;
        globalDeleted: number;
        globalDirectories: number;
        globalFiles: number;
        globalSymlinks: number;
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
        pullErrors: number;
        receiveOnlyChangedBytes: number;
        receiveOnlyChangedDeletes: number;
        receiveOnlyChangedDirectories: number;
        receiveOnlyChangedFiles: number;
        receiveOnlyChangedSymlinks: number;
        sequence: number;
        state: string;
        stateChanged: string;
        version: number;
    }
}

export default Folder;






