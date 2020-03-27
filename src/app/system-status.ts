export interface SystemStatus {
    alloc: number;
    connectionServiceStatus: any;
    cpuPercent: number; // allows returns 0
    discoveryEnabled: boolean;
    discoveryErrors: any;
    discoveryMethods: number;
    goroutines: number;
    lastDialStatus: any;
    myID: string;
    pathSeparator: string;
    startTime: string;
    sys: number;
    themes: string[];
    tilde: string;
    uptime: number;
}