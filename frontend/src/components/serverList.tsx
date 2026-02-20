import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { IconActivityHeartbeat } from '@tabler/icons-react'
import { ServerCard } from './serverCard'
import type { Server } from '@/types'

const PAGE_SIZE = 12

export function ServerList({
    servers,
    onSelect,
    onCheckAll,
    loading,
}: {
    servers: Server[]
    onSelect: (id: number) => void
    onCheckAll: () => void
    loading: boolean
}) {
    const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)

    const sorted = [...servers].sort((a, b) => {
        if (a.active) return -1
        if (b.active) return 1
        if (a.latency_ms === -1 && b.latency_ms === -1) return 0
        if (a.latency_ms === -1) return 1
        if (b.latency_ms === -1) return -1
        return a.latency_ms - b.latency_ms
    })

    const visible = sorted.slice(0, visibleCount)
    const hasMore = visibleCount < sorted.length

    return (
        <div className='space-y-3'>
            <div className='flex items-center justify-between'>
                <h2 className='text-base font-semibold'>
                    Серверы{' '}
                    <span className='text-sm font-normal text-muted-foreground'>
                        ({servers.length})
                    </span>
                </h2>
                <Button
                    variant='outline'
                    size='sm'
                    onClick={onCheckAll}
                    disabled={loading}
                >
                    <IconActivityHeartbeat className='size-4' />
                    {loading ? 'Проверка...' : 'Пинг'}
                </Button>
            </div>

            <div className='grid grid-cols-1 md:grid-cols-2 gap-2'>
                {visible.map(server => (
                    <ServerCard
                        key={server.id}
                        server={server}
                        onSelect={onSelect}
                        loading={loading}
                    />
                ))}
            </div>

            {hasMore && (
                <Button
                    variant='ghost'
                    className='w-full text-muted-foreground'
                    onClick={() => setVisibleCount(c => c + PAGE_SIZE)}
                >
                    Показать ещё ({sorted.length - visibleCount})
                </Button>
            )}

            {servers.length === 0 && (
                <div className='text-center py-8 text-muted-foreground text-sm'>
                    Нет серверов. Добавьте подписку.
                </div>
            )}
        </div>
    )
}
