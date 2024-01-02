import { environment } from '../environments/environment'

export const deviceID = (): String => {
    // keep consistent with ShortIDStringLength in lib/protocol/deviceid.go
    return environment.production ? globalThis.metadata['deviceIDShort'] : '1234567';
}

export const apiURL: String = '/'
export const apiRetry: number = 3;