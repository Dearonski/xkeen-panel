import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@/hooks/useAuth'
import { useEventSource } from '@/hooks/useEventSource'
import { useStreamLatency } from '@/hooks/useStreamLatency'
import { api } from '@/lib/api'
import { StatusBadge } from '@/components/statusBadge'
import { SubscriptionForm } from '@/components/subscriptionForm'
import { ServerList } from '@/components/serverList'
import { Controls } from '@/components/controls'
import { XKeenCard } from '@/components/xkeenCard'
import { SettingsCard } from '@/components/settingsCard'
import { PasskeyCard } from '@/components/passkeyCard'
import { LogViewer } from '@/components/logViewer'
import { Button } from '@/components/ui/button'
import { IconLogout, IconLoader2 } from '@tabler/icons-react'
import type {
    Status,
    SubscriptionInfo,
    Server,
    SelfTestResult,
    PoolStatus,
} from '@/types'

export function DashboardPage() {
    const { logout } = useAuth()
    const qc = useQueryClient()

    // SSE: status, logs and restart events in real time
    useEventSource()
    const { check: checkLatency, checking: checkingLatency } =
        useStreamLatency()

    const status = useQuery({
        queryKey: ['status'],
        queryFn: () => api.get<Status>('/api/status'),
    })

    const restarting = status.data?.restarting ?? false

    const subscription = useQuery({
        queryKey: ['subscription'],
        queryFn: () => api.get<SubscriptionInfo>('/api/subscription'),
    })

    const servers = useQuery({
        queryKey: ['servers'],
        queryFn: () =>
            api
                .get<{ servers: Server[] }>('/api/servers')
                .then(d => d.servers ?? []),
    })

    const pool = useQuery({
        queryKey: ['pool'],
        queryFn: () => api.get<PoolStatus>('/api/pool'),
    })

    const logs = useQuery({
        queryKey: ['logs'],
        queryFn: () =>
            api
                .get<{ lines: string[] }>('/api/logs?lines=50')
                .then(d => d.lines ?? []),
    })

    const updateSub = useMutation({
        mutationFn: (url: string) =>
            api.post<{ servers: Server[] }>('/api/subscription', { url }),
        onSettled: () => {
            qc.invalidateQueries({ queryKey: ['subscription'] })
            qc.invalidateQueries({ queryKey: ['servers'] })
        },
    })

    const refreshSub = useMutation({
        mutationFn: () =>
            api.post<{ servers: Server[] }>('/api/subscription/refresh'),
        onSettled: () => {
            qc.invalidateQueries({ queryKey: ['subscription'] })
            qc.invalidateQueries({ queryKey: ['servers'] })
        },
    })

    const selectServer = useMutation({
        mutationFn: (id: number) => api.post('/api/servers/select', { id }),
        onMutate: id => {
            qc.setQueryData<Server[]>(['servers'], old =>
                old?.map(s => ({ ...s, active: s.id === id })),
            )
            qc.setQueryData<Status>(['status'], old =>
                old ? { ...old, restarting: true } : old,
            )
        },
        onSettled: () => {
            qc.invalidateQueries({ queryKey: ['servers'] })
            qc.invalidateQueries({ queryKey: ['status'] })
        },
    })

    const restart = useMutation({
        mutationFn: () => api.post('/api/xkeen/restart'),
        onMutate: () => {
            qc.setQueryData<Status>(['status'], old =>
                old ? { ...old, restarting: true } : old,
            )
        },
    })

    const start = useMutation({
        mutationFn: () => api.post('/api/xkeen/start'),
        onSettled: () => qc.invalidateQueries({ queryKey: ['status'] }),
    })

    const stop = useMutation({
        mutationFn: () => api.post('/api/xkeen/stop'),
        onSettled: () => qc.invalidateQueries({ queryKey: ['status'] }),
    })

    const poolAction = useMutation({
        mutationFn: (action: 'enable' | 'disable' | 'sync') =>
            api.post(`/api/pool/${action}`),
        onMutate: () => {
            qc.setQueryData<Status>(['status'], old =>
                old ? { ...old, restarting: true } : old,
            )
        },
        onSettled: () => {
            qc.invalidateQueries({ queryKey: ['pool'] })
            qc.invalidateQueries({ queryKey: ['status'] })
        },
    })

    const syncMihomo = useMutation({
        mutationFn: () => api.post('/api/mihomo/sync'),
        onMutate: () => {
            qc.setQueryData<Status>(['status'], old =>
                old ? { ...old, restarting: true } : old,
            )
        },
        onSettled: () => qc.invalidateQueries({ queryKey: ['status'] }),
    })

    const setCountry = useMutation({
        mutationFn: ({ id, country }: { id: number; country: string }) =>
            api.post('/api/servers/country', { id, country }),
        onSettled: () => qc.invalidateQueries({ queryKey: ['servers'] }),
    })

    const toggleWatchdog = useMutation({
        mutationFn: (active: boolean) =>
            api.post('/api/watchdog/toggle', { active }),
        onMutate: active => {
            qc.setQueryData<Status>(['status'], old =>
                old ? { ...old, watchdog_active: active } : old,
            )
        },
        onSettled: () => qc.invalidateQueries({ queryKey: ['status'] }),
    })

    const s = status.data

    return (
        <div className='min-h-screen'>
            {/* Баннер рестарта */}
            {restarting && (
                <div className='bg-amber-500/10 border-b border-amber-500/30 px-4 py-2.5 flex items-center justify-center gap-2 text-sm text-amber-400'>
                    <IconLoader2 className='size-4 animate-spin' />
                    XKeen перезапускается...
                </div>
            )}
            {/* Шапка */}
            <header className='bg-card border-b sticky top-0 z-10'>
                <div className='max-w-6xl mx-auto px-4 py-3 flex items-center justify-between'>
                    <div>
                        <h1 className='text-lg font-bold flex items-center gap-2'>
                            <img src='/favicon.svg' alt='' className='size-6' />
                            XKeen Panel
                        </h1>
                        <div className='flex items-center gap-3 mt-0.5'>
                            <StatusBadge
                                connected={s?.connected ?? false}
                                xrayRunning={s?.xray_running ?? false}
                                latency={s?.latency_ms ?? -1}
                            />
                            {s?.current_server && (
                                <span className='text-xs text-muted-foreground'>
                                    {s.current_server}
                                    {s.protocol && ` (${s.protocol})`}
                                </span>
                            )}
                        </div>
                    </div>
                    <Button variant='outline' size='sm' onClick={logout}>
                        <IconLogout className='size-4' />
                        Выйти
                    </Button>
                </div>
            </header>
            {/* Контент */}
            <main className='max-w-6xl mx-auto px-4 py-4'>
                <div className='grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4'>
                    <div className='space-y-4'>
                        <SubscriptionForm
                            subscription={subscription.data ?? null}
                            onUpdate={url => updateSub.mutate(url)}
                            onRefresh={() => refreshSub.mutate()}
                            loading={
                                updateSub.isPending || refreshSub.isPending
                            }
                        />
                        <Controls
                            watchdogActive={s?.watchdog_active ?? false}
                            coreRunning={s?.xray_running ?? false}
                            onRestart={() => restart.mutate()}
                            onStart={() => start.mutate()}
                            onStop={() => stop.mutate()}
                            onSelfTest={() =>
                                api.post<SelfTestResult>('/api/xkeen/selftest')
                            }
                            onToggleWatchdog={active =>
                                toggleWatchdog.mutate(active)
                            }
                            loading={
                                restart.isPending ||
                                start.isPending ||
                                stop.isPending ||
                                restarting
                            }
                        />
                        <XKeenCard
                            status={s}
                            pool={pool.data}
                            onEnablePool={() => poolAction.mutate('enable')}
                            onDisablePool={() => poolAction.mutate('disable')}
                            onSyncPool={() => poolAction.mutate('sync')}
                            onSyncMihomo={() => syncMihomo.mutate()}
                            loading={
                                poolAction.isPending ||
                                syncMihomo.isPending ||
                                restarting
                            }
                        />
                        <SettingsCard />
                        <PasskeyCard />
                        <LogViewer
                            logs={logs.data ?? []}
                            onRefresh={() =>
                                qc.invalidateQueries({ queryKey: ['logs'] })
                            }
                            loading={logs.isFetching}
                        />
                    </div>
                    <div>
                        <ServerList
                            servers={servers.data ?? []}
                            onSelect={id => selectServer.mutate(id)}
                            onSetCountry={(id, country) =>
                                setCountry.mutate({ id, country })
                            }
                            onCheckAll={checkLatency}
                            loading={
                                selectServer.isPending ||
                                checkingLatency ||
                                restarting
                            }
                        />
                    </div>
                </div>
            </main>
        </div>
    )
}
