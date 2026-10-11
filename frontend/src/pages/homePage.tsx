import { type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import {
    IconDeviceDesktop,
    IconDownload,
    IconRefresh,
    IconWorld,
} from '@tabler/icons-react'
import { IconTile, Switch } from '@/components/ios'
import { Logo } from '@/components/logo'
import {
    useDevices,
    useLogs,
    usePool,
    useServers,
    useStatus,
    useSubscription,
    useUpdate,
} from '@/hooks/usePanelData'
import { useServerActions } from '@/hooks/useServerActions'
import {
    countryName,
    panelEvents,
    serverCountry,
    statusInfo,
    timeAgo,
    toneDot,
    toneFill,
    toneText,
} from '@/lib/format'
import { cn } from '@/lib/utils'

export function HomePage() {
    const status = useStatus().data
    const servers = useServers().data ?? []
    const pool = usePool().data
    const subscription = useSubscription().data
    const devices = useDevices().data
    const update = useUpdate().data
    const logs = useLogs().data ?? []
    const { auto, pin } = useServerActions()

    const info = statusInfo(status)
    const current = servers.find(s => s.active)
    const code = serverCountry(current)
    const isPool = pool?.mode === 'pool'
    const isAuto = isPool && !pool?.pin_manual
    const events = panelEvents(logs, 5)

    const inVpn = devices?.devices.filter(d => d.vpn).length ?? 0
    const online = devices?.devices.filter(d => d.active).length ?? 0

    const toggleAuto = (next: boolean) => {
        if (next) auto.mutate()
        else if (pool?.current_tag) pin.mutate(pool.current_tag)
    }

    return (
        <>
            <header className='flex items-center justify-between gap-3 px-1 lg:hidden'>
                <div className='flex items-center gap-2.5'>
                    <Logo size={32} />
                    <h1 className='text-[34px] font-bold tracking-[0.37px]'>
                        XKeen
                    </h1>
                </div>
                <StatusPill label={info.label} tone={info.tone} />
            </header>
            <h1 className='hidden px-1 text-[34px] font-bold tracking-[0.37px] lg:block'>
                Главная
            </h1>

            <div className='grid gap-4 lg:grid-cols-2 lg:items-start'>
                <div className='flex flex-col gap-4'>
                    <section className='flex flex-col gap-3.5 rounded-card bg-card p-[18px]'>
                        <div className='flex items-center gap-3.5'>
                            <div className='flex size-14 shrink-0 items-center justify-center rounded-2xl bg-white/8 text-[22px] font-bold tracking-[-0.5px]'>
                                {code || '—'}
                            </div>
                            <div className='min-w-0'>
                                <div className='truncate text-[22px] font-bold tracking-[-0.26px]'>
                                    {countryName(code) ||
                                        status?.current_server ||
                                        'Сервер не выбран'}
                                </div>
                                <div className='truncate text-[15px] tracking-[-0.24px] text-muted-foreground'>
                                    {current?.name ?? status?.current_server}
                                    {status &&
                                        status.latency_ms > 0 &&
                                        ` · ${status.latency_ms} мс`}
                                </div>
                            </div>
                        </div>
                        {isPool && (
                            <div className='flex min-h-11 items-center gap-3 border-t border-white/10 pt-2.5'>
                                <div className='min-w-0 flex-1'>
                                    <div className='text-[17px] tracking-[-0.43px]'>
                                        Выбирать сервер сам
                                    </div>
                                    <div className='mt-0.5 text-[13px] text-muted-foreground'>
                                        {pool?.pin_note ||
                                            (isAuto
                                                ? `Самый быстрый из ${pool?.pool_tags?.length ?? 0} серверов пула`
                                                : 'Сервер выбран вручную')}
                                    </div>
                                </div>
                                <Switch
                                    checked={isAuto}
                                    onChange={toggleAuto}
                                    label='Выбирать сервер сам'
                                    disabled={
                                        auto.isPending ||
                                        pin.isPending ||
                                        !pool?.api_available
                                    }
                                />
                            </div>
                        )}
                    </section>

                    <div className='grid grid-cols-2 gap-3.5'>
                        <Widget
                            to='/servers'
                            icon={
                                <IconTile color='#0a84ff'>
                                    <IconWorld
                                        className='size-[17px]'
                                        stroke={2.2}
                                    />
                                </IconTile>
                            }
                            value={subscription?.server_count ?? 0}
                            caption={
                                isPool
                                    ? `серверов · ${pool?.pool_tags?.length ?? 0} в пуле`
                                    : 'серверов'
                            }
                        />
                        <Widget
                            to='/devices'
                            icon={
                                <IconTile color='#30d158'>
                                    <IconDeviceDesktop
                                        className='size-[17px]'
                                        stroke={2.2}
                                    />
                                </IconTile>
                            }
                            value={
                                <>
                                    {inVpn}{' '}
                                    <span className='text-[15px] font-normal text-muted-foreground'>
                                        из {devices?.devices.length ?? 0}
                                    </span>
                                </>
                            }
                            caption={
                                devices
                                    ? `через VPN · ${online} в сети`
                                    : 'устройств'
                            }
                        />
                        <Widget
                            to='/servers'
                            icon={
                                <IconTile color='#ff9f0a'>
                                    <IconRefresh
                                        className='size-[17px]'
                                        stroke={2.2}
                                    />
                                </IconTile>
                            }
                            value={timeAgo(subscription?.last_updated) || '—'}
                            caption='с обновления подписки'
                        />
                        <Widget
                            to='/settings/update'
                            icon={
                                <IconTile color='#6366f1'>
                                    <IconDownload
                                        className='size-[17px]'
                                        stroke={2.2}
                                    />
                                </IconTile>
                            }
                            value={status?.panel_version ?? '—'}
                            caption={
                                update?.available && update.latest
                                    ? `доступна ${update.latest.tag}`
                                    : update?.auto
                                      ? `обновится в ${String(update.auto_hour).padStart(2, '0')}:00`
                                      : 'версия панели'
                            }
                        />
                    </div>
                </div>

                <section className='flex flex-col gap-1.5'>
                    <h2 className='px-4 text-[13px] uppercase tracking-[-0.08px] text-muted-foreground'>
                        События
                    </h2>
                    <div className='divide-y divide-white/10 rounded-card bg-card px-4'>
                        {events.length === 0 && (
                            <p className='py-3 text-[15px] text-muted-foreground'>
                                Пока ничего не происходило
                            </p>
                        )}
                        {events.map((e, i) => (
                            <div
                                key={i}
                                className='flex gap-3 py-3 text-[15px] leading-snug tracking-[-0.24px]'
                            >
                                <span className='shrink-0 tabular-nums text-muted-foreground'>
                                    {e.time}
                                </span>
                                <span className='min-w-0'>{e.text}</span>
                            </div>
                        ))}
                    </div>
                </section>
            </div>
        </>
    )
}

function StatusPill({
    label,
    tone,
}: {
    label: string
    tone: keyof typeof toneText
}) {
    return (
        <div
            className={cn(
                'flex min-h-9 items-center gap-1.5 rounded-full px-3.5 text-[15px] font-semibold tracking-[-0.24px]',
                toneFill[tone],
                toneText[tone],
            )}
        >
            <span className={cn('size-2 rounded-full', toneDot[tone])} />
            {label}
        </div>
    )
}

function Widget({
    to,
    icon,
    value,
    caption,
}: {
    to: string
    icon: ReactNode
    value: ReactNode
    caption: string
}) {
    return (
        <Link
            to={to}
            className='flex min-h-28 flex-col gap-2 rounded-card bg-card p-4'
        >
            {icon}
            <span className='truncate text-[22px] font-bold tracking-[-0.26px]'>
                {value}
            </span>
            <span className='text-[13px] leading-snug text-muted-foreground'>
                {caption}
            </span>
        </Link>
    )
}
