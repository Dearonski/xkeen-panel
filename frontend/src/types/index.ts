export type Server = {
    id: number
    name: string
    address: string
    port: number
    protocol: 'vless' | 'vmess' | 'trojan' | 'shadowsocks'
    active: boolean
    latency_ms: number
    country?: string
    country_override?: string
    last_checked?: string
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
    core: 'xray' | 'mihomo'
    mode: string
    xkeen_version: string
    generation: number
}

export type PoolStatus = {
    mode: 'single' | 'pool'
    core: string
    balancer_tag?: string
    pool_tags?: string[]
    proxy_tags?: string[]
    pinned_tag?: string
    current_tag?: string
    api_available?: boolean
}

export type PoolSyncResult = {
    success: boolean
    changed: boolean
    added?: string[]
    removed?: string[]
    replaced?: string[]
    live: boolean
    restarting: boolean
}

export type SettingsResponse = {
    path: string
    settings: Record<string, any>
}

export type ListResponse = {
    path: string
    kind: 'ports' | 'ips'
    content: string
}

export type SelfTestResult = {
    success: boolean
    core: string
    output: string
    error?: string
}

export type SubscriptionInfo = {
    url: string
    last_updated: string
    server_count: number
}

export type AuthStatus = {
    setup_required: boolean
    passkey_enabled?: boolean
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
