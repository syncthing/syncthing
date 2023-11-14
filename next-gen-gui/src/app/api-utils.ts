import { environment } from '../environments/environment'

export const deviceID = (): String => {
    return environment.production ? globalThis.metadata['deviceIDShort'] : '1234567';
}

export const apiURL: String = '/'
export const apiRetry: number = 3;