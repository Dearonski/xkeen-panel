import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { IconPin, IconWand } from '@tabler/icons-react'
import { cn, flagFor, latencyClass } from '@/lib/utils'
import type { PoolNode, PoolStatus, Server } from '@/types'

const timeOf = (iso: string) =>
    new Date(iso).toLocaleTimeString('ru-RU', {
        hour: '2-digit',
        minute: '2-digit',
    })

export function PoolNodes({
    pool,
    servers,
    onPin,
    onAuto,
    error,
    loading,
}: {
    pool: PoolStatus
    servers: Server[]
    onPin: (tag: string) => void
    onAuto: () => void
    error?: string
    loading: boolean
}) {
    const nodes = pool.nodes ?? []
    const active = pool.current_tag ?? pool.pinned_tag
    const manual = pool.pin_manual ?? false
    const canPin = pool.api_available !== false

    // The latency check streams into the server list, so read it from there
    const latencyOf = (node: PoolNode) =>
        servers.find(s => s.id === node.server_id)?.latency_ms ??
        node.latency_ms

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base flex items-center justify-between gap-2'>
                    <span>
                        Пул{' '}
                        <span className='text-sm font-normal text-muted-foreground'>
                            ({nodes.length})
                        </span>
                    </span>
                    <Badge variant='outline'>
                        {manual ? 'выбрано вручную' : 'автовыбор'}
                    </Badge>
                </CardTitle>
            </CardHeader>
            <CardContent className='space-y-2'>
                {!manual && pool.pin_note && (
                    <p className='text-xs text-amber-400'>
                        Ручной выбор снят: {pool.pin_note}
                    </p>
                )}
                {error && <p className='text-xs text-red-400'>{error}</p>}

                <div className='space-y-1'>
                    {nodes.map(node => {
                        const isActive = node.tag === active
                        const ms = latencyOf(node)
                        return (
                            <button
                                key={node.tag}
                                type='button'
                                disabled={
                                    loading || !canPin || (isActive && manual)
                                }
                                onClick={() => onPin(node.tag)}
                                className={cn(
                                    'w-full flex items-center gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors',
                                    'enabled:hover:bg-muted/50 disabled:cursor-default',
                                    isActive
                                        ? 'border-emerald-500/50 bg-emerald-500/5'
                                        : 'border-transparent',
                                )}
                            >
                                <span className='shrink-0'>
                                    {flagFor(node.name, node.country)}
                                </span>
                                <span className='truncate flex-1 min-w-0'>
                                    {node.name}
                                </span>
                                {node.server_id < 0 && (
                                    <Badge
                                        variant='outline'
                                        className='text-[10px] shrink-0'
                                    >
                                        нет в подписке
                                    </Badge>
                                )}
                                {node.excluded_until && (
                                    <span
                                        className='text-[11px] text-amber-400 shrink-0'
                                        title='Через эту ноду не работали сервисы — панель её пока не выбирает'
                                    >
                                        исключена до{' '}
                                        {timeOf(node.excluded_until)}
                                    </span>
                                )}
                                {isActive && manual && (
                                    <IconPin className='size-3.5 text-emerald-400 shrink-0' />
                                )}
                                <span
                                    className={cn(
                                        'font-mono text-xs w-14 text-right shrink-0',
                                        ms > 0
                                            ? latencyClass(ms)
                                            : 'text-muted-foreground',
                                    )}
                                >
                                    {ms > 0 ? `${ms}ms` : '—'}
                                </span>
                                {isActive && (
                                    <span className='shrink-0 w-2 h-2 rounded-full bg-emerald-500' />
                                )}
                            </button>
                        )
                    })}
                </div>

                {manual && (
                    <Button
                        variant='outline'
                        className='w-full'
                        onClick={onAuto}
                        disabled={loading}
                    >
                        <IconWand className='size-4' />
                        Вернуть автовыбор
                    </Button>
                )}
                <p className='text-xs text-muted-foreground'>
                    {manual
                        ? 'Панель держит выбранную ноду, даже если пинг вырос, и сменит её, только если через неё перестанут работать сервисы.'
                        : 'Панель сама держит трафик на лучшей ноде и меняет её, если через неё перестают работать сервисы. Нажмите на ноду, чтобы закрепить её вручную.'}
                </p>
            </CardContent>
        </Card>
    )
}
