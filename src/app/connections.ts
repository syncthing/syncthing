export interface SystemConnections {
    connections: { deviceId?: Connection };
    total: Connection;
}

export interface Connection {
    address: string;
    at: string;
    clientVersion: string;
    connected: boolean;
    crypto: string;
    inBytesTotal: number;
    outBytesTotal: number;
    paused: boolean;
    type: string;
}