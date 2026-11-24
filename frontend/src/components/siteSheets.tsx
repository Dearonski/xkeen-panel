import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
    IconBrandSentry,
    IconBrandTiktokFilled,
    IconCheck,
    IconChevronRight,
    IconLock,
    IconQuestionMark,
    IconSearch,
    IconWorld,
    IconX,
} from '@tabler/icons-react'
import { Group, Row, Segmented } from '@/components/ios'
import { Sheet } from '@/components/sheet'
import { useSiteSources } from '@/hooks/usePanelData'
import { useSiteActions } from '@/hooks/useSiteActions'
import { countryName, timeAgo } from '@/lib/format'
import { siteTemplates, type SiteTemplate } from '@/lib/siteTemplates'
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
    const current = site.nodes.find(n => n.key === site.sticky)
    if (current)
        return `${current.name} · подходят ${site.passing} из ${site.nodes.length}${when}`
    return `${site.passing} из ${site.nodes.length} серверов${when}`
}

// Tabler has no Claude mark; this path is Simple Icons' (CC0)
function ClaudeMark() {
    return (
        <svg
            viewBox='0 0 24 24'
            fill='currentColor'
            className='size-[58%]'
            aria-hidden
        >
            <path d='m4.7144 15.9555 4.7174-2.6471.079-.2307-.079-.1275h-.2307l-.7893-.0486-2.6956-.0729-2.3375-.0971-2.2646-.1214-.5707-.1215-.5343-.7042.0546-.3522.4797-.3218.686.0608 1.5179.1032 2.2767.1578 1.6514.0972 2.4468.255h.3886l.0546-.1579-.1336-.0971-.1032-.0972L6.973 9.8356l-2.55-1.6879-1.3356-.9714-.7225-.4918-.3643-.4614-.1578-1.0078.6557-.7225.8803.0607.2246.0607.8925.686 1.9064 1.4754 2.4893 1.8336.3643.3035.1457-.1032.0182-.0728-.164-.2733-1.3539-2.4467-1.445-2.4893-.6435-1.032-.17-.6194c-.0607-.255-.1032-.4674-.1032-.7285L6.287.1335 6.6997 0l.9957.1336.419.3642.6192 1.4147 1.0018 2.2282 1.5543 3.0296.4553.8985.2429.8318.091.255h.1579v-.1457l.1275-1.706.2368-2.0947.2307-2.6957.0789-.7589.3764-.9107.7468-.4918.5828.2793.4797.686-.0668.4433-.2853 1.8517-.5586 2.9021-.3643 1.9429h.2125l.2429-.2429.9835-1.3053 1.6514-2.0643.7286-.8196.85-.9046.5464-.4311h1.0321l.759 1.1293-.34 1.1657-1.0625 1.3478-.8804 1.1414-1.2628 1.7-.7893 1.36.0729.1093.1882-.0183 2.8535-.607 1.5421-.2794 1.8396-.3157.8318.3886.091.3946-.3278.8075-1.967.4857-2.3072.4614-3.4364.8136-.0425.0304.0486.0607 1.5482.1457.6618.0364h1.621l3.0175.2247.7892.522.4736.6376-.079.4857-1.2142.6193-1.6393-.3886-3.825-.9107-1.3113-.3279h-.1822v.1093l1.0929 1.0686 2.0035 1.8092 2.5075 2.3314.1275.5768-.3218.4554-.34-.0486-2.2039-1.6575-.85-.7468-1.9246-1.621h-.1275v.17l.4432.6496 2.3436 3.5214.1214 1.0807-.17.3521-.6071.2125-.6679-.1214-1.3721-1.9246L14.38 17.959l-1.1414-1.9428-.1397.079-.674 7.2552-.3156.3703-.7286.2793-.6071-.4614-.3218-.7468.3218-1.4753.3886-1.9246.3157-1.53.2853-1.9004.17-.6314-.0121-.0425-.1397.0182-1.4328 1.9672-2.1796 2.9446-1.7243 1.8456-.4128.164-.7164-.3704.0667-.6618.4008-.5889 2.386-3.0357 1.4389-1.882.929-1.0868-.0062-.1579h-.0546l-6.3385 4.1164-1.1293.1457-.4857-.4554.0608-.7467.2307-.2429 1.9064-1.3114Z' />
        </svg>
    )
}

const templateLook: Record<
    SiteTemplate['id'],
    { color: string; icon: ReactNode }
> = {
    tiktok: {
        color: '#000000',
        icon: <IconBrandTiktokFilled className='size-[60%]' />,
    },
    claude: {
        color: '#D97757',
        icon: <ClaudeMark />,
    },
    sentry: {
        color: '#362D59',
        icon: <IconBrandSentry className='size-[60%]' stroke={2} />,
    },
}

function TemplateTile({
    template,
    size = 30,
}: {
    template?: SiteTemplate
    size?: number
}) {
    const look = template
        ? templateLook[template.id]
        : {
              color: '#8E8E93',
              icon: <IconWorld className='size-[60%]' stroke={2} />,
          }
    return (
        <span
            className='flex shrink-0 items-center justify-center text-white'
            style={{
                background: look.color,
                width: size,
                height: size,
                borderRadius: size * 0.2667,
                boxShadow: 'inset 0 0 0 1px rgb(255 255 255 / 0.12)',
            }}
        >
            {look.icon}
        </span>
    )
}

const chevron = (
    <IconChevronRight
        className='size-4 shrink-0 text-label-tertiary'
        stroke={2.6}
    />
)

type AddStep = 'pick' | 'template' | 'custom'

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
    const [step, setStep] = useState<AddStep>('pick')
    const [template, setTemplate] = useState<SiteTemplate | null>(null)
    const [domains, setDomains] = useState('')
    const [ips, setIPs] = useState('')
    const [checkURL, setCheckURL] = useState('')
    const [countryPattern, setCountryPattern] = useState('')
    const [advanced, setAdvanced] = useState(false)
    const [error, setError] = useState('')

    const fill = (t: SiteTemplate | null) => {
        setTemplate(t)
        setDomains(t?.domains ?? '')
        setIPs(t?.ips ?? '')
        setCheckURL(t?.checkURL ?? '')
        setCountryPattern(t?.countryPattern ?? '')
        setAdvanced(!!t?.countryPattern)
        setError('')
    }

    useEffect(() => {
        if (!open) return
        setStep('pick')
        fill(null)
    }, [open])

    const submit = async () => {
        if (!domains.trim() || create.isPending) return
        setError('')
        try {
            const res = await create.mutateAsync({
                domains,
                ips,
                check_url: checkURL,
                country_pattern: countryPattern,
            })
            onCreated(res.id)
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось добавить')
        }
    }

    const done = {
        label: create.isPending ? 'Добавляю…' : 'Добавить',
        onClick: submit,
        disabled: !domains.trim() || create.isPending,
    }

    if (step === 'pick') {
        return (
            <Sheet open={open} onClose={onClose} title='Новый сайт'>
                <Group footer='Сайт пойдёт только через серверы, которые его открывают. Остальной трафик не меняется.'>
                    {siteTemplates.map(t => (
                        <Row
                            key={t.id}
                            icon={<TemplateTile template={t} />}
                            label={t.name}
                            hint={t.tagline}
                            trailing={chevron}
                            onClick={() => {
                                fill(t)
                                setStep('template')
                            }}
                        />
                    ))}
                </Group>
                <Group>
                    <Row
                        icon={<TemplateTile />}
                        label='Другой сайт'
                        hint='Свои домены и адрес проверки'
                        trailing={chevron}
                        onClick={() => {
                            fill(null)
                            setStep('custom')
                        }}
                    />
                </Group>
            </Sheet>
        )
    }

    if (step === 'template' && template) {
        const count = (s: string) => s.split(/[\s,;]+/).filter(Boolean).length
        return (
            <Sheet
                open={open}
                onClose={onClose}
                title=''
                back={{ label: 'Сайты', onClick: () => setStep('pick') }}
                done={done}
            >
                <div className='flex flex-col items-center gap-3 px-4 pt-2 text-center'>
                    <TemplateTile template={template} size={72} />
                    <h3 className='text-[22px] font-bold tracking-[0.35px]'>
                        {template.name}
                    </h3>
                    <p className='text-[15px] leading-snug tracking-[-0.24px] text-muted-foreground'>
                        {template.about}
                    </p>
                </div>
                <Group
                    footer={
                        error ? (
                            <span className='text-destructive'>{error}</span>
                        ) : undefined
                    }
                >
                    <Row label='Домены' detail={count(template.domains)} />
                    {template.ips && (
                        <Row label='IP-адреса' detail={count(template.ips)} />
                    )}
                    <Row
                        label='Проверка'
                        detail={new URL(template.checkURL).hostname}
                    />
                    {template.countryPattern && (
                        <Row label='Страна' detail='со слов сайта' />
                    )}
                </Group>
                <Group>
                    <Row
                        label='Изменить вручную'
                        tone='tint'
                        onClick={() => setStep('custom')}
                    />
                </Group>
            </Sheet>
        )
    }

    return (
        <Sheet
            open={open}
            onClose={onClose}
            title={template?.name ?? 'Другой сайт'}
            back={{
                label: template ? template.name : 'Сайты',
                onClick: () => setStep(template ? 'template' : 'pick'),
            }}
            done={done}
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
                footer='Страница, которая не открывается через плохие серверы, — например, страница входа. Пусто — главная страница первого домена.'
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

            {advanced ? (
                <Group
                    title='Страна со слов сайта'
                    footer='Регулярное выражение, группа в скобках которого — код страны на странице проверки. Сервер, где сайт видит страну из исключений автовыбора, не проходит.'
                >
                    <label htmlFor='site-country' className='sr-only'>
                        Страна со слов сайта
                    </label>
                    <input
                        id='site-country'
                        type='text'
                        autoComplete='off'
                        autoCapitalize='none'
                        autoCorrect='off'
                        spellCheck={false}
                        value={countryPattern}
                        onChange={e => setCountryPattern(e.target.value)}
                        placeholder='"country":"([A-Z]{2})"'
                        className='min-h-[52px] w-full bg-transparent font-mono text-[15px] outline-none placeholder:text-label-tertiary'
                    />
                </Group>
            ) : (
                <button
                    type='button'
                    onClick={() => setAdvanced(true)}
                    className='-mt-2 cursor-pointer self-start px-4 text-[15px] text-tint'
                >
                    Дополнительно
                </button>
            )}
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
                                  : `Проверяется ${site.check_url}.${site.country_pattern ? ' Страну панель берёт из ответа сайта.' : ''} Панель перепроверяет сайт, когда меняются его серверы, и раз в 12 часов.${site.sticky ? ' Сайт ходит через один сервер и уходит с него, только когда тот перестаёт работать.' : ''}`
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
                                    current={site.sticky === n.key}
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
    if (node.exit_ip)
        return `выход ${node.exit_ip} · ${countryLabel(node.exit_country)}`
    if (node.exit_country)
        return `сайт видит ${countryLabel(node.exit_country)}`
    return ''
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
    current,
    lockedAs,
    pending,
    busy,
    onPick,
}: {
    node: SiteNode
    current: boolean
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
                    current && 'сайт идёт через него',
                    countryName(node.country),
                    exitLabel(node),
                    node.split_exit && 'два выходных адреса',
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
