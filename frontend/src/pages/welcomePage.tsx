import { useEffect, useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
    IconCircleCheckFilled,
    IconCircleDashed,
    IconDeviceDesktop,
    IconLink,
    IconLoader2,
    IconPlayerPlay,
    IconTerminal2,
} from '@tabler/icons-react'
import { DeviceGroups, NewDevicesGroup } from '@/components/deviceList'
import { Group, Row } from '@/components/ios'
import { Logo } from '@/components/logo'
import { useDevices, useStatus } from '@/hooks/usePanelData'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

export type SetupState = {
    xkeen_installed: boolean
    xkeen_version: string
    core: string
    core_running: boolean
    autostart: boolean
    subscription: boolean
    servers: number
    configured: boolean
    pool_nodes: number
    router_api: boolean
    policy_found: boolean
    devices_in_vpn: number
    new_devices_vpn: boolean
    watchdog: boolean
    complete: boolean
}

type StepId = 'xkeen' | 'subscription' | 'devices' | 'start'

const STEPS: StepId[] = ['subscription', 'devices', 'start']

export const SETUP_LATER_KEY = 'xkeen_setup_later'

const INSTALL =
    'curl -fsSL https://raw.githubusercontent.com/Dearonski/xkeen-panel/main/install.sh | sh -s -- --with-xkeen'

export function useSetupState() {
    return useQuery({
        queryKey: ['setup'],
        queryFn: () => api.get<SetupState>('/api/setup/state'),
    })
}

function firstStep(s: SetupState): StepId {
    if (!s.xkeen_installed) return 'xkeen'
    if (!s.subscription || !s.configured) return 'subscription'
    if (!s.policy_found) return 'devices'
    return 'start'
}

export function WelcomePage() {
    const navigate = useNavigate()
    const setup = useSetupState()
    const [step, setStep] = useState<StepId | null>(null)
    const s = setup.data

    useEffect(() => {
        if (s && step === null) setStep(firstStep(s))
    }, [s, step])

    const later = () => {
        try {
            sessionStorage.setItem(SETUP_LATER_KEY, '1')
        } catch {
            // Private mode can refuse storage; the redirect then simply asks again
        }
        navigate('/')
    }

    const index = step ? STEPS.indexOf(step) : -1

    return (
        <div className='flex min-h-dvh flex-col'>
            <div aria-hidden className='bg-page fixed inset-0 -z-10' />

            <header className='mx-auto flex w-full max-w-lg items-center justify-between px-5 pt-[max(16px,env(safe-area-inset-top))]'>
                <div className='flex items-center gap-2'>
                    <Logo size={26} />
                    <span className='text-[17px] font-semibold'>XKeen</span>
                </div>
                {s?.xkeen_installed && (
                    <button
                        type='button'
                        onClick={later}
                        className='min-h-11 cursor-pointer px-1 text-[17px] text-tint'
                    >
                        Позже
                    </button>
                )}
            </header>

            {index >= 0 && (
                <div
                    className='mx-auto mt-3 flex w-full max-w-lg gap-1.5 px-5'
                    aria-label={`Шаг ${index + 1} из ${STEPS.length}`}
                >
                    {STEPS.map((id, i) => (
                        <span
                            key={id}
                            className={cn(
                                'h-1 flex-1 rounded-full transition-colors duration-300',
                                i <= index ? 'bg-primary' : 'bg-white/12',
                            )}
                        />
                    ))}
                </div>
            )}

            <main
                key={step ?? 'loading'}
                className='mx-auto flex w-full max-w-lg flex-1 flex-col px-5 pb-[max(24px,env(safe-area-inset-bottom))] pt-8 motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-right-4 motion-safe:duration-300'
            >
                {!s && (
                    <div className='flex flex-1 items-center justify-center'>
                        <IconLoader2 className='size-6 animate-spin text-muted-foreground' />
                    </div>
                )}
                {s && step === 'xkeen' && (
                    <XKeenStep onRetry={() => setup.refetch()} />
                )}
                {s && step === 'subscription' && (
                    <SubscriptionStep
                        state={s}
                        onNext={() => setStep('devices')}
                    />
                )}
                {s && step === 'devices' && (
                    <DevicesStep
                        state={s}
                        onBack={() => setStep('subscription')}
                        onNext={() => setStep('start')}
                    />
                )}
                {s && step === 'start' && (
                    <StartStep state={s} onBack={() => setStep('devices')} />
                )}
            </main>
        </div>
    )
}

function StepHeader({
    icon,
    color,
    title,
    children,
}: {
    icon: ReactNode
    color: string
    title: string
    children: ReactNode
}) {
    return (
        <div className='mb-6 flex flex-col items-center text-center'>
            <span
                className='mb-4 flex size-16 items-center justify-center rounded-[18px] text-white'
                style={{ background: color }}
            >
                {icon}
            </span>
            <h1 className='text-[28px] font-bold leading-tight tracking-[-0.8px]'>
                {title}
            </h1>
            <p className='mt-2 text-[17px] leading-snug tracking-[-0.43px] text-muted-foreground'>
                {children}
            </p>
        </div>
    )
}

function Actions({
    primary,
    secondary,
}: {
    primary: {
        label: string
        onClick: () => void
        disabled?: boolean
        busy?: boolean
    }
    secondary?: { label: string; onClick: () => void }
}) {
    return (
        <div className='mt-auto flex flex-col gap-2 pt-8'>
            <button
                type='button'
                onClick={primary.onClick}
                disabled={primary.disabled || primary.busy}
                className='flex min-h-[52px] cursor-pointer items-center justify-center gap-2 rounded-2xl bg-primary text-[17px] font-semibold transition-opacity disabled:opacity-50'
            >
                {primary.busy && (
                    <IconLoader2 className='size-5 animate-spin' />
                )}
                {primary.label}
            </button>
            {secondary && (
                <button
                    type='button'
                    onClick={secondary.onClick}
                    className='min-h-11 cursor-pointer text-[17px] text-tint'
                >
                    {secondary.label}
                </button>
            )}
        </div>
    )
}

function XKeenStep({ onRetry }: { onRetry: () => void }) {
    const [copied, setCopied] = useState(false)
    const copy = async () => {
        await navigator.clipboard?.writeText(INSTALL)
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
    }

    return (
        <>
            <StepHeader
                icon={<IconTerminal2 className='size-8' stroke={2} />}
                color='#8e8e93'
                title='XKeen не установлен'
            >
                Панель управляет XKeen, а его на роутере нет. Подключитесь к
                роутеру по SSH и выполните команду — она поставит XKeen и
                обновит панель.
            </StepHeader>
            <Group
                footer={
                    copied
                        ? 'Скопировано'
                        : 'Нужен Entware на USB-накопителе и компоненты Keenetic «Модули ядра подсистемы Netfilter» и «Протокол IPv6».'
                }
            >
                <button
                    type='button'
                    onClick={copy}
                    className='w-full cursor-pointer break-all py-3 text-left font-mono text-[13px] leading-relaxed'
                >
                    {INSTALL}
                </button>
            </Group>
            <Actions primary={{ label: 'Проверить снова', onClick: onRetry }} />
        </>
    )
}

function SubscriptionStep({
    state,
    onNext,
}: {
    state: SetupState
    onNext: () => void
}) {
    const qc = useQueryClient()
    const [editing, setEditing] = useState(!state.subscription)
    const [value, setValue] = useState('')
    const valid = /^https?:\/\/\S+$/.test(value.trim())

    const save = useMutation({
        mutationFn: (url: string) =>
            api.post<{ server_count: number; pool_error?: string }>(
                '/api/subscription',
                { url },
            ),
        onSuccess: async res => {
            await qc.invalidateQueries({ queryKey: ['setup'] })
            qc.invalidateQueries({ queryKey: ['servers'] })
            qc.invalidateQueries({ queryKey: ['subscription'] })
            if (!res.pool_error) setEditing(false)
        },
    })

    const error = save.error?.message ?? save.data?.pool_error
    const ready = state.subscription && state.configured && !editing

    const paste = async () => {
        try {
            setValue((await navigator.clipboard.readText()).trim())
        } catch {
            // Clipboard access is refused outside a user gesture on some browsers
        }
    }

    return (
        <>
            <StepHeader
                icon={<IconLink className='size-8' stroke={2} />}
                color='#6366f1'
                title='Подписка'
            >
                Вставьте ссылку от VPN-провайдера. Панель скачает серверы,
                выберет самые быстрые и сама соберёт конфигурацию Xray.
            </StepHeader>

            {ready ? (
                <Group footer='Российские сайты пойдут напрямую, остальное — через VPN.'>
                    <Row
                        icon={
                            <IconCircleCheckFilled className='size-[22px] text-success' />
                        }
                        label='Подписка загружена'
                        hint={`${state.servers} серверов${state.pool_nodes ? ` · в пуле ${state.pool_nodes}` : ''}`}
                    />
                    <Row
                        label='Заменить ссылку'
                        tone='tint'
                        onClick={() => setEditing(true)}
                    />
                </Group>
            ) : (
                <Group
                    footer={
                        error ? (
                            <span className='text-destructive'>{error}</span>
                        ) : (
                            'Обычно её дают в личном кабинете или в боте провайдера.'
                        )
                    }
                >
                    <form
                        className='flex min-h-[52px] items-center gap-2'
                        onSubmit={e => {
                            e.preventDefault()
                            if (valid) save.mutate(value.trim())
                        }}
                    >
                        <label htmlFor='welcome-url' className='sr-only'>
                            Ссылка на подписку
                        </label>
                        <input
                            id='welcome-url'
                            type='url'
                            inputMode='url'
                            autoComplete='off'
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
            )}

            <Actions
                primary={
                    ready
                        ? { label: 'Далее', onClick: onNext }
                        : {
                              label: save.isPending
                                  ? 'Загружаю серверы…'
                                  : 'Загрузить',
                              onClick: () => save.mutate(value.trim()),
                              disabled: !valid,
                              busy: save.isPending,
                          }
                }
                secondary={
                    editing && state.subscription
                        ? { label: 'Отмена', onClick: () => setEditing(false) }
                        : undefined
                }
            />
        </>
    )
}

function DevicesStep({
    state,
    onBack,
    onNext,
}: {
    state: SetupState
    onBack: () => void
    onNext: () => void
}) {
    const qc = useQueryClient()
    const devices = useDevices()
    const createPolicy = useMutation({
        mutationFn: () => api.post<SetupState>('/api/setup/policy'),
        onSuccess: data => {
            qc.setQueryData(['setup'], data)
            qc.invalidateQueries({ queryKey: ['devices'] })
        },
    })

    return (
        <>
            <StepHeader
                icon={<IconDeviceDesktop className='size-8' stroke={2} />}
                color='#30d158'
                title='Устройства'
            >
                {state.policy_found
                    ? 'Выберите, кто пойдёт через VPN. Остальные устройства будут работать напрямую.'
                    : 'Через VPN пойдут только выбранные устройства. Для этого на роутере нужна политика доступа xkeen — панель создаст её сама.'}
            </StepHeader>

            {!state.router_api && (
                <Group footer='Панель обращается к API роутера на 127.0.0.1:79. Если в xkeen.json задан rci_token, он должен быть верным.'>
                    <Row label='API роутера недоступен' tone='destructive' />
                </Group>
            )}

            {state.router_api && !state.policy_found && createPolicy.error && (
                <p className='px-4 text-[13px] text-destructive'>
                    {createPolicy.error.message}
                </p>
            )}

            {state.policy_found && devices.data && (
                <div className='flex flex-col gap-5'>
                    <NewDevicesGroup data={devices.data} />
                    <DeviceGroups data={devices.data} />
                </div>
            )}

            <Actions
                primary={
                    state.policy_found
                        ? { label: 'Далее', onClick: onNext }
                        : {
                              label: 'Создать политику',
                              onClick: () => createPolicy.mutate(),
                              disabled: !state.router_api,
                              busy: createPolicy.isPending,
                          }
                }
                secondary={{ label: 'Назад', onClick: onBack }}
            />
        </>
    )
}

function Check({ done, label }: { done: boolean; label: string }) {
    return (
        <Row
            icon={
                done ? (
                    <IconCircleCheckFilled className='size-[22px] text-success' />
                ) : (
                    <IconCircleDashed className='size-[22px] text-label-tertiary' />
                )
            }
            label={label}
        />
    )
}

function StartStep({
    state,
    onBack,
}: {
    state: SetupState
    onBack: () => void
}) {
    const qc = useQueryClient()
    const navigate = useNavigate()
    const [waitingSince, setWaitingSince] = useState<number | null>(null)
    const status = useStatus()
    const s = status.data

    const finish = useMutation({
        mutationFn: () => api.post<SetupState>('/api/setup/finish'),
        onSuccess: data => {
            qc.setQueryData(['setup'], data)
            setWaitingSince(Date.now())
        },
    })

    const verified =
        !!s?.connected &&
        !!s.proxy_checked &&
        (state.core_running || waitingSince !== null)
    const waiting = waitingSince !== null && !verified
    const timedOut = waiting && Date.now() - waitingSince > 90000

    useEffect(() => {
        if (!waiting) return
        const timer = setInterval(() => status.refetch(), 3000)
        return () => clearInterval(timer)
    }, [waiting, status])

    if (verified && (waitingSince !== null || state.complete)) {
        return (
            <>
                <StepHeader
                    icon={<IconCircleCheckFilled className='size-9' />}
                    color='#30d158'
                    title='Готово'
                >
                    VPN работает: связь проверена через выбранный сервер
                    {s?.latency_ms ? ` (${s.latency_ms} мс)` : ''}. Панель будет
                    следить за ним и сама заменит, если он упадёт.
                </StepHeader>
                <Actions
                    primary={{
                        label: 'Открыть панель',
                        onClick: () => navigate('/'),
                    }}
                />
            </>
        )
    }

    return (
        <>
            <StepHeader
                icon={<IconPlayerPlay className='size-8' stroke={2} />}
                color='#ff9f0a'
                title='Запуск'
            >
                Панель запустит XKeen, включит его автозапуск при старте роутера
                и проверит, что связь через VPN работает.
            </StepHeader>

            <Group
                footer={
                    finish.error ? (
                        <span className='text-destructive'>
                            {finish.error.message}
                        </span>
                    ) : timedOut ? (
                        <>
                            Связь через VPN пока не подтвердилась. Загляните в{' '}
                            <Link
                                to='/settings/diagnostics'
                                className='text-tint'
                            >
                                диагностику
                            </Link>{' '}
                            — там видно, что именно не работает.
                        </>
                    ) : undefined
                }
            >
                <Check done={state.core_running} label='XKeen запущен' />
                <Check
                    done={state.autostart}
                    label='Запускается вместе с роутером'
                />
                <Check
                    done={state.watchdog}
                    label='Упавший сервер меняется сам'
                />
                <Row
                    icon={
                        verified ? (
                            <IconCircleCheckFilled className='size-[22px] text-success' />
                        ) : waiting ? (
                            <IconLoader2 className='size-[22px] animate-spin text-tint' />
                        ) : (
                            <IconCircleDashed className='size-[22px] text-label-tertiary' />
                        )
                    }
                    label={
                        waiting
                            ? 'Проверяю связь через VPN…'
                            : 'Связь через VPN'
                    }
                />
            </Group>

            <Actions
                primary={{
                    label: waiting ? 'Проверяю…' : 'Запустить',
                    onClick: () => finish.mutate(),
                    busy: finish.isPending || waiting,
                }}
                secondary={{ label: 'Назад', onClick: onBack }}
            />
        </>
    )
}
