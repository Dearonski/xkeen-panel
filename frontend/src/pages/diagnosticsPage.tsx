import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
    IconCircleCheckFilled,
    IconCircleXFilled,
    IconAlertCircleFilled,
} from '@tabler/icons-react'
import { Group, PageTitle, Row } from '@/components/ios'
import { useLogs, usePool, useStatus } from '@/hooks/usePanelData'
import { api } from '@/lib/api'
import { isSet, timeAgo } from '@/lib/format'
import type { PoolSyncResult, SelfTestResult, Status } from '@/types'

const back = { to: '/settings', label: 'Настройки' }

type Verdict = 'ok' | 'bad' | 'unknown'

function Check({
    verdict,
    label,
    detail,
}: {
    verdict: Verdict
    label: string
    detail?: ReactNode
}) {
    const icon =
        verdict === 'ok' ? (
            <IconCircleCheckFilled className='size-[22px] text-success' />
        ) : verdict === 'bad' ? (
            <IconCircleXFilled className='size-[22px] text-destructive' />
        ) : (
            <IconAlertCircleFilled className='size-[22px] text-label-tertiary' />
        )
    return <Row icon={icon} label={label} detail={detail} />
}

export function DiagnosticsPage() {
    const qc = useQueryClient()
    const s = useStatus().data
    const pool = usePool().data
    const logs = useLogs().data ?? []
    const [confirming, setConfirming] = useState<
        'restart' | 'stop' | 'pool' | null
    >(null)
    const logRef = useRef<HTMLDivElement>(null)

    useEffect(() => {
        if (logRef.current)
            logRef.current.scrollTop = logRef.current.scrollHeight
    }, [logs.length])

    const markRestarting = () =>
        qc.setQueryData<Status>(['status'], old =>
            old ? { ...old, restarting: true } : old,
        )
    const settle = () => {
        qc.invalidateQueries({ queryKey: ['status'] })
        qc.invalidateQueries({ queryKey: ['pool'] })
    }

    const restart = useMutation({
        mutationFn: () => api.post('/api/xkeen/restart'),
        onMutate: markRestarting,
        onSettled: settle,
    })
    const start = useMutation({
        mutationFn: () => api.post('/api/xkeen/start'),
        onSettled: settle,
    })
    const stop = useMutation({
        mutationFn: () => api.post('/api/xkeen/stop'),
        onSettled: settle,
    })
    const selftest = useMutation({
        mutationFn: () => api.post<SelfTestResult>('/api/xkeen/selftest'),
    })
    const poolAction = useMutation({
        mutationFn: (action: 'enable' | 'sync') =>
            api.post<PoolSyncResult>(`/api/pool/${action}`),
        onMutate: markRestarting,
        onSettled: settle,
    })
    const mihomo = useMutation({
        mutationFn: () => api.post('/api/mihomo/sync'),
        onMutate: markRestarting,
        onSettled: settle,
    })

    const confirm = (action: 'restart' | 'stop' | 'pool', run: () => void) => {
        if (confirming === action) {
            run()
            setConfirming(null)
            return
        }
        setConfirming(action)
        setTimeout(() => setConfirming(c => (c === action ? null : c)), 4000)
    }

    const checked = isSet(s?.last_check)
    const proxyVerdict: Verdict =
        !checked || !s?.proxy_checked ? 'unknown' : s.connected ? 'ok' : 'bad'
    const busy =
        s?.restarting || restart.isPending || start.isPending || stop.isPending
    const isPool = pool?.mode === 'pool'
    const sync = poolAction.data

    return (
        <>
            <PageTitle
                title='Диагностика'
                back={back}
                subtitle={
                    checked
                        ? `Последняя проверка ${timeAgo(s?.last_check)} назад`
                        : undefined
                }
            />

            <div className='grid grid-cols-1 gap-5 lg:grid-cols-2 lg:items-start'>
                <div className='flex flex-col gap-5'>
                    <Group
                        title='Проверки'
                        footer={
                            s && !s.proxy_checked && checked
                                ? 'Связь проверена напрямую с роутера: у ядра нет проверочного входа, поэтому работа самого VPN не подтверждена.'
                                : undefined
                        }
                    >
                        <Check
                            verdict={proxyVerdict}
                            label='Прокси через VPN'
                            detail={
                                s?.connected && s.latency_ms > 0
                                    ? `${s.latency_ms} мс`
                                    : undefined
                            }
                        />
                        <Check
                            verdict={
                                !checked
                                    ? 'unknown'
                                    : s?.internet
                                      ? 'ok'
                                      : 'bad'
                            }
                            label='Интернет у роутера'
                        />
                        <Check
                            verdict={
                                !s ? 'unknown' : s.xray_running ? 'ok' : 'bad'
                            }
                            label={`Ядро ${s?.core ?? ''}`}
                            detail={s?.mode}
                        />
                        <Check
                            verdict={
                                !s
                                    ? 'unknown'
                                    : s.watchdog_active
                                      ? 'ok'
                                      : 'bad'
                            }
                            label='Смена упавшего сервера'
                            detail={s?.watchdog_active ? 'вкл.' : 'выкл.'}
                        />
                    </Group>

                    <Group
                        footer={
                            selftest.data ? (
                                <span
                                    className={
                                        selftest.data.success
                                            ? 'text-success'
                                            : 'text-destructive'
                                    }
                                >
                                    {selftest.data.success
                                        ? `Конфигурация ${selftest.data.core} корректна`
                                        : selftest.data.output ||
                                          selftest.data.error}
                                </span>
                            ) : undefined
                        }
                    >
                        <Row
                            label={
                                confirming === 'restart'
                                    ? 'Нажмите ещё раз для перезапуска'
                                    : 'Перезапустить XKeen'
                            }
                            tone='tint'
                            onClick={() =>
                                confirm('restart', () => restart.mutate())
                            }
                            disabled={busy}
                        />
                        <Row
                            label={
                                selftest.isPending
                                    ? 'Проверяю…'
                                    : 'Проверить конфигурацию'
                            }
                            tone='tint'
                            onClick={() => selftest.mutate()}
                            disabled={selftest.isPending}
                        />
                        {s?.xray_running === false ? (
                            <Row
                                label='Запустить прокси'
                                tone='tint'
                                onClick={() => start.mutate()}
                                disabled={busy}
                            />
                        ) : (
                            <Row
                                label={
                                    confirming === 'stop'
                                        ? 'Нажмите ещё раз — VPN отключится'
                                        : 'Остановить прокси'
                                }
                                tone='destructive'
                                onClick={() =>
                                    confirm('stop', () => stop.mutate())
                                }
                                disabled={busy}
                            />
                        )}
                    </Group>

                    <Group
                        title='Дополнительно'
                        footer={
                            (poolAction.error ?? mihomo.error)?.message ??
                            (sync
                                ? sync.changed
                                    ? `Пул обновлён: +${sync.added?.length ?? 0}, −${sync.removed?.length ?? 0}${sync.live ? ', без перезапуска' : ''}`
                                    : 'Пул уже совпадает с подпиской'
                                : s?.core === 'mihomo'
                                  ? 'Mihomo сам выбирает сервер в proxy-group; панель только приводит список серверов к подписке.'
                                  : isPool
                                    ? 'Панель синхронизирует пул сама при каждом обновлении подписки.'
                                    : 'Пул из лучших серверов подписки: Xray уходит с упавшего сервера без перезапуска. Конфиги перезаписываются, рядом остаются копии .bak.')
                        }
                    >
                        {s?.core === 'mihomo' ? (
                            <Row
                                label='Синхронизировать серверы Mihomo'
                                tone='tint'
                                onClick={() => mihomo.mutate()}
                                disabled={busy || mihomo.isPending}
                            />
                        ) : isPool ? (
                            <Row
                                label='Синхронизировать пул с подпиской'
                                tone='tint'
                                onClick={() => poolAction.mutate('sync')}
                                disabled={busy || poolAction.isPending}
                            />
                        ) : (
                            <Row
                                label={
                                    confirming === 'pool'
                                        ? 'Нажмите ещё раз — маршрутизация перепишется'
                                        : 'Собрать пул из подписки'
                                }
                                tone='tint'
                                onClick={() =>
                                    confirm('pool', () =>
                                        poolAction.mutate('enable'),
                                    )
                                }
                                disabled={
                                    busy ||
                                    poolAction.isPending ||
                                    s?.core !== 'xray'
                                }
                            />
                        )}
                    </Group>
                </div>

                <Group title='Журнал'>
                    <div
                        ref={logRef}
                        className='max-h-[60dvh] overflow-y-auto py-3 lg:max-h-[calc(100dvh-14rem)]'
                    >
                        {logs.map((line, i) => (
                            <div
                                key={i}
                                className='break-words font-mono text-[12px] leading-relaxed text-white/85'
                            >
                                {line}
                            </div>
                        ))}
                    </div>
                </Group>
            </div>
        </>
    )
}
