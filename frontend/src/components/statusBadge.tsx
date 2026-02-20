import { cn } from '@/lib/utils'

export function StatusBadge({
    connected,
    xrayRunning,
    latency,
}: {
    connected: boolean
    xrayRunning: boolean
    latency: number
}) {
    return (
        <div className='flex items-center gap-3'>
            {/* Статус xray-процесса */}
            <div className='flex items-center gap-1.5'>
                <div
                    className={cn(
                        'w-2 h-2 rounded-full',
                        xrayRunning
                            ? 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.6)]'
                            : 'bg-destructive shadow-[0_0_6px_rgba(239,68,68,0.5)]',
                    )}
                />
                <span className='text-xs text-muted-foreground'>
                    {xrayRunning ? 'xray' : 'xray off'}
                </span>
            </div>

            {/* Статус соединения */}
            <div className='flex items-center gap-1.5'>
                <div
                    className={cn(
                        'w-2 h-2 rounded-full',
                        connected
                            ? 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.6)]'
                            : 'bg-zinc-500',
                    )}
                />
                <span className='text-xs text-muted-foreground'>
                    {connected ? 'online' : 'offline'}
                </span>
                {connected && latency > 0 && (
                    <span className='text-xs text-muted-foreground'>
                        {latency}ms
                    </span>
                )}
            </div>
        </div>
    )
}
