import { useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn, flagFor, latencyClass } from '@/lib/utils'
import type { Server } from '@/types'

const protocolVariant: Record<string, string> = {
    vless: 'bg-violet-500/15 text-violet-400 border-violet-500/25',
    vmess: 'bg-amber-500/15 text-amber-400 border-amber-500/25',
    trojan: 'bg-red-500/15 text-red-400 border-red-500/25',
    shadowsocks: 'bg-cyan-500/15 text-cyan-400 border-cyan-500/25',
}

const maskAddress = (addr: string) => {
    const parts = addr.split('.')
    if (parts.length === 4) return `${parts[0]}.${parts[1]}.*.*`
    return addr.length > 12 ? addr.substring(0, 8) + '...' : addr
}

export function ServerCard({
    server,
    onSelect,
    onSetCountry,
    poolMode,
    loading,
}: {
    server: Server
    onSelect: (id: number) => void
    onSetCountry?: (id: number, country: string) => void
    poolMode?: boolean
    loading: boolean
}) {
    const [editing, setEditing] = useState(false)
    const [value, setValue] = useState(
        server.country_override || server.country || '',
    )

    const country = server.country_override || server.country
    const flag = flagFor(server.name, country)
    const outsidePool = poolMode && !server.pool_tag

    const saveCountry = () => {
        onSetCountry?.(server.id, value.trim().toUpperCase())
        setEditing(false)
    }

    return (
        <Card
            className={cn(
                'transition-colors',
                server.active &&
                    'border-emerald-500/50 shadow-[0_0_12px_rgba(16,185,129,0.08)]',
                outsidePool && 'opacity-60',
            )}
        >
            <CardContent className='flex items-start justify-between gap-3 py-3'>
                <div className='min-w-0 flex-1'>
                    <div className='flex items-center gap-2 mb-1.5'>
                        {flag && <span className='shrink-0'>{flag}</span>}
                        <span className='text-sm font-medium truncate'>
                            {server.name}
                        </span>
                        {server.active && (
                            <span className='shrink-0 w-2 h-2 rounded-full bg-emerald-500' />
                        )}
                    </div>

                    <div className='flex flex-wrap items-center gap-1.5 text-xs'>
                        <Badge
                            variant='outline'
                            className={cn(
                                'text-[10px] uppercase font-semibold',
                                protocolVariant[server.protocol],
                            )}
                        >
                            {server.protocol === 'shadowsocks'
                                ? 'SS'
                                : server.protocol}
                        </Badge>
                        {poolMode && server.pool_tag && (
                            <Badge
                                variant='outline'
                                className='text-[10px] bg-sky-500/15 text-sky-400 border-sky-500/25'
                            >
                                в пуле
                            </Badge>
                        )}
                        <span className='text-muted-foreground'>
                            {maskAddress(server.address)}:{server.port}
                        </span>
                        {server.latency_ms > 0 && (
                            <span
                                className={cn(
                                    'font-mono',
                                    latencyClass(server.latency_ms),
                                )}
                            >
                                {server.latency_ms}ms
                            </span>
                        )}
                        {server.latency_ms === -1 && (
                            <span className='text-muted-foreground'>—</span>
                        )}
                        {onSetCountry &&
                            (editing ? (
                                <span className='flex items-center gap-1'>
                                    <Input
                                        value={value}
                                        onChange={e =>
                                            setValue(
                                                e.target.value
                                                    .replace(/[^a-zA-Z]/g, '')
                                                    .slice(0, 2),
                                            )
                                        }
                                        placeholder='RU'
                                        className='h-6 w-12 px-1.5 text-[11px] uppercase'
                                        autoFocus
                                    />
                                    <button
                                        type='button'
                                        onClick={saveCountry}
                                        className='text-emerald-400 hover:text-emerald-300'
                                    >
                                        ✓
                                    </button>
                                    <button
                                        type='button'
                                        onClick={() => setEditing(false)}
                                        className='text-muted-foreground hover:text-foreground'
                                    >
                                        ✕
                                    </button>
                                </span>
                            ) : (
                                <button
                                    type='button'
                                    onClick={() => {
                                        setValue(country || '')
                                        setEditing(true)
                                    }}
                                    className='text-muted-foreground hover:text-foreground underline decoration-dotted'
                                >
                                    {country
                                        ? server.country_override
                                            ? `${country}*`
                                            : country
                                        : 'страна?'}
                                </button>
                            ))}
                    </div>
                </div>

                {outsidePool && (
                    <span
                        className='shrink-0 text-xs text-muted-foreground'
                        title='Пул собирается из лучших серверов подписки — закрепить можно только ноду из него'
                    >
                        не в пуле
                    </span>
                )}
                {!server.active && !outsidePool && (
                    <Button
                        size='sm'
                        variant='outline'
                        onClick={() => onSelect(server.id)}
                        disabled={loading}
                    >
                        {poolMode ? 'Закрепить' : 'Выбрать'}
                    </Button>
                )}
            </CardContent>
        </Card>
    )
}
