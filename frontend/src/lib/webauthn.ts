import { api } from './api'

function b64urlToBuf(s: string): ArrayBuffer {
    const pad = '='.repeat((4 - (s.length % 4)) % 4)
    const b64 = (s + pad).replace(/-/g, '+').replace(/_/g, '/')
    const bin = atob(b64)
    const buf = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i)
    return buf.buffer
}

function bufToB64url(buf: ArrayBuffer): string {
    const bytes = new Uint8Array(buf)
    let bin = ''
    for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
    return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function passkeySupported(): boolean {
    return (
        typeof window !== 'undefined' &&
        typeof window.PublicKeyCredential !== 'undefined'
    )
}

// registerPasskey проводит церемонию регистрации passkey (требует авторизации).
export async function registerPasskey(): Promise<void> {
    /* eslint-disable @typescript-eslint/no-explicit-any */
    const opts: any = await api.post<any>(
        '/api/account/passkey/register/begin',
    )
    const pk = opts.publicKey
    pk.challenge = b64urlToBuf(pk.challenge)
    pk.user.id = b64urlToBuf(pk.user.id)
    if (pk.excludeCredentials) {
        pk.excludeCredentials.forEach((c: any) => (c.id = b64urlToBuf(c.id)))
    }

    const cred = (await navigator.credentials.create({
        publicKey: pk,
    })) as PublicKeyCredential | null
    if (!cred) throw new Error('passkey не создан')

    const resp = cred.response as AuthenticatorAttestationResponse
    await api.post('/api/account/passkey/register/finish', {
        id: cred.id,
        rawId: bufToB64url(cred.rawId),
        type: cred.type,
        response: {
            attestationObject: bufToB64url(resp.attestationObject),
            clientDataJSON: bufToB64url(resp.clientDataJSON),
        },
        clientExtensionResults: cred.getClientExtensionResults(),
    })
}

// getPasskeyToken проводит церемонию входа и возвращает JWT.
export async function getPasskeyToken(): Promise<string> {
    const opts: any = await api.post<any>('/api/auth/login/passkey/begin')
    const pk = opts.publicKey
    pk.challenge = b64urlToBuf(pk.challenge)
    if (pk.allowCredentials) {
        pk.allowCredentials.forEach((c: any) => (c.id = b64urlToBuf(c.id)))
    }

    const cred = (await navigator.credentials.get({
        publicKey: pk,
    })) as PublicKeyCredential | null
    if (!cred) throw new Error('passkey не получен')

    const resp = cred.response as AuthenticatorAssertionResponse
    const data = await api.post<{ token: string }>(
        '/api/auth/login/passkey/finish',
        {
            id: cred.id,
            rawId: bufToB64url(cred.rawId),
            type: cred.type,
            response: {
                authenticatorData: bufToB64url(resp.authenticatorData),
                clientDataJSON: bufToB64url(resp.clientDataJSON),
                signature: bufToB64url(resp.signature),
                userHandle: resp.userHandle
                    ? bufToB64url(resp.userHandle)
                    : '',
            },
            clientExtensionResults: cred.getClientExtensionResults(),
        },
    )
    /* eslint-enable @typescript-eslint/no-explicit-any */
    return data.token
}
