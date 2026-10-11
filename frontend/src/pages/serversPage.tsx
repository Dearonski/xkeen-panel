import { useEffect, useMemo, useState } from 'react'
import {
    IconActivityHeartbeat,
    IconCheck,
    IconChevronRight,
    IconLink,
    IconPencil,
    IconRefresh,
    IconSearch,
} from '@tabler/icons-react'
import { CircleButton, Group, IconTile, PageTitle, Row } from '@/components/ios'
import { Sheet } from '@/components/sheet'
import { useServers, useSubscription } from '@/hooks/usePanelData'
import { useServerActions } from '@/hooks/useServerActions'
import { useStreamLatency } from '@/hooks/useStreamLatency'
import {
    countryName,
    latencyTone,
    serverCountry,
    timeAgo,
    toneText,
} from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Server } from '@/types'

const OTHER = '—'

// One list on a phone, separate cards side by side on a desktop
const TILE = 'lg:rounded-card lg:bg-card lg:px-4'

type CountryGroup = { code: string; list: Server[] }

export function ServersPage() {
    const servers = useServers().data ?? []
    const subscription = useSubscription().data
    const { select, refreshSubscription, setSubscription, setCountry } =
        useServerActions()
    const { check, checking } = useStreamLatency()
    const [query, setQuery] = useState('')
    const [country, setCountryFilter] = useState('all')
    const [sheetOpen, setSheetOpen] = useState(false)

    const countries = useMemo(() => {
        const counts = new Map<string, number>()
        servers.forEach(s => {
            const c = serverCountry(s) || OTHER
            counts.set(c, (counts.get(c) ?? 0) + 1)
        })
        return [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c)
    }, [servers])

    const groups = useMemo(() => {
        const q = query.trim().toLowerCase()
        const byCountry = new Map<string, Server[]>()
        servers.forEach(s => {
            const c = serverCountry(s) || OTHER
            if (country !== 'all' && c !== country) return
            if (
                q &&
                !s.name.toLowerCase().includes(q) &&
                !countryName(c).toLowerCase().includes(q)
            )
                return
            byCountry.set(c, [...(byCountry.get(c) ?? []), s])
        })
        const hasActive = (g: CountryGroup) =>
            g.list.some(s => s.active) ? 0 : 1
        return [...byCountry.entries()]
            .map(([code, list]) => ({ code, list: list.sort(byLatency) }))
            .sort(
                (a, b) =>
                    hasActive(a) - hasActive(b) ||
                    (countryName(a.code) || 'я').localeCompare(
                        countryName(b.code) || 'я',
                        'ru',
                    ),
            )
    }, [servers, query, country])

    const updated = timeAgo(subscription?.last_updated)
    const rowProps = (s: Server) => ({
        server: s,
        busy: select.isPending,
        onSelect: () => select.mutate(s.id),
        onSetCountry: (c: string) =>
            setCountry.mutate({ id: s.id, country: c }),
    })

    return (
        <>
            <PageTitle
                title='Серверы'
                subtitle={
                    subscription?.url
                        ? `${subscription.server_count} серверов${updated ? ` · подписка обновлена ${updated} назад` : ''}`
                        : 'Добавьте ссылку на подписку'
                }
                action={
                    <div className='flex items-center gap-2'>
                        <SearchField
                            value={query}
                            onChange={setQuery}
                            className='hidden w-64 lg:flex'
                        />
                        {subscription?.url && (
                            <CircleButton
                                label='Обновить подписку'
                                onClick={() => refreshSubscription.mutate()}
                                disabled={refreshSubscription.isPending}
                            >
                                <IconRefresh
                                    className={cn(
                                        'size-5',
                                        refreshSubscription.isPending &&
                                            'animate-spin',
                                    )}
                                />
                            </CircleButton>
                        )}
                    </div>
                }
            />

            <div className='divide-y divide-white/10 rounded-card bg-card px-4 lg:grid lg:grid-cols-2 lg:gap-4 lg:divide-y-0 lg:bg-transparent lg:px-0'>
                <div className={TILE}>
                    <Row
                        icon={
                            <IconTile color='#6366f1'>
                                <IconLink
                                    className='size-[17px]'
                                    stroke={2.2}
                                />
                            </IconTile>
                        }
                        label={
                            subscription?.url ? 'Подписка' : 'Добавить подписку'
                        }
                        tone={subscription?.url ? undefined : 'tint'}
                        hint={
                            subscription?.url
                                ? `${hostOf(subscription.url)}${updated ? ` · обновлена ${updated} назад` : ''}`
                                : 'Ссылка от VPN-провайдера'
                        }
                        onClick={() => setSheetOpen(true)}
                        trailing={
                            <IconChevronRight
                                className='size-4 shrink-0 text-label-tertiary'
                                stroke={2.6}
                            />
                        }
                    />
                </div>
                {subscription?.url && (
                    <div className={TILE}>
                        <Row
                            icon={
                                <IconTile color='#30d158'>
                                    <IconActivityHeartbeat
                                        className='size-[17px]'
                                        stroke={2.2}
                                    />
                                </IconTile>
                            }
                            label={
                                checking ? 'Проверяю пинг…' : 'Проверить пинг'
                            }
                            hint='До самих серверов, без VPN'
                            onClick={check}
                            disabled={checking}
                        />
                    </div>
                )}
            </div>

            {servers.length > 0 && (
                <div className='flex flex-col gap-3'>
                    <SearchField
                        value={query}
                        onChange={setQuery}
                        className='lg:hidden'
                    />
                    <div
                        role='group'
                        aria-label='Страна'
                        className='-mx-4 flex gap-2 overflow-x-auto px-4 [mask-image:linear-gradient(to_right,transparent,black_16px,black_calc(100%-40px),transparent)] [scrollbar-width:none]'
                    >
                        {['all', ...countries].map(c => (
                            <button
                                key={c}
                                type='button'
                                aria-pressed={c === country}
                                onClick={() => setCountryFilter(c)}
                                className={cn(
                                    'min-h-9 shrink-0 cursor-pointer select-none rounded-full px-4 text-[15px] tracking-[-0.24px] transition-colors',
                                    c === country
                                        ? 'bg-white font-semibold text-black'
                                        : 'border border-white/12 bg-white/6 font-medium hover:bg-white/10',
                                )}
                            >
                                {c === 'all'
                                    ? 'Все'
                                    : countryName(c) || 'Без страны'}
                            </button>
                        ))}
                    </div>
                </div>
            )}

            <SubscriptionSheet
                open={sheetOpen}
                onClose={() => setSheetOpen(false)}
                url={subscription?.url}
                onSave={url => setSubscription.mutateAsync(url)}
            />

            <div className='flex flex-col gap-4 lg:block lg:columns-2 lg:gap-4'>
                {groups.map(g => (
                    <Group
                        key={g.code}
                        title={groupTitle(g)}
                        className='lg:mb-4 lg:break-inside-avoid'
                    >
                        {g.list.map(s => (
                            <ServerRow key={s.id} {...rowProps(s)} />
                        ))}
                    </Group>
                ))}
            </div>

            {servers.length > 0 && groups.length === 0 && (
                <p className='px-4 text-[15px] text-muted-foreground'>
                    Ничего не нашлось
                </p>
            )}
        </>
    )
}

function groupTitle(g: CountryGroup) {
    return `${countryName(g.code) || 'Страна не определена'} · ${g.list.length}`
}

function byLatency(a: Server, b: Server) {
    if (a.active !== b.active) return a.active ? -1 : 1
    const la = a.latency_ms < 0 ? Infinity : a.latency_ms || Infinity
    const lb = b.latency_ms < 0 ? Infinity : b.latency_ms || Infinity
    return la - lb
}

function statusOf(server: Server) {
    if (server.active) return { label: 'Выбран', className: 'text-tint' }
    if (server.latency_ms === -1)
        return { label: 'Не отвечает', className: 'text-destructive' }
    if (server.pool_tag)
        return { label: 'В пуле', className: 'text-muted-foreground' }
    return null
}

const hostOf = (url: string) => url.replace(/^https?:\/\/([^/]+).*$/, '$1')

// Subscription links carry the account token, so only the host and both ends are shown
function maskUrl(url: string) {
    const m = url.match(/^https?:\/\/([^/]+)(\/.*)?$/)
    if (!m) return '…'
    const path = m[2] ?? ''
    return path.length > 16
        ? `${m[1]}${path.slice(0, 9)}…${path.slice(-4)}`
        : `${m[1]}${path}`
}

function SearchField({
    value,
    onChange,
    className,
}: {
    value: string
    onChange: (value: string) => void
    className?: string
}) {
    return (
        <label
            className={cn(
                'flex min-h-10 items-center gap-1.5 rounded-xl bg-fill px-2.5',
                className,
            )}
        >
            <IconSearch className='size-4 text-muted-foreground' stroke={2.2} />
            <span className='sr-only'>Поиск</span>
            <input
                type='search'
                value={value}
                onChange={e => onChange(e.target.value)}
                placeholder='Поиск'
                className='min-w-0 flex-1 bg-transparent text-[17px] tracking-[-0.43px] outline-none placeholder:text-muted-foreground'
            />
        </label>
    )
}

function ServerRow({
    server,
    busy,
    onSelect,
    onSetCountry,
}: {
    server: Server
    busy: boolean
    onSelect: () => void
    onSetCountry: (country: string) => void
}) {
    const [editing, setEditing] = useState(false)
    const [value, setValue] = useState(serverCountry(server))
    const status = statusOf(server)
    const ping =
        server.latency_ms === 0
            ? ''
            : server.latency_ms < 0
              ? '—'
              : `${server.latency_ms} мс`

    if (editing) {
        return (
            <form
                className='flex min-h-12 items-center gap-2 py-2'
                onSubmit={e => {
                    e.preventDefault()
                    onSetCountry(value.trim().toUpperCase())
                    setEditing(false)
                }}
            >
                <span className='min-w-0 flex-1 truncate text-[17px]'>
                    {server.name}
                </span>
                <label className='sr-only' htmlFor={`country-${server.id}`}>
                    Код страны
                </label>
                <input
                    id={`country-${server.id}`}
                    value={value}
                    onChange={e =>
                        setValue(
                            e.target.value
                                .replace(/[^a-zA-Z]/g, '')
                                .slice(0, 2),
                        )
                    }
                    placeholder='NL'
                    autoFocus
                    className='h-9 w-14 rounded-lg bg-fill px-2 text-center text-[15px] uppercase outline-none'
                />
                <button
                    type='submit'
                    className='min-h-9 cursor-pointer px-2 text-[15px] font-semibold text-tint'
                >
                    Готово
                </button>
            </form>
        )
    }

    return (
        <div className='flex items-center gap-1'>
            <button
                type='button'
                onClick={onSelect}
                disabled={busy || server.active}
                className='flex min-h-[52px] min-w-0 flex-1 cursor-pointer items-center gap-3 py-2 text-left disabled:cursor-default'
            >
                <span className='min-w-0 flex-1'>
                    <span className='block break-words text-[17px] tracking-[-0.43px]'>
                        {server.name}
                    </span>
                    {status && (
                        <span
                            className={cn(
                                'mt-0.5 block text-[13px]',
                                status.className,
                            )}
                        >
                            {status.label}
                        </span>
                    )}
                </span>
                <span
                    className={cn(
                        'shrink-0 text-right text-[15px] tabular-nums',
                        toneText[latencyTone(server.latency_ms)],
                    )}
                >
                    {ping}
                </span>
                <span className='flex w-6 justify-center'>
                    {server.active && (
                        <IconCheck
                            className='size-[18px] text-tint'
                            stroke={2.8}
                            aria-label='Выбран'
                        />
                    )}
                </span>
            </button>
            <button
                type='button'
                aria-label={`Изменить страну для ${server.name}`}
                onClick={() => {
                    setValue(serverCountry(server))
                    setEditing(true)
                }}
                className='flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-full text-label-tertiary hover:text-muted-foreground'
            >
                <IconPencil className='size-4' />
            </button>
        </div>
    )
}

function SubscriptionSheet({
    open,
    onClose,
    url,
    onSave,
}: {
    open: boolean
    onClose: () => void
    url?: string
    onSave: (url: string) => Promise<unknown>
}) {
    const [value, setValue] = useState('')
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState('')
    const [copied, setCopied] = useState(false)

    useEffect(() => {
        if (!open) return
        setValue('')
        setError('')
    }, [open])

    const valid = /^https?:\/\/\S+$/.test(value.trim())

    const submit = async () => {
        if (!valid || saving) return
        setSaving(true)
        setError('')
        try {
            await onSave(value.trim())
            onClose()
        } catch (e) {
            setError(
                e instanceof Error
                    ? e.message
                    : 'Не удалось загрузить подписку',
            )
        } finally {
            setSaving(false)
        }
    }

    const paste = async () => {
        try {
            setValue((await navigator.clipboard.readText()).trim())
        } catch {
            setError(
                'Браузер не дал прочитать буфер обмена — вставьте ссылку вручную',
            )
        }
    }

    const copy = async () => {
        if (!url) return
        await navigator.clipboard?.writeText(url)
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
    }

    return (
        <Sheet
            open={open}
            onClose={onClose}
            title='Подписка'
            done={{
                label: saving ? 'Загружаю…' : 'Готово',
                onClick: submit,
                disabled: !valid || saving,
            }}
        >
            <Group
                title={url ? 'Новая ссылка' : 'Ссылка'}
                footer={
                    error ? (
                        <span className='text-destructive'>{error}</span>
                    ) : (
                        'Ссылку даёт VPN-провайдер — обычно в личном кабинете или в боте. Панель загрузит серверы и сама пересоберёт пул.'
                    )
                }
            >
                <form
                    className='flex min-h-[52px] items-center gap-2'
                    onSubmit={e => {
                        e.preventDefault()
                        submit()
                    }}
                >
                    <label htmlFor='subscription-url' className='sr-only'>
                        Ссылка на подписку
                    </label>
                    <input
                        id='subscription-url'
                        type='url'
                        inputMode='url'
                        autoComplete='off'
                        autoFocus
                        value={value}
                        onChange={e => setValue(e.target.value)}
                        placeholder='https://'
                        className='min-w-0 flex-1 bg-transparent text-[17px] tracking-[-0.43px] outline-none placeholder:text-label-tertiary'
                    />
                    {!value && (
                        <button
                            type='button'
                            onClick={paste}
                            className='min-h-9 shrink-0 cursor-pointer rounded-full bg-fill px-3 text-[15px] font-medium text-tint'
                        >
                            Вставить
                        </button>
                    )}
                </form>
            </Group>

            {url && (
                <Group title='Сейчас'>
                    <Row
                        label={
                            <span className='block truncate font-mono text-[15px]'>
                                {maskUrl(url)}
                            </span>
                        }
                        trailing={
                            <button
                                type='button'
                                onClick={copy}
                                className='min-h-9 shrink-0 cursor-pointer px-1 text-[15px] text-tint'
                            >
                                {copied ? 'Скопировано' : 'Скопировать'}
                            </button>
                        }
                    />
                </Group>
            )}
        </Sheet>
    )
}
