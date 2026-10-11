import { useEffect, useMemo, useState } from 'react'
import { IconCheck } from '@tabler/icons-react'
import { Group, Row } from '@/components/ios'
import { Sheet } from '@/components/sheet'
import { countryName } from '@/lib/format'

export type PanelSettings = {
    avoid_countries: string[]
    subscription_refresh_minutes: number
}

const check = <IconCheck className='size-5 shrink-0 text-tint' stroke={2.6} />

export function CountriesSheet({
    open,
    onClose,
    value,
    candidates,
    onSave,
}: {
    open: boolean
    onClose: () => void
    value: string[]
    candidates: string[]
    onSave: (countries: string[]) => Promise<unknown>
}) {
    const [picked, setPicked] = useState<string[]>(value)
    const [error, setError] = useState('')

    useEffect(() => {
        if (open) {
            setPicked(value)
            setError('')
        }
    }, [open, value])

    const options = useMemo(
        () =>
            [...new Set([...value, ...candidates, 'RU', 'BY'])].sort((a, b) =>
                (countryName(a) || a).localeCompare(countryName(b) || b, 'ru'),
            ),
        [value, candidates],
    )

    const toggle = (code: string) =>
        setPicked(p =>
            p.includes(code) ? p.filter(c => c !== code) : [...p, code],
        )

    const save = async () => {
        try {
            await onSave(picked)
            onClose()
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось сохранить')
        }
    }

    return (
        <Sheet
            open={open}
            onClose={onClose}
            title='Не выбирать страны'
            done={{ label: 'Готово', onClick: save }}
        >
            <Group
                footer={
                    error ? (
                        <span className='text-destructive'>{error}</span>
                    ) : (
                        'Панель не возьмёт серверы этих стран ни в пул, ни при автоматической замене. Вручную выбрать их по-прежнему можно.'
                    )
                }
            >
                {options.map(code => (
                    <Row
                        key={code}
                        label={countryName(code) || code}
                        onClick={() => toggle(code)}
                        trailing={
                            picked.includes(code) ? (
                                check
                            ) : (
                                <span className='size-5' />
                            )
                        }
                    />
                ))}
            </Group>
        </Sheet>
    )
}

const INTERVALS = [
    { minutes: 15, label: 'Каждые 15 минут' },
    { minutes: 30, label: 'Каждые 30 минут' },
    { minutes: 60, label: 'Каждый час' },
    { minutes: 180, label: 'Каждые 3 часа' },
    { minutes: 360, label: 'Каждые 6 часов' },
    { minutes: 1440, label: 'Раз в сутки' },
    { minutes: 0, label: 'Не обновлять' },
]

export function intervalLabel(minutes: number) {
    if (minutes === 0) return 'Выкл.'
    if (minutes === 1440) return 'раз в сутки'
    if (minutes % 60 === 0) return `каждые ${minutes / 60} ч`
    return `каждые ${minutes} мин`
}

export function RefreshSheet({
    open,
    onClose,
    value,
    onSave,
}: {
    open: boolean
    onClose: () => void
    value: number
    onSave: (minutes: number) => Promise<unknown>
}) {
    const [picked, setPicked] = useState(value)
    const [error, setError] = useState('')

    useEffect(() => {
        if (open) {
            setPicked(value)
            setError('')
        }
    }, [open, value])

    const save = async () => {
        try {
            await onSave(picked)
            onClose()
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось сохранить')
        }
    }

    return (
        <Sheet
            open={open}
            onClose={onClose}
            title='Обновлять подписку'
            done={{ label: 'Готово', onClick: save }}
        >
            <Group
                footer={
                    error ? (
                        <span className='text-destructive'>{error}</span>
                    ) : (
                        'Провайдеры меняют адреса серверов. Панель скачивает подписку заново и обновляет пул без перезапуска, если сервер в нём не изменился.'
                    )
                }
            >
                {INTERVALS.map(i => (
                    <Row
                        key={i.minutes}
                        label={i.label}
                        onClick={() => setPicked(i.minutes)}
                        trailing={
                            picked === i.minutes ? (
                                check
                            ) : (
                                <span className='size-5' />
                            )
                        }
                    />
                ))}
            </Group>
        </Sheet>
    )
}
