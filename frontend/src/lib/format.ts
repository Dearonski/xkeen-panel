import type { Server, Status } from '@/types'

const regions = new Intl.DisplayNames(['ru'], { type: 'region' })

export function countryName(code?: string) {
    if (!code || code.length !== 2) return ''
    try {
        return regions.of(code.toUpperCase()) ?? code
    } catch {
        return code
    }
}

export function serverCountry(server?: Server) {
    return (server?.country_override || server?.country || '').toUpperCase()
}

// Go marshals an unset time as year 1
export function isSet(iso?: string) {
    return !!iso && !iso.startsWith('0001')
}

export function timeAgo(iso?: string) {
    if (!isSet(iso)) return ''
    const minutes = Math.round((Date.now() - new Date(iso!).getTime()) / 60000)
    if (minutes < 1) return 'только что'
    if (minutes < 60) return `${minutes} мин`
    const hours = Math.round(minutes / 60)
    if (hours < 24) return `${hours} ч`
    return `${Math.round(hours / 24)} дн`
}

export type Tone = 'good' | 'warn' | 'bad' | 'idle'

export function statusInfo(s?: Status): { label: string; tone: Tone } {
    if (!s) return { label: 'Загрузка', tone: 'idle' }
    if (s.restarting) return { label: 'Перезапуск', tone: 'warn' }
    if (!s.xray_running) return { label: 'Прокси остановлен', tone: 'bad' }
    if (!isSet(s.last_check)) return { label: 'Проверяю', tone: 'idle' }
    if (s.connected && s.proxy_checked)
        return { label: 'Защищено', tone: 'good' }
    if (s.connected) return { label: 'Не подтверждено', tone: 'warn' }
    if (!s.internet) return { label: 'Нет интернета', tone: 'bad' }
    return { label: 'VPN не работает', tone: 'bad' }
}

export const toneText: Record<Tone, string> = {
    good: 'text-success',
    warn: 'text-warning',
    bad: 'text-destructive',
    idle: 'text-muted-foreground',
}

export const toneFill: Record<Tone, string> = {
    good: 'bg-success/15',
    warn: 'bg-warning/15',
    bad: 'bg-destructive/15',
    idle: 'bg-fill',
}

export const toneDot: Record<Tone, string> = {
    good: 'bg-success',
    warn: 'bg-warning',
    bad: 'bg-destructive',
    idle: 'bg-muted-foreground',
}

export type PanelEvent = { time: string; text: string }

const routine = [
    /\[OK\]/,
    /Watchdog (запущен|остановлен)/,
    /\[HEALTH\] https?:/,
    /\[WARN\]/,
]

// The panel log doubles as an event feed once routine heartbeat lines are dropped
export function panelEvents(lines: string[], limit: number): PanelEvent[] {
    const events: PanelEvent[] = []
    for (let i = lines.length - 1; i >= 0 && events.length < limit; i--) {
        const line = lines[i]
        if (routine.some(re => re.test(line))) continue
        const m = line.match(
            /^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}):\d{2} (?:\[[^\]]+\] )?(.*)$/,
        )
        if (!m) continue
        const today = new Date().toISOString().slice(0, 10) === m[1]
        const day = today
            ? ''
            : new Date(m[1]).toLocaleDateString('ru-RU', {
                  day: 'numeric',
                  month: 'short',
              }) + ' '
        const text = m[3]
            .replace(/\s*\((manual|sub-\d+)\)/g, '')
            .replace(/:\s*(manual|sub-\d+)$/, '')
        events.push({ time: day + m[2], text })
    }
    return events
}

export function latencyTone(ms: number): Tone {
    if (ms < 0) return 'bad'
    if (ms < 200) return 'idle'
    if (ms < 500) return 'warn'
    return 'bad'
}
