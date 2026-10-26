import { useEffect, useState } from 'react'
import { IconCheck, IconQuestionMark, IconX } from '@tabler/icons-react'
import { Group, Row } from '@/components/ios'
import { Sheet } from '@/components/sheet'
import { useSiteActions } from '@/hooks/useSiteActions'
import { countryName, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Site, SiteNode } from '@/types'

export function siteSummary(site: Site) {
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
    const [checkURL, setCheckURL] = useState('')
    const [error, setError] = useState('')

    useEffect(() => {
        if (!open) return
        setDomains('')
        setCheckURL('')
        setError('')
    }, [open])

    const submit = async () => {
        if (!domains.trim() || create.isPending) return
        setError('')
        try {
            const res = await create.mutateAsync({
                domains,
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
    const { check, remove } = useSiteActions()
    const [confirming, setConfirming] = useState(false)
    const [error, setError] = useState('')

    useEffect(() => {
        setConfirming(false)
        setError('')
    }, [site?.id])

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
                            site.passing === 0 && site.checked_at
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

                    {site.nodes.length > 0 && (
                        <Group title='Серверы пула'>
                            {site.nodes.map(n => (
                                <NodeResult key={n.tag} node={n} />
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

function NodeResult({ node }: { node: SiteNode }) {
    const icon =
        node.state === 'ok' ? (
            <IconCheck className='size-[18px] text-success' stroke={2.6} />
        ) : node.state === 'fail' ? (
            <IconX className='size-[18px] text-destructive' stroke={2.6} />
        ) : (
            <IconQuestionMark
                className='size-[18px] text-muted-foreground'
                stroke={2.4}
            />
        )

    const detail =
        node.state === 'unknown'
            ? 'не проверен'
            : node.error
              ? node.error
              : node.state === 'ok'
                ? `${node.ms} мс`
                : `ответ ${node.status}`

    return (
        <Row
            icon={<span className='flex w-[30px] justify-center'>{icon}</span>}
            label={<span className='block break-words'>{node.name}</span>}
            hint={countryName(node.country) || undefined}
            trailing={
                <span
                    className={cn(
                        'max-w-[40%] shrink-0 truncate text-right text-[15px] tabular-nums',
                        node.state === 'fail'
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
