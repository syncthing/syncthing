export interface Folder {
    id: string;
    label: string;
    status: FolderStatus;
}

export interface FolderStatus {
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