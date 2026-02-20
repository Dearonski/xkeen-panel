import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
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
    loading,
}: {
    server: Server
    onSelect: (id: number) => void
    loading: boolean
}) {
    return (
        <Card
            className={cn(
                'transition-colors',
                server.active &&
                    'border-emerald-500/50 shadow-[0_0_12px_rgba(16,185,129,0.08)]',
            )}
        >
            <CardContent className='flex items-start justify-between gap-3 py-3'>
                <div className='min-w-0 flex-1'>
                    <div className='flex items-center gap-2 mb-1.5'>
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
                        <span className='text-muted-foreground'>
                            {maskAddress(server.address)}:{server.port}
                        </span>
                        {server.latency_ms > 0 && (
                            <span
                                className={cn(
                                    'font-mono',
                                    server.latency_ms < 200
                                        ? 'text-emerald-400'
                                        : server.latency_ms < 500
                                          ? 'text-amber-400'
                                          : 'text-red-400',
                                )}
                            >
                                {server.latency_ms}ms
                            </span>
                        )}
                        {server.latency_ms === -1 && (
                            <span className='text-muted-foreground'>—</span>
                        )}
                    </div>
                </div>

                {!server.active && (
                    <Button
                        size='sm'
                        variant='outline'
                        onClick={() => onSelect(server.id)}
                        disabled={loading}
                    >
                        Выбрать
                    </Button>
                )}
            </CardContent>
        </Card>
    )
}
