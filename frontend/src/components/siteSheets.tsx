import { useEffect, useState } from 'react'
import {
    IconCheck,
    IconLock,
    IconQuestionMark,
    IconX,
} from '@tabler/icons-react'
import { Group, Row } from '@/components/ios'
import { Sheet } from '@/components/sheet'
import { useSiteActions } from '@/hooks/useSiteActions'
import { countryName, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Site, SiteLock, SiteNode } from '@/types'

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
                            !site.lock && site.passing === 0 && site.checked_at
                                ? 'Пока ни один сервер не прошёл проверку, сайт идёт через весь пул, как раньше.'
                                : `Проверяется ${site.check_url}. Панель перепроверяет сайт, когда меняется пул, и раз в 12 часов.`
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

                    {site.nodes.length > 0 && (
                        <Group title='Серверы пула' footer={nodesFooter}>
                            {site.nodes.map(n => (
                                <NodeResult
                                    key={n.tag}
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
