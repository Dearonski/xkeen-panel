import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import type { ListResponse, SettingsResponse } from '@/types'

const LISTS: { name: string; label: string; hint: string }[] = [
    {
        name: 'port_proxying',
        label: 'Порты проксирования',
        hint: 'Один порт или диапазон вида 596:599 в строке. # — комментарий',
    },
    {
        name: 'port_exclude',
        label: 'Порты-исключения',
        hint: 'Нельзя использовать вместе с портами проксирования — приоритет у них',
    },
    {
        name: 'ip_exclude',
        label: 'IP-исключения',
        hint: 'Обязательно с маской: 10.0.0.1/32, 192.168.0.0/16',
    },
]

export function SettingsCard() {
    const qc = useQueryClient()
    const [tab, setTab] = useState<string>('xkeen')
    const [error, setError] = useState<string | null>(null)
    const [saved, setSaved] = useState<string | null>(null)

    const settings = useQuery({
        queryKey: ['xkeen-settings'],
        queryFn: () => api.get<SettingsResponse>('/api/xkeen/settings'),
    })

    const list = useQuery({
        queryKey: ['xkeen-list', tab],
        queryFn: () => api.get<ListResponse>(`/api/xkeen/lists/${tab}`),
        enabled: tab !== 'xkeen',
    })

    const [ghProxy, setGhProxy] = useState('')
    const [listText, setListText] = useState('')

    useEffect(() => {
        const section = settings.data?.settings?.xkeen
        setGhProxy(
            typeof section?.gh_proxy === 'string' ? section.gh_proxy : '',
        )
    }, [settings.data])

    useEffect(() => {
        setListText(list.data?.content ?? '')
    }, [list.data])

    const report = (message: string | null, err: unknown) => {
        setSaved(message)
        setError(err instanceof Error ? err.message : err ? String(err) : null)
        if (message) setTimeout(() => setSaved(null), 3000)
    }

    const saveSettings = useMutation({
        mutationFn: () => {
            const current = settings.data?.settings ?? {}
            const section = {
                ...((current.xkeen as Record<string, unknown>) ?? {}),
            }
            if (ghProxy.trim()) {
                section.gh_proxy = ghProxy.trim()
            } else {
                delete section.gh_proxy
            }

            const next = { ...current }
            if (Object.keys(section).length > 0) {
                next.xkeen = section
            } else {
                delete next.xkeen
            }

            return api.put('/api/xkeen/settings', { settings: next })
        },
        onSuccess: () => {
            report('Сохранено', null)
            qc.invalidateQueries({ queryKey: ['xkeen-settings'] })
        },
        onError: err => report(null, err),
    })

    const saveList = useMutation({
        mutationFn: () =>
            api.put(`/api/xkeen/lists/${tab}`, { content: listText }),
        onSuccess: () => {
            report('Сохранено — примените перезапуском XKeen', null)
            qc.invalidateQueries({ queryKey: ['xkeen-list', tab] })
        },
        onError: err => report(null, err),
    })

    const active = LISTS.find(l => l.name === tab)

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base'>Настройки XKeen</CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
                <div className='flex flex-wrap gap-1'>
                    <Button
                        size='sm'
                        variant={tab === 'xkeen' ? 'default' : 'outline'}
                        onClick={() => setTab('xkeen')}
                    >
                        xkeen.json
                    </Button>
                    {LISTS.map(l => (
                        <Button
                            key={l.name}
                            size='sm'
                            variant={tab === l.name ? 'default' : 'outline'}
                            onClick={() => setTab(l.name)}
                        >
                            {l.label}
                        </Button>
                    ))}
                </div>

                {tab === 'xkeen' ? (
                    <div className='space-y-2'>
                        <Label htmlFor='gh-proxy'>
                            Прокси для загрузок с GitHub
                        </Label>
                        <Input
                            id='gh-proxy'
                            value={ghProxy}
                            onChange={e => setGhProxy(e.target.value)}
                            placeholder='https://gh-proxy.com'
                        />
                        <p className='text-xs text-muted-foreground'>
                            Используется XKeen при обновлении компонентов, когда
                            GitHub недоступен напрямую.
                        </p>
                        <Button
                            className='w-full'
                            onClick={() => saveSettings.mutate()}
                            disabled={saveSettings.isPending}
                        >
                            Сохранить
                        </Button>
                    </div>
                ) : (
                    <div className='space-y-2'>
                        <textarea
                            className='w-full h-40 rounded-md border bg-transparent p-2 font-mono text-xs'
                            value={listText}
                            onChange={e => setListText(e.target.value)}
                            spellCheck={false}
                        />
                        <p className='text-xs text-muted-foreground'>
                            {active?.hint}
                        </p>
                        <Button
                            className='w-full'
                            onClick={() => saveList.mutate()}
                            disabled={saveList.isPending || list.isLoading}
                        >
                            Сохранить
                        </Button>
                    </div>
                )}

                {error && <p className='text-xs text-red-400'>{error}</p>}
                {saved && <p className='text-xs text-emerald-400'>{saved}</p>}
            </CardContent>
        </Card>
    )
}
