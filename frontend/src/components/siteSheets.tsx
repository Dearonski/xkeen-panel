import { useEffect, useMemo, useState } from 'react'
import {
    IconCheck,
    IconLock,
    IconQuestionMark,
    IconSearch,
    IconX,
} from '@tabler/icons-react'
import { Group, Row, Segmented } from '@/components/ios'
import { Sheet } from '@/components/sheet'
import { useSiteSources } from '@/hooks/usePanelData'
import { useSiteActions } from '@/hooks/useSiteActions'
import { countryName, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Site, SiteLock, SiteNode } from '@/types'

type SourceMode = 'pool' | 'countries' | 'nodes'

function sourceMode(site: Site): SourceMode {
    if (site.source?.nodes?.length) return 'nodes'
    if (site.source?.countries?.length) return 'countries'
    return 'pool'
}

function sameSet(a: string[], b: string[]) {
    return a.length === b.length && a.every(x => b.includes(x))
}

export function siteSummary(site: Site) {
    const lock = site.lock
    if (lock?.blocked)
        return `Заблокирован: ${lock.reason || 'выход не подтверждён'}`
    if (lock) return `Только ${lock.exit_ip} · ${countryLabel(lock.country)}`
    if (site.checking) return 'Проверяю серверы…'
    if (!site.checked_at) return 'Ещё не проверен'
    const ago = timeAgo(site.checked_at)
    const when = ago ? ` · ${ago === 'только что' ? ago : `${ago} назад`}` : ''
    if (site.passing === 0) return `Не открывается ни через один сервер${when}`
    return `${site.passing} из ${site.nodes.length} серверов${when}`
}

export function AddSiteSheet({
    open,
    onClose,
    onCreated,
}: {
    open: boolean
    onClose: () => void
    onCreated: (id: number) => void
}) {
    const { create } = useSiteActions()
    const [domains, setDomains] = useState('')
    const [ips, setIPs] = useState('')
    const [checkURL, setCheckURL] = useState('')
    const [error, setError] = useState('')

    useEffect(() => {
        if (!open) return
        setDomains('')
        setIPs('')
        setCheckURL('')
        setError('')
    }, [open])

    const submit = async () => {
        if (!domains.trim() || create.isPending) return
        setError('')
        try {
            const res = await create.mutateAsync({
                domains,
                ips,
                check_url: checkURL,
            })
            onCreated(res.id)
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось добавить')
        }
    }

    return (
        <Sheet
            open={open}
            onClose={onClose}
            title='Новый сайт'
            done={{
                label: create.isPending ? 'Добавляю…' : 'Добавить',
                onClick: submit,
                disabled: !domains.trim() || create.isPending,
            }}
        >
            <Group
                title='Домены'
                footer={
                    error ? (
                        <span className='text-destructive'>{error}</span>
                    ) : (
                        'Через пробел или запятую. Поддомены входят сами: example.com покрывает и www.example.com.'
                    )
                }
            >
                <label htmlFor='site-domains' className='sr-only'>
                    Домены
                </label>
                <textarea
                    id='site-domains'
                    value={domains}
                    onChange={e => setDomains(e.target.value)}
                    placeholder='example.com'
                    rows={2}
                    autoFocus
                    autoCapitalize='none'
                    autoCorrect='off'
                    spellCheck={false}
                    className='block w-full resize-none bg-transparent py-3 text-[17px] tracking-[-0.43px] outline-none placeholder:text-label-tertiary'
                />
            </Group>

            <Group
                title='IP-адреса'
                footer='Необязательно. Подсети через пробел — ловят соединения, в которых домен не распознан.'
            >
                <label htmlFor='site-ips' className='sr-only'>
                    IP-адреса
                </label>
                <textarea
                    id='site-ips'
                    value={ips}
                    onChange={e => setIPs(e.target.value)}
                    placeholder='203.0.113.0/24'
                    rows={1}
                    autoCapitalize='none'
                    autoCorrect='off'
                    spellCheck={false}
                    className='block w-full resize-none bg-transparent py-3 text-[17px] tracking-[-0.43px] outline-none placeholder:text-label-tertiary'
                />
            </Group>

            <Group
                title='Адрес проверки'
                footer='Страница, которая не открывается через плохие серверы, — например, страница входа. Панель откроет её через каждый сервер пула и пустит сайт только через те, где она ответила без ошибки. Пусто — главная страница первого домена.'
            >
                <label htmlFor='site-check' className='sr-only'>
                    Адрес проверки
                </label>
                <input
                    id='site-check'
                    type='url'
                    inputMode='url'
                    autoComplete='off'
                    autoCapitalize='none'
                    value={checkURL}
                    onChange={e => setCheckURL(e.target.value)}
                    placeholder={`https://${firstDomain(domains) || 'example.com'}/`}
                    className='min-h-[52px] w-full bg-transparent text-[17px] tracking-[-0.43px] outline-none placeholder:text-label-tertiary'
                />
            </Group>
        </Sheet>
    )
}

function firstDomain(input: string) {
    return input
        .trim()
        .split(/[\s,;]+/)[0]
        ?.replace(/^https?:\/\//, '')
        .replace(/[/?#].*$/, '')
}

export function SiteSheet({
    site,
    onClose,
}: {
    site?: Site
    onClose: () => void
}) {
    const { check, remove, lock } = useSiteActions()
    const [confirming, setConfirming] = useState(false)
    const [error, setError] = useState('')
    const [pending, setPending] = useState<string | null>(null)
    const [lockError, setLockError] = useState('')

    useEffect(() => {
        setConfirming(false)
        setError('')
        setPending(null)
        setLockError('')
    }, [site?.id])

    const pick = async (node: string) => {
        if (!site || lock.isPending) return
        setLockError('')
        if (pending !== node) {
            setPending(node)
            return
        }
        try {
            await lock.mutateAsync({ id: site.id, node })
            setPending(null)
        } catch (e) {
            setLockError(
                e instanceof Error ? e.message : 'Не удалось закрепить',
            )
        }
    }

    const nodesFooter = !site ? undefined : lockError ? (
        <span className='text-destructive'>{lockError}</span>
    ) : pending ? (
        `Панель ещё раз узнает выходной IP через этот сервер и пустит сайт только через него.${site.lock ? '' : ' Ядро перезапустится.'}`
    ) : site.lock ? (
        'Нажмите на другой сервер, чтобы перенести закрепление.'
    ) : site.nodes.some(n => n.state === 'ok' && n.exit_ip) ? (
        `Нажмите на сервер, чтобы сайт ходил только через него. Если его IP или страна изменятся, панель заблокирует сайт, а не переключит на другой сервер.${site.ips.length === 0 ? ' Без IP-адресов сайт не защищён, пока перезапускается ядро.' : ''}`
    ) : undefined

    const del = async () => {
        if (!site) return
        if (!confirming) {
            setConfirming(true)
            return
        }
        try {
            await remove.mutateAsync(site.id)
            onClose()
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось удалить')
        }
    }

    return (
        <Sheet open={!!site} onClose={onClose} title={site?.domains[0] ?? ''}>
            {site && (
                <>
                    <Group
                        footer={
                            !site.lock && site.fallback
                                ? 'Под выбранный источник не подошёл ни один сервер, поэтому сайт идёт через весь пул.'
                                : !site.lock &&
                                    site.passing === 0 &&
                                    site.checked_at
                                  ? sourceMode(site) === 'pool'
                                      ? 'Пока ни один сервер не прошёл проверку, сайт идёт через весь пул, как раньше.'
                                      : 'Пока ни один сервер не прошёл проверку, сайт идёт через все выбранные серверы.'
                                  : `Проверяется ${site.check_url}. Панель перепроверяет сайт, когда меняются его серверы, и раз в 12 часов.`
                        }
                    >
                        <Row
                            label={site.domains.join(', ')}
                            hint={siteSummary(site)}
                        />
                        <Row
                            label={
                                site.checking ? 'Проверяю…' : 'Проверить снова'
                            }
                            tone='tint'
                            disabled={site.checking || check.isPending}
                            onClick={() => check.mutate(site.id)}
                        />
                    </Group>

                    {site.lock && <LockGroup site={site} />}

                    <SourceGroup site={site} />

                    {site.nodes.length > 0 && (
                        <Group title={nodesTitle(site)} footer={nodesFooter}>
                            {site.nodes.map(n => (
                                <NodeResult
                                    key={n.key}
                                    node={n}
                                    lockedAs={
                                        site.lock?.node === n.key
                                            ? site.lock
                                            : undefined
                                    }
                                    pending={pending === n.key}
                                    busy={lock.isPending}
                                    onPick={
                                        n.state === 'ok' &&
                                        n.exit_ip &&
                                        site.lock?.node !== n.key
                                            ? () => pick(n.key)
                                            : undefined
                                    }
                                />
                            ))}
                        </Group>
                    )}

                    <Group
                        footer={
                            error ? (
                                <span className='text-destructive'>
                                    {error}
                                </span>
                            ) : confirming ? (
                                'Сайт снова пойдёт через весь пул. Ядро перезапустится.'
                            ) : undefined
                        }
                    >
                        <Row
                            label={
                                confirming ? 'Точно удалить?' : 'Удалить сайт'
                            }
                            tone='destructive'
                            disabled={remove.isPending}
                            onClick={del}
                        />
                    </Group>
                </>
            )}
        </Sheet>
    )
}

function nodesTitle(site: Site) {
    if (site.fallback) return 'Серверы пула'
    switch (sourceMode(site)) {
        case 'countries':
            return 'Серверы выбранных стран'
        case 'nodes':
            return 'Выбранные серверы'
        default:
            return 'Серверы пула'
    }
}

function SourceGroup({ site }: { site: Site }) {
    const { setSource } = useSiteActions()
    const [mode, setMode] = useState<SourceMode>(sourceMode(site))
    const [countries, setCountries] = useState<string[]>(
        site.source?.countries ?? [],
    )
    const [nodes, setNodes] = useState<string[]>(site.source?.nodes ?? [])
    const [query, setQuery] = useState('')
    const [error, setError] = useState('')
    const options = useSiteSources(mode !== 'pool').data

    useEffect(() => {
        setMode(sourceMode(site))
        setCountries(site.source?.countries ?? [])
        setNodes(site.source?.nodes ?? [])
        setQuery('')
        setError('')
        // Only a different site resets the draft; a recheck must not wipe it
    }, [site.id])

    const servers = useMemo(() => {
        const q = query.trim().toLowerCase()
        return (options?.servers ?? [])
            .filter(
                s =>
                    !q ||
                    s.name.toLowerCase().includes(q) ||
                    countryName(s.exit_country || s.country)
                        .toLowerCase()
                        .includes(q),
            )
            .sort(
                (a, b) =>
                    Number(nodes.includes(b.key)) -
                        Number(nodes.includes(a.key)) ||
                    a.name.localeCompare(b.name, 'ru'),
            )
        // Selection order is frozen while the list is open, so rows do not jump under the finger
    }, [options, query])

    const current = sourceMode(site)
    const dirty =
        mode !== current ||
        (mode === 'countries' &&
            !sameSet(countries, site.source?.countries ?? [])) ||
        (mode === 'nodes' && !sameSet(nodes, site.source?.nodes ?? []))
    const valid =
        mode === 'pool' ||
        (mode === 'countries' && countries.length > 0) ||
        (mode === 'nodes' && nodes.length > 0)

    const toggle = (list: string[], value: string) =>
        list.includes(value) ? list.filter(v => v !== value) : [...list, value]

    const apply = async () => {
        setError('')
        try {
            await setSource.mutateAsync({
                id: site.id,
                source: {
                    countries: mode === 'countries' ? countries : [],
                    nodes: mode === 'nodes' ? nodes : [],
                },
            })
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось сохранить')
        }
    }

    const footer = error ? (
        <span className='text-destructive'>{error}</span>
    ) : mode === 'pool' ? (
        'Сайт проверяется и ходит через серверы пула.'
    ) : mode === 'countries' ? (
        'Страна определяется по реальному выходу сервера, а не по названию. Сайт пойдёт только через серверы подписки с выходом из выбранных стран, которые его открывают. Пул не меняется.'
    ) : (
        'Сайт пойдёт только через отмеченные серверы подписки, которые его открывают. Пул не меняется.'
    )

    return (
        <Group title='Через какие серверы' footer={footer}>
            <div className='py-3'>
                <Segmented
                    label='Источник серверов'
                    value={mode}
                    onChange={setMode}
                    options={[
                        { value: 'pool', label: 'Пул' },
                        { value: 'countries', label: 'Страны' },
                        { value: 'nodes', label: 'Серверы' },
                    ]}
                />
            </div>

            {mode === 'countries' && (
                <div className='py-3'>
                    {options && options.countries.length === 0 ? (
                        <p className='text-[15px] text-muted-foreground'>
                            Страны выходов ещё не известны: панель проверяет
                            серверы подписки, это займёт несколько минут.
                        </p>
                    ) : (
                        <div
                            role='group'
                            aria-label='Страны'
                            className='flex flex-wrap gap-2'
                        >
                            {(options?.countries ?? []).map(c => (
                                <button
                                    key={c.code}
                                    type='button'
                                    aria-pressed={countries.includes(c.code)}
                                    onClick={() =>
                                        setCountries(toggle(countries, c.code))
                                    }
                                    className={cn(
                                        'min-h-9 cursor-pointer select-none rounded-full px-3.5 text-[15px] tracking-[-0.24px] transition-colors',
                                        countries.includes(c.code)
                                            ? 'bg-white font-semibold text-black'
                                            : 'border border-white/12 bg-white/6 font-medium hover:bg-white/10',
                                    )}
                                >
                                    {countryLabel(c.code)} · {c.servers}
                                </button>
                            ))}
                        </div>
                    )}
                </div>
            )}

            {mode === 'nodes' && (
                <div className='py-3'>
                    <label className='flex min-h-10 items-center gap-1.5 rounded-xl bg-fill px-2.5'>
                        <IconSearch
                            className='size-4 text-muted-foreground'
                            stroke={2.2}
                        />
                        <span className='sr-only'>Поиск сервера</span>
                        <input
                            type='search'
                            value={query}
                            onChange={e => setQuery(e.target.value)}
                            placeholder='Поиск'
                            className='min-w-0 flex-1 bg-transparent text-[17px] tracking-[-0.43px] outline-none placeholder:text-muted-foreground'
                        />
                    </label>
                    <div className='mt-2 max-h-80 divide-y divide-white/10 overflow-y-auto'>
                        {servers.map(s => {
                            const picked = nodes.includes(s.key)
                            const exit = s.exit_country || s.country
                            return (
                                <button
                                    key={s.key}
                                    type='button'
                                    role='checkbox'
                                    aria-checked={picked}
                                    onClick={() =>
                                        setNodes(toggle(nodes, s.key))
                                    }
                                    className='flex min-h-11 w-full cursor-pointer items-center gap-3 py-1.5 text-left'
                                >
                                    <span className='flex w-6 justify-center'>
                                        {picked && (
                                            <IconCheck
                                                className='size-[18px] text-tint'
                                                stroke={2.8}
                                            />
                                        )}
                                    </span>
                                    <span className='min-w-0 flex-1'>
                                        <span className='block break-words text-[15px]'>
                                            {s.name}
                                        </span>
                                        {exit && (
                                            <span className='block text-[13px] text-muted-foreground'>
                                                {s.exit_country
                                                    ? `выход ${countryLabel(s.exit_country)}`
                                                    : countryLabel(s.country)}
                                            </span>
                                        )}
                                    </span>
                                </button>
                            )
                        })}
                        {options && servers.length === 0 && (
                            <p className='py-3 text-[15px] text-muted-foreground'>
                                Ничего не нашлось
                            </p>
                        )}
                    </div>
                </div>
            )}

            {dirty && (
                <Row
                    label={setSource.isPending ? 'Сохраняю…' : 'Применить'}
                    tone='tint'
                    disabled={!valid || setSource.isPending}
                    onClick={apply}
                />
            )}
        </Group>
    )
}

function countryLabel(code?: string) {
    return countryName(code) || code || '?'
}

function exitLabel(node: SiteNode) {
    if (!node.exit_ip) return ''
    return `${node.exit_ip} · ${countryLabel(node.exit_country)}`
}

function LockGroup({ site }: { site: Site }) {
    const { lock, unlock } = useSiteActions()
    const [error, setError] = useState('')

    useEffect(() => setError(''), [site.id])

    const current = site.lock
    if (!current) return null
    const busy = lock.isPending || unlock.isPending

    const run = async (action: () => Promise<unknown>) => {
        setError('')
        try {
            await action()
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Не получилось')
        }
    }

    const verified = timeAgo(current.verified_at)
    const status = current.blocked
        ? `Заблокирован: ${current.reason || 'выход не подтверждён'}`
        : verified
          ? `${current.name} · проверено ${verified === 'только что' ? verified : `${verified} назад`}${current.live ? '' : ', только сервер'}`
          : `${current.name} · ещё не проверено`

    return (
        <Group
            title='Постоянный IP'
            footer={
                error ? (
                    <span className='text-destructive'>{error}</span>
                ) : current.blocked ? (
                    'Трафик сайта никуда не идёт. Панель проверяет сервер каждую минуту и вернёт трафик, когда выход снова совпадёт.'
                ) : (
                    'Сайт ходит только через этот сервер. При смене IP или страны панель сразу заблокирует сайт.'
                )
            }
        >
            <Row
                icon={<LockIcon blocked={current.blocked} />}
                label={`${current.exit_ip} · ${countryLabel(current.country)}`}
                hint={
                    <span className={cn(current.blocked && 'text-destructive')}>
                        {status}
                    </span>
                }
            />
            {current.blocked && (
                <Row
                    label={
                        lock.isPending
                            ? 'Проверяю…'
                            : 'Принять текущий IP этого сервера'
                    }
                    tone='tint'
                    disabled={busy}
                    onClick={() =>
                        run(() =>
                            lock.mutateAsync({
                                id: site.id,
                                node: current.node,
                            }),
                        )
                    }
                />
            )}
            <Row
                label='Снять закрепление'
                tone='destructive'
                disabled={busy}
                onClick={() => run(() => unlock.mutateAsync(site.id))}
            />
        </Group>
    )
}

function LockIcon({ blocked }: { blocked: boolean }) {
    return (
        <span className='flex w-[30px] justify-center'>
            <IconLock
                className={cn(
                    'size-[18px]',
                    blocked ? 'text-destructive' : 'text-success',
                )}
                stroke={2.4}
            />
        </span>
    )
}

function NodeResult({
    node,
    lockedAs,
    pending,
    busy,
    onPick,
}: {
    node: SiteNode
    lockedAs?: SiteLock
    pending: boolean
    busy: boolean
    onPick?: () => void
}) {
    const icon = lockedAs ? (
        <LockIcon blocked={lockedAs.blocked} />
    ) : node.state === 'ok' ? (
        <IconCheck className='size-[18px] text-success' stroke={2.6} />
    ) : node.state === 'fail' ? (
        <IconX className='size-[18px] text-destructive' stroke={2.6} />
    ) : (
        <IconQuestionMark
            className='size-[18px] text-muted-foreground'
            stroke={2.4}
        />
    )

    const detail = pending
        ? busy
            ? 'Проверяю…'
            : 'Закрепить?'
        : node.state === 'unknown'
          ? 'не проверен'
          : node.error
            ? node.error
            : node.state === 'ok'
              ? `${node.ms} мс`
              : `ответ ${node.status}`

    return (
        <Row
            icon={
                lockedAs ? (
                    icon
                ) : (
                    <span className='flex w-[30px] justify-center'>{icon}</span>
                )
            }
            label={<span className='block break-words'>{node.name}</span>}
            onClick={onPick}
            disabled={busy && !pending}
            hint={
                [
                    countryName(node.country),
                    node.exit_ip && `выход ${exitLabel(node)}`,
                ]
                    .filter(Boolean)
                    .join(' · ') || undefined
            }
            trailing={
                <span
                    className={cn(
                        'max-w-[40%] shrink-0 truncate text-right text-[15px] tabular-nums',
                        pending
                            ? 'text-tint'
                            : node.state === 'fail'
                              ? 'text-destructive'
                              : 'text-muted-foreground',
                    )}
                >
                    {detail}
                </span>
            }
        />
    )
}
