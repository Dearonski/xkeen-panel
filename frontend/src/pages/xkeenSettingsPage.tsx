import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Group, PageTitle, Row } from '@/components/ios'
import { api } from '@/lib/api'
import type { ListResponse, SettingsResponse } from '@/types'

const back = { to: '/settings', label: 'Настройки' }

const LISTS = [
    {
        name: 'port_proxying',
        label: 'Порты проксирования',
        hint: 'Один порт или диапазон вида 596:599 в строке, # — комментарий. Пусто — все порты.',
    },
    {
        name: 'port_exclude',
        label: 'Порты-исключения',
        hint: 'Не используются вместе с портами проксирования — у тех приоритет.',
    },
    {
        name: 'ip_exclude',
        label: 'IP-исключения',
        hint: 'Обязательно с маской: 10.0.0.1/32, 192.168.0.0/16.',
    },
]

const fieldClass =
    'w-full rounded-xl bg-fill px-3 outline-none placeholder:text-muted-foreground'
const saveClass =
    'min-h-11 w-full cursor-pointer rounded-xl bg-primary text-[17px] font-semibold disabled:opacity-50'

export function XkeenSettingsPage() {
    const qc = useQueryClient()
    const settings = useQuery({
        queryKey: ['xkeen-settings'],
        queryFn: () => api.get<SettingsResponse>('/api/xkeen/settings'),
    })
    const [ghProxy, setGhProxy] = useState('')

    useEffect(() => {
        const proxy = settings.data?.settings?.xkeen?.gh_proxy
        setGhProxy(typeof proxy === 'string' ? proxy : '')
    }, [settings.data])

    const save = useMutation({
        mutationFn: () => {
            const current = settings.data?.settings ?? {}
            const section = {
                ...((current.xkeen as Record<string, unknown>) ?? {}),
            }
            if (ghProxy.trim()) section.gh_proxy = ghProxy.trim()
            else delete section.gh_proxy
            const next = { ...current }
            if (Object.keys(section).length > 0) next.xkeen = section
            else delete next.xkeen
            return api.put('/api/xkeen/settings', { settings: next })
        },
        onSuccess: () => qc.invalidateQueries({ queryKey: ['xkeen-settings'] }),
    })

    return (
        <>
            <PageTitle
                title='Порты и исключения'
                back={back}
                subtitle='Изменения списков применяются после перезапуска XKeen'
            />

            <div className='grid grid-cols-1 gap-5 lg:grid-cols-2 lg:items-start'>
                {LISTS.map(list => (
                    <ListEditor key={list.name} {...list} />
                ))}

                <Group
                    title='Загрузки с GitHub'
                    footer={
                        save.error ? (
                            <span className='text-destructive'>
                                {save.error.message}
                            </span>
                        ) : save.isSuccess ? (
                            <span className='text-success'>Сохранено</span>
                        ) : (
                            'Прокси, через который XKeen скачивает обновления, когда GitHub недоступен напрямую.'
                        )
                    }
                >
                    <div className='flex flex-col gap-2 py-3'>
                        <label htmlFor='gh-proxy' className='sr-only'>
                            Прокси для GitHub
                        </label>
                        <input
                            id='gh-proxy'
                            value={ghProxy}
                            onChange={e => setGhProxy(e.target.value)}
                            placeholder='https://gh-proxy.com'
                            className={`${fieldClass} min-h-11 text-[17px]`}
                        />
                        <button
                            type='button'
                            onClick={() => save.mutate()}
                            disabled={save.isPending}
                            className={saveClass}
                        >
                            Сохранить
                        </button>
                    </div>
                </Group>
            </div>
        </>
    )
}

function ListEditor({
    name,
    label,
    hint,
}: {
    name: string
    label: string
    hint: string
}) {
    const qc = useQueryClient()
    const [open, setOpen] = useState(false)
    const [text, setText] = useState('')
    const list = useQuery({
        queryKey: ['xkeen-list', name],
        queryFn: () => api.get<ListResponse>(`/api/xkeen/lists/${name}`),
        enabled: open,
    })

    useEffect(() => setText(list.data?.content ?? ''), [list.data])

    const save = useMutation({
        mutationFn: () =>
            api.put(`/api/xkeen/lists/${name}`, { content: text }),
        onSuccess: () =>
            qc.invalidateQueries({ queryKey: ['xkeen-list', name] }),
    })

    return (
        <Group
            footer={
                save.error ? (
                    <span className='text-destructive'>
                        {save.error.message}
                    </span>
                ) : save.isSuccess ? (
                    <span className='text-success'>
                        Сохранено — перезапустите XKeen в диагностике
                    </span>
                ) : (
                    hint
                )
            }
        >
            <Row
                label={label}
                detail={open ? 'Скрыть' : 'Изменить'}
                onClick={() => setOpen(v => !v)}
            />
            {open && (
                <div className='flex flex-col gap-2 py-3'>
                    <label htmlFor={`list-${name}`} className='sr-only'>
                        {label}
                    </label>
                    <textarea
                        id={`list-${name}`}
                        value={text}
                        onChange={e => setText(e.target.value)}
                        spellCheck={false}
                        rows={8}
                        className={`${fieldClass} py-2 font-mono text-[13px]`}
                    />
                    <button
                        type='button'
                        onClick={() => save.mutate()}
                        disabled={save.isPending || list.isLoading}
                        className={saveClass}
                    >
                        Сохранить
                    </button>
                </div>
            )}
        </Group>
    )
}
