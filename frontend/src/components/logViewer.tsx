import { useState, useRef, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { IconChevronDown, IconRefresh } from '@tabler/icons-react'

export function LogViewer({
    logs,
    onRefresh,
    loading,
}: {
    logs: string[]
    onRefresh: () => void
    loading: boolean
}) {
    const [expanded, setExpanded] = useState(false)
    const scrollRef = useRef<HTMLDivElement>(null)

    useEffect(() => {
        if (expanded && scrollRef.current) {
            scrollRef.current.scrollTop = scrollRef.current.scrollHeight
        }
    }, [logs, expanded])

    return (
        <div className='rounded-xl border bg-card'>
            <button
                onClick={() => setExpanded(!expanded)}
                className='w-full flex items-center justify-between p-4 hover:bg-muted/50 transition-colors rounded-xl'
            >
                <span className='text-base font-semibold'>Логи</span>
                <IconChevronDown
                    className={cn(
                        'size-5 text-muted-foreground transition-transform',
                        expanded && 'rotate-180',
                    )}
                />
            </button>

            {expanded && (
                <div className='border-t'>
                    <div className='flex justify-end p-2'>
                        <Button
                            variant='ghost'
                            size='sm'
                            onClick={onRefresh}
                            disabled={loading}
                        >
                            <IconRefresh
                                className={cn(
                                    'size-4',
                                    loading && 'animate-spin',
                                )}
                            />
                            {loading ? 'Загрузка...' : 'Обновить'}
                        </Button>
                    </div>
                    <div
                        ref={scrollRef}
                        className='max-h-48 lg:max-h-80 overflow-y-auto px-4 pb-4'
                    >
                        {logs.length === 0 ? (
                            <p className='text-sm text-muted-foreground'>
                                Нет записей
                            </p>
                        ) : (
                            <pre className='text-xs font-mono text-muted-foreground whitespace-pre-wrap break-all space-y-0.5'>
                                {logs.map((line, i) => (
                                    <div
                                        key={i}
                                        className={cn(
                                            (line.includes('[FAIL]') ||
                                                line.includes('[ERROR]')) &&
                                                'text-red-400',
                                            line.includes('[OK]') &&
                                                'text-emerald-400',
                                            line.includes('[FAILOVER]') &&
                                                'text-amber-400',
                                        )}
                                    >
                                        {line}
                                    </div>
                                ))}
                            </pre>
                        )}
                    </div>
                </div>
            )}
        </div>
    )
}
