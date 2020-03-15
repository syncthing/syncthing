export const deviceID = (): String => {
    const dID: String = globalThis.metadata['deviceID'];
    return dID.substring(0, 5)
}

export const apiURL: String = 'http://127.0.0.1:8384/'