import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { IconKey, IconCopy, IconCheck } from '@tabler/icons-react'
import type { AccessKeyStatus } from '@/types'

export function AccessKeyCard() {
    const qc = useQueryClient()
    const [freshKey, setFreshKey] = useState('')
    const [copied, setCopied] = useState(false)

    const status = useQuery({
        queryKey: ['accountKey'],
        queryFn: () => api.get<AccessKeyStatus>('/api/account/key'),
    })

    const generate = useMutation({
        mutationFn: () =>
            api.post<{ access_key: string }>('/api/account/key'),
        onSuccess: data => {
            setFreshKey(data.access_key)
            setCopied(false)
            qc.invalidateQueries({ queryKey: ['accountKey'] })
        },
    })

    const revoke = useMutation({
        mutationFn: () => api.del('/api/account/key'),
        onSuccess: () => {
            setFreshKey('')
            qc.invalidateQueries({ queryKey: ['accountKey'] })
        },
    })

    const copyKey = async () => {
        try {
            await navigator.clipboard.writeText(freshKey)
            setCopied(true)
            setTimeout(() => setCopied(false), 2000)
        } catch {
            // буфер недоступен — пользователь скопирует вручную
        }
    }

    const hasKey = status.data?.has_key ?? false
    const busy = generate.isPending || revoke.isPending

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base flex items-center gap-2'>
                    <IconKey className='size-4' />
                    Ключ доступа
                </CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
                <p className='text-xs text-muted-foreground'>
                    Вход одним ключом без TOTP. Сохраните его в менеджере паролей
                    (Google/Apple) — вход будет в одно касание. Учтите: кто
                    угодно с этим ключом войдёт без 2FA.
                </p>

                {freshKey && (
                    <div className='space-y-1.5'>
                        <div className='p-2 bg-muted rounded-md font-mono text-xs break-all'>
                            {freshKey}
                        </div>
                        <div className='flex items-center gap-2'>
                            <Button
                                size='sm'
                                variant='outline'
                                onClick={copyKey}
                            >
                                {copied ? (
                                    <IconCheck className='size-4 text-emerald-400' />
                                ) : (
                                    <IconCopy className='size-4' />
                                )}
                                {copied ? 'Скопировано' : 'Копировать'}
                            </Button>
                            <span className='text-[11px] text-amber-400'>
                                Показывается один раз — сохраните сейчас
                            </span>
                        </div>
                    </div>
                )}

                {hasKey && !freshKey && (
                    <div className='text-xs text-muted-foreground'>
                        Ключ активен:{' '}
                        <span className='font-mono'>
                            ••••{status.data?.hint}
                        </span>
                    </div>
                )}

                <div className='flex items-center gap-2'>
                    <Button
                        size='sm'
                        variant='outline'
                        onClick={() => generate.mutate()}
                        disabled={busy}
                    >
                        {hasKey ? 'Перевыпустить' : 'Создать ключ'}
                    </Button>
                    {hasKey && (
                        <Button
                            size='sm'
                            variant='ghost'
                            className='text-destructive'
                            onClick={() => revoke.mutate()}
                            disabled={busy}
                        >
                            Отключить
                        </Button>
                    )}
                </div>
            </CardContent>
        </Card>
    )
}
