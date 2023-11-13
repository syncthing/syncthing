import { environment } from '../environments/environment'

export const deviceID = (): String => {
    const dID: String = environment.production ? globalThis.metadata['deviceID'] : '1234567';
    return dID.substring(0, 7)
}

export const apiURL: String = '/'
export const apiRetry: number = 3;