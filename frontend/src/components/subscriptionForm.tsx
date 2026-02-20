import { useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { IconRefresh } from '@tabler/icons-react'
import type { SubscriptionInfo } from '@/types'

export function SubscriptionForm({
    subscription,
    onUpdate,
    onRefresh,
    loading,
}: {
    subscription: SubscriptionInfo | null
    onUpdate: (url: string) => void
    onRefresh: () => void
    loading: boolean
}) {
    const [url, setUrl] = useState('')
    const [expanded, setExpanded] = useState(false)

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault()
        if (url.trim()) {
            onUpdate(url.trim())
            setUrl('')
        }
    }

    const formatDate = (dateStr: string) => {
        if (!dateStr) return '—'
        return new Date(dateStr).toLocaleString('ru-RU', {
            day: '2-digit',
            month: '2-digit',
            year: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        })
    }

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base'>Подписка</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
                {subscription?.url && (
                    <div className='space-y-2 text-sm'>
                        <div className='flex items-center justify-between'>
                            <span className='text-muted-foreground'>URL:</span>
                            <button
                                onClick={() => setExpanded(!expanded)}
                                className='text-xs text-primary hover:underline'
                            >
                                {expanded ? 'Скрыть' : 'Показать'}
                            </button>
                        </div>
                        {expanded ? (
                            <p className='text-xs break-all font-mono bg-muted rounded-md p-2'>
                                {subscription.url}
                            </p>
                        ) : (
                            <p className='text-muted-foreground truncate'>
                                {subscription.url.substring(0, 40)}...
                            </p>
                        )}
                        <div className='flex justify-between'>
                            <span className='text-muted-foreground'>
                                Обновлено:
                            </span>
                            <span>{formatDate(subscription.last_updated)}</span>
                        </div>
                        <div className='flex justify-between'>
                            <span className='text-muted-foreground'>
                                Серверов:
                            </span>
                            <span>{subscription.server_count}</span>
                        </div>
                    </div>
                )}

                <form onSubmit={handleSubmit} className='space-y-3'>
                    <Input
                        type='url'
                        value={url}
                        onChange={e => setUrl(e.target.value)}
                        placeholder='https://provider.com/sub/...'
                    />
                    <div className='flex gap-2'>
                        <Button
                            type='submit'
                            disabled={!url.trim() || loading}
                            className='flex-1'
                        >
                            Применить
                        </Button>
                        {subscription?.url && (
                            <Button
                                type='button'
                                variant='outline'
                                size='icon'
                                onClick={onRefresh}
                                disabled={loading}
                            >
                                <IconRefresh
                                    className={loading ? 'animate-spin' : ''}
                                />
                            </Button>
                        )}
                    </div>
                </form>
            </CardContent>
        </Card>
    )
}
