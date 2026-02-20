export type Server = {
    id: number
    name: string
    address: string
    port: number
    protocol: 'vless' | 'vmess' | 'trojan' | 'shadowsocks'
    active: boolean
    latency_ms: number
}

export type Status = {
    connected: boolean
    xray_running: boolean
    restarting: boolean
    current_server: string
    protocol: string
    latency_ms: number
    uptime: string
    last_check: string
    watchdog_active: boolean
}

export type SubscriptionInfo = {
    url: string
    last_updated: string
    server_count: number
}

export type AuthStatus = {
    setup_required: boolean
}

export type SetupResponse = {
    totp_secret: string
    totp_qr: string
}

export type TokenResponse = {
    token: string
}

export type ApiError = {
    error: string
}
