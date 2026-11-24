import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { IconLoader2 } from '@tabler/icons-react'
import { Group, PageTitle, Row, Switch } from '@/components/ios'
import { useCoreUpdate, useInfraUpdate, useUpdate } from '@/hooks/usePanelData'
import { api } from '@/lib/api'
import { isSet } from '@/lib/format'
import type { CoreUpdateState, InfraUpdateState, UpdateState } from '@/types'

const back = { to: '/settings', label: 'Настройки' }

// Release notes are GitHub markdown; plain text reads fine here
const plainNotes = (md: string) =>
    md.replace(/\*\*/g, '').replace(/^#+\s*/gm, '')

const withV = (version: string) =>
    version.startsWith('v') ? version : `v${version}`

const checkedAt = (iso?: string) =>
    isSet(iso)
        ? new Date(iso!).toLocaleString('ru-RU', {
              day: 'numeric',
              month: 'short',
              hour: '2-digit',
              minute: '2-digit',
          })
        : null

export function UpdatePage() {
    const qc = useQueryClient()
    const u = useUpdate().data
    const [showNotes, setShowNotes] = useState(false)

    const store = (state: UpdateState) => qc.setQueryData(['update'], state)
    const check = useMutation({
        mutationFn: () => api.post<UpdateState>('/api/update/check'),
        onSuccess: store,
    })
    const apply = useMutation({
        mutationFn: () => api.post<UpdateState>('/api/update/apply'),
        onSuccess: store,
    })
    const setAuto = useMutation({
        mutationFn: (auto: boolean) =>
            api.put<UpdateState>('/api/update/settings', { auto }),
        onMutate: auto =>
            qc.setQueryData<UpdateState>(['update'], old =>
                old ? { ...old, auto } : old,
            ),
        onSuccess: store,
        onError: () => qc.invalidateQueries({ queryKey: ['update'] }),
    })

    if (!u) return <PageTitle title='Обновления' back={back} />

    const busy = u.updating || check.isPending || apply.isPending
    const error =
        (check.error ?? apply.error ?? setAuto.error)?.message ??
        u.apply_error ??
        u.check_error
    const hour = String(u.auto_hour).padStart(2, '0')
    const lastCheck = checkedAt(u.last_check)

    if (!u.supported) {
        return (
            <>
                <PageTitle title='Обновления' back={back} />
                <div className='grid grid-cols-1 gap-5 lg:grid-cols-2 lg:items-start'>
                    <Group title='Панель'>
                        <Row
                            label='Самообновление недоступно'
                            hint={u.reason}
                            detail={u.current}
                        />
                    </Group>
                    <XKeenGroup />
                    <CoreGroup />
                    <GeoGroup />
                </div>
            </>
        )
    }

    return (
        <>
            <PageTitle title='Обновления' back={back} />

            <div className='grid grid-cols-1 gap-5 lg:grid-cols-2 lg:items-start'>
                <Group
                    title='Панель'
                    footer={
                        u.auto
                            ? `Новые версии ставятся сами ночью, в ${hour}:00 по времени роутера.`
                            : 'Панель только проверяет новые версии — ставить их нужно вручную.'
                    }
                >
                    <Row
                        label='Обновляться автоматически'
                        trailing={
                            <Switch
                                checked={u.auto}
                                onChange={auto => setAuto.mutate(auto)}
                                label='Обновляться автоматически'
                                disabled={setAuto.isPending}
                            />
                        }
                    />
                </Group>

                <Group
                    footer={
                        error && !u.updating ? (
                            <span className='text-destructive'>{error}</span>
                        ) : undefined
                    }
                >
                    {u.updating ? (
                        <Row
                            icon={
                                <IconLoader2 className='size-5 animate-spin text-warning' />
                            }
                            label={`Устанавливаю ${u.latest?.tag ?? ''}`}
                            hint='Панель перезапустится сама'
                        />
                    ) : u.available && u.latest ? (
                        <>
                            <Row
                                label={`Доступна ${u.latest.tag}`}
                                hint={
                                    u.skip === u.latest.tag
                                        ? u.failed_start === u.latest.tag
                                            ? 'Не запустилась и была откачена — ставится только вручную'
                                            : 'Ставится только вручную'
                                        : undefined
                                }
                            />
                            {u.latest.notes && (
                                <Row
                                    label={
                                        showNotes
                                            ? 'Скрыть изменения'
                                            : 'Что нового'
                                    }
                                    tone='tint'
                                    onClick={() => setShowNotes(v => !v)}
                                />
                            )}
                            {showNotes && u.latest.notes && (
                                <pre className='max-h-72 overflow-auto whitespace-pre-wrap py-3 font-sans text-[13px] leading-snug text-muted-foreground'>
                                    {plainNotes(u.latest.notes)}
                                </pre>
                            )}
                            <Row
                                label={`Обновить до ${u.latest.tag}`}
                                tone='tint'
                                onClick={() => apply.mutate()}
                                disabled={busy}
                            />
                        </>
                    ) : (
                        <Row
                            label='Установлена последняя версия'
                            hint={
                                lastCheck ? `Проверено ${lastCheck}` : undefined
                            }
                        />
                    )}
                    {u.failed_start && u.skip !== u.latest?.tag && (
                        <Row
                            label={`${u.failed_start} не запустилась`}
                            hint={`Панель вернулась на ${u.current}`}
                        />
                    )}
                    <Row
                        label={
                            check.isPending ? 'Проверяю…' : 'Проверить сейчас'
                        }
                        tone='tint'
                        onClick={() => check.mutate()}
                        disabled={busy}
                    />
                </Group>

                <XKeenGroup />
                <CoreGroup />
                <GeoGroup />
            </div>
        </>
    )
}

function CoreGroup() {
    const qc = useQueryClient()
    const c = useCoreUpdate().data
    const [showNotes, setShowNotes] = useState(false)

    const store = (state: CoreUpdateState) =>
        qc.setQueryData(['core-update'], state)
    const check = useMutation({
        mutationFn: () => api.post<CoreUpdateState>('/api/core/update/check'),
        onSuccess: store,
    })
    const apply = useMutation({
        mutationFn: () => api.post<CoreUpdateState>('/api/core/update/apply'),
        onSuccess: store,
    })

    if (!c) return null

    if (!c.supported) {
        return (
            <Group title='Ядро Xray'>
                <Row label='Обновление недоступно' hint={c.reason} />
            </Group>
        )
    }

    const busy = c.updating || check.isPending || apply.isPending
    const error =
        (check.error ?? apply.error)?.message ?? c.apply_error ?? c.check_error
    const lastCheck = checkedAt(c.last_check)

    return (
        <Group
            title='Ядро Xray'
            footer={
                error && !c.updating ? (
                    <span className='text-destructive'>{error}</span>
                ) : (
                    'Перед заменой панель проверит новую версию на текущих настройках. Если ядро не запустится, вернётся прежняя.'
                )
            }
        >
            {c.updating ? (
                <Row
                    icon={
                        <IconLoader2 className='size-5 animate-spin text-warning' />
                    }
                    label={c.step || 'Обновляю ядро'}
                    hint='Прокси прервётся на несколько секунд'
                />
            ) : c.available && c.latest ? (
                <>
                    <Row
                        label={`Доступна ${c.latest.tag}`}
                        hint={`Установлена ${withV(c.current)}`}
                    />
                    {c.latest.notes && (
                        <Row
                            label={
                                showNotes ? 'Скрыть изменения' : 'Что нового'
                            }
                            tone='tint'
                            onClick={() => setShowNotes(v => !v)}
                        />
                    )}
                    {showNotes && c.latest.notes && (
                        <pre className='max-h-72 overflow-auto whitespace-pre-wrap py-3 font-sans text-[13px] leading-snug text-muted-foreground'>
                            {plainNotes(c.latest.notes)}
                        </pre>
                    )}
                    <Row
                        label={`Обновить Xray до ${c.latest.tag}`}
                        tone='tint'
                        onClick={() => apply.mutate()}
                        disabled={busy}
                    />
                </>
            ) : (
                <Row
                    label={c.current ? `Xray ${withV(c.current)}` : 'Xray'}
                    hint={
                        c.latest
                            ? `Последняя версия${lastCheck ? ` · проверено ${lastCheck}` : ''}`
                            : lastCheck
                              ? `Проверено ${lastCheck}`
                              : undefined
                    }
                />
            )}
            <Row
                label={check.isPending ? 'Проверяю…' : 'Проверить сейчас'}
                tone='tint'
                onClick={() => check.mutate()}
                disabled={busy}
            />
        </Group>
    )
}

function useInfraActions() {
    const qc = useQueryClient()
    const store = (state: InfraUpdateState) =>
        qc.setQueryData(['infra-update'], state)
    const check = useMutation({
        mutationFn: () => api.post<InfraUpdateState>('/api/infra/update/check'),
        onSuccess: store,
    })
    const apply = useMutation({
        mutationFn: (item: 'xkeen' | 'geo') =>
            api.post<InfraUpdateState>(`/api/infra/update/${item}/apply`),
        onSuccess: store,
    })
    return { check, apply }
}

function XKeenGroup() {
    const infra = useInfraUpdate().data
    const { check, apply } = useInfraActions()
    const [showNotes, setShowNotes] = useState(false)

    if (!infra) return null
    const x = infra.xkeen

    if (!x.supported) {
        return (
            <Group title='XKeen'>
                <Row
                    label='Обновление недоступно'
                    hint={x.reason}
                    detail={x.current || undefined}
                />
            </Group>
        )
    }

    const busy =
        x.updating || infra.geo.updating || check.isPending || apply.isPending
    const error =
        (check.error ?? apply.error)?.message ?? x.apply_error ?? x.check_error
    const lastCheck = checkedAt(x.last_check)

    return (
        <Group
            title='XKeen'
            footer={
                error && !x.updating ? (
                    <span className='text-destructive'>{error}</span>
                ) : (
                    'Панель запускает xkeen -uk: XKeen скачивает новую версию, заново регистрирует себя и перезапускает прокси.'
                )
            }
        >
            {x.updating ? (
                <Row
                    icon={
                        <IconLoader2 className='size-5 animate-spin text-warning' />
                    }
                    label='Обновляю XKeen'
                    hint='Прокси перезапустится, это займёт несколько минут'
                />
            ) : x.available && x.latest ? (
                <>
                    <Row
                        label={`Доступна ${x.latest.tag}`}
                        hint={`Установлена ${x.current}`}
                    />
                    {x.latest.notes && (
                        <Row
                            label={
                                showNotes ? 'Скрыть изменения' : 'Что нового'
                            }
                            tone='tint'
                            onClick={() => setShowNotes(v => !v)}
                        />
                    )}
                    {showNotes && x.latest.notes && (
                        <pre className='max-h-72 overflow-auto whitespace-pre-wrap py-3 font-sans text-[13px] leading-snug text-muted-foreground'>
                            {plainNotes(x.latest.notes)}
                        </pre>
                    )}
                    <Row
                        label={`Обновить XKeen до ${x.latest.tag}`}
                        tone='tint'
                        onClick={() => apply.mutate('xkeen')}
                        disabled={busy}
                    />
                </>
            ) : (
                <Row
                    label={`XKeen ${x.current}`}
                    hint={
                        x.latest
                            ? `Последняя версия${lastCheck ? ` · проверено ${lastCheck}` : ''}`
                            : lastCheck
                              ? `Проверено ${lastCheck}`
                              : undefined
                    }
                />
            )}
            <Row
                label={check.isPending ? 'Проверяю…' : 'Проверить сейчас'}
                tone='tint'
                onClick={() => check.mutate()}
                disabled={busy}
            />
        </Group>
    )
}

function GeoGroup() {
    const infra = useInfraUpdate().data
    const { apply } = useInfraActions()

    if (!infra) return null
    const g = infra.geo

    if (!g.supported) {
        return (
            <Group title='Геобазы'>
                <Row label='Обновление недоступно' hint={g.reason} />
            </Group>
        )
    }

    const busy = g.updating || infra.xkeen.updating || apply.isPending
    const error = apply.error?.message ?? g.apply_error
    const updated = checkedAt(g.updated_at)

    return (
        <Group
            title='Геобазы'
            footer={
                error && !g.updating ? (
                    <span className='text-destructive'>{error}</span>
                ) : (
                    `${g.schedule ? `XKeen обновляет базы сам ${g.schedule}.` : 'Автообновление баз в XKeen не настроено.'} После обновления прокси перезапускается.`
                )
            }
        >
            {g.updating ? (
                <Row
                    icon={
                        <IconLoader2 className='size-5 animate-spin text-warning' />
                    }
                    label='Обновляю геобазы'
                    hint='Прокси прервётся на несколько секунд'
                />
            ) : (
                <Row
                    label='Базы GeoIP и GeoSite'
                    hint={[
                        updated && `Обновлены ${updated}`,
                        `файлов: ${g.files}`,
                    ]
                        .filter(Boolean)
                        .join(' · ')}
                />
            )}
            <Row
                label='Обновить сейчас'
                tone='tint'
                onClick={() => apply.mutate('geo')}
                disabled={busy}
            />
        </Group>
    )
}
