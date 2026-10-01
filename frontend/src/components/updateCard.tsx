import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
    IconArrowBackUp,
    IconDownload,
    IconLoader2,
    IconRefresh,
} from '@tabler/icons-react'
import { api } from '@/lib/api'
import type { UpdateState } from '@/types'

const dateOf = (iso?: string) =>
    iso && !iso.startsWith('0001')
        ? new Date(iso).toLocaleString('ru-RU', {
              day: 'numeric',
              month: 'short',
              hour: '2-digit',
              minute: '2-digit',
          })
        : null

// Release notes are GitHub markdown; the card shows them as plain text
const plainNotes = (md: string) =>
    md.replace(/\*\*/g, '').replace(/^#+\s*/gm, '')

export function UpdateCard() {
    const qc = useQueryClient()
    const [confirmRollback, setConfirmRollback] = useState(false)
    const [showNotes, setShowNotes] = useState(false)

    const update = useQuery({
        queryKey: ['update'],
        queryFn: () => api.get<UpdateState>('/api/update'),
    })

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

    const u = update.data
    if (!u) return null

    const busy =
        u.updating || check.isPending || apply.isPending || rollback.isPending
    const error =
        (check.error ?? apply.error ?? rollback.error ?? setAuto.error)
            ?.message ??
        u.apply_error ??
        u.check_error
    const lastCheck = dateOf(u.last_check)
    const hour = String(u.auto_hour).padStart(2, '0')

    const onRollback = () => {
        if (confirmRollback) {
            rollback.mutate()
            setConfirmRollback(false)
        } else {
            setConfirmRollback(true)
            setTimeout(() => setConfirmRollback(false), 5000)
        }
    }

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base flex items-center justify-between'>
                    Обновления
                    <Badge variant='outline'>{u.current}</Badge>
                </CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
                {!u.supported ? (
                    <p className='text-xs text-muted-foreground'>
                        Самообновление недоступно: {u.reason}
                    </p>
                ) : (
                    <>
                        <div className='flex items-center justify-between gap-3'>
                            <Label
                                htmlFor='auto-update'
                                className='text-sm font-normal'
                            >
                                Автообновление
                            </Label>
                            <Switch
                                id='auto-update'
                                checked={u.auto}
                                onCheckedChange={auto => setAuto.mutate(auto)}
                                disabled={setAuto.isPending}
                            />
                        </div>
                        <p className='text-xs text-muted-foreground'>
                            {u.auto
                                ? `Новые версии ставятся сами ночью, в ${hour}:00 по времени роутера.`
                                : 'Панель только проверяет новые версии — ставить их нужно кнопкой.'}
                        </p>

                        {u.updating ? (
                            <div className='flex items-center gap-2 text-sm text-amber-400'>
                                <IconLoader2 className='size-4 animate-spin' />
                                Устанавливаю {u.latest?.tag} — панель
                                перезапустится
                            </div>
                        ) : u.available && u.latest ? (
                            <div className='space-y-2 rounded-md border border-emerald-500/30 bg-emerald-500/5 p-3'>
                                <div className='flex items-center justify-between gap-2 text-sm'>
                                    <span>
                                        Доступна{' '}
                                        <span className='font-semibold'>
                                            {u.latest.tag}
                                        </span>
                                    </span>
                                    {u.latest.notes && (
                                        <button
                                            type='button'
                                            onClick={() =>
                                                setShowNotes(v => !v)
                                            }
                                            className='text-xs text-muted-foreground hover:text-foreground underline decoration-dotted'
                                        >
                                            {showNotes
                                                ? 'скрыть'
                                                : 'что нового'}
                                        </button>
                                    )}
                                </div>
                                {showNotes && (
                                    <pre className='max-h-48 overflow-auto whitespace-pre-wrap text-xs text-muted-foreground font-sans'>
                                        {plainNotes(u.latest.notes)}
                                    </pre>
                                )}
                                {u.skip === u.latest.tag && (
                                    <p className='text-xs text-amber-400'>
                                        Эту версию автообновление пропускает
                                        {u.failed_start === u.latest.tag
                                            ? ' — она не запустилась и была откачена'
                                            : ' — с неё был откат'}
                                        . Поставить её можно только вручную.
                                    </p>
                                )}
                                <Button
                                    className='w-full'
                                    onClick={() => apply.mutate()}
                                    disabled={busy}
                                >
                                    <IconDownload className='size-4' />
                                    Обновить до {u.latest.tag}
                                </Button>
                            </div>
                        ) : (
                            <p className='text-xs text-muted-foreground'>
                                Установлена последняя версия
                                {lastCheck && ` (проверено ${lastCheck})`}
                            </p>
                        )}

                        {u.failed_start && u.skip !== u.latest?.tag && (
                            <p className='text-xs text-amber-400'>
                                {u.failed_start} не запустилась — панель
                                вернулась на {u.current}.
                            </p>
                        )}
                        {error && !u.updating && (
                            <p className='text-xs text-red-400'>{error}</p>
                        )}

                        <div className='flex gap-2'>
                            <Button
                                variant='outline'
                                className='flex-1'
                                onClick={() => check.mutate()}
                                disabled={busy}
                            >
                                <IconRefresh
                                    className={
                                        check.isPending
                                            ? 'size-4 animate-spin'
                                            : 'size-4'
                                    }
                                />
                                Проверить
                            </Button>
                            {u.previous && (
                                <Button
                                    variant={
                                        confirmRollback
                                            ? 'destructive'
                                            : 'outline'
                                    }
                                    className='flex-1'
                                    onClick={onRollback}
                                    disabled={busy}
                                >
                                    <IconArrowBackUp className='size-4' />
                                    {confirmRollback
                                        ? 'Точно откатить?'
                                        : `Откатить на ${u.previous}`}
                                </Button>
                            )}
                        </div>
                    </>
                )}
            </CardContent>
        </Card>
    )
}
