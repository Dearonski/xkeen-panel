import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { IconLoader2 } from '@tabler/icons-react'
import { Group, PageTitle, Row, Switch } from '@/components/ios'
import { useUpdate } from '@/hooks/usePanelData'
import { api } from '@/lib/api'
import { isSet } from '@/lib/format'
import type { UpdateState } from '@/types'

const back = { to: '/settings', label: 'Настройки' }

// Release notes are GitHub markdown; plain text reads fine here
const plainNotes = (md: string) =>
    md.replace(/\*\*/g, '').replace(/^#+\s*/gm, '')

export function UpdatePage() {
    const qc = useQueryClient()
    const u = useUpdate().data
    const [confirmRollback, setConfirmRollback] = useState(false)
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
    const rollback = useMutation({
        mutationFn: () => api.post<UpdateState>('/api/update/rollback'),
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

    const busy =
        u.updating || check.isPending || apply.isPending || rollback.isPending
    const error =
        (check.error ?? apply.error ?? rollback.error ?? setAuto.error)
            ?.message ??
        u.apply_error ??
        u.check_error
    const hour = String(u.auto_hour).padStart(2, '0')
    const lastCheck = isSet(u.last_check)
        ? new Date(u.last_check!).toLocaleString('ru-RU', {
              day: 'numeric',
              month: 'short',
              hour: '2-digit',
              minute: '2-digit',
          })
        : null

    const onRollback = () => {
        if (!confirmRollback) {
            setConfirmRollback(true)
            setTimeout(() => setConfirmRollback(false), 5000)
            return
        }
        rollback.mutate()
        setConfirmRollback(false)
    }

    if (!u.supported) {
        return (
            <>
                <PageTitle title='Обновления' back={back} />
                <Group>
                    <Row
                        label='Самообновление недоступно'
                        hint={u.reason}
                        detail={u.current}
                    />
                </Group>
            </>
        )
    }

    return (
        <>
            <PageTitle
                title='Обновления'
                subtitle={`Установлена ${u.current}`}
                back={back}
            />

            <div className='grid gap-5 lg:grid-cols-2 lg:items-start'>
                <Group
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
                                            : 'С неё был откат — ставится только вручную'
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
                    {u.previous && (
                        <Row
                            label={
                                confirmRollback
                                    ? 'Точно откатить?'
                                    : `Откатить на ${u.previous}`
                            }
                            tone='destructive'
                            onClick={onRollback}
                            disabled={busy}
                        />
                    )}
                </Group>
            </div>
        </>
    )
}
