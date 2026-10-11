import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
    IconActivity,
    IconBan,
    IconChevronRight,
    IconRefresh,
    IconAdjustmentsHorizontal,
    IconArrowsExchange,
    IconCpu,
    IconDownload,
    IconFingerprint,
    IconWand,
} from '@tabler/icons-react'
import { Group, IconTile, PageTitle, Row, Switch } from '@/components/ios'
import {
    CountriesSheet,
    RefreshSheet,
    intervalLabel,
    type PanelSettings,
} from '@/components/settingsSheets'
import { useAuth } from '@/hooks/useAuth'
import { useServers, useStatus, useUpdate } from '@/hooks/usePanelData'
import { serverCountry } from '@/lib/format'
import { api } from '@/lib/api'
import { passkeySupported, registerPasskey } from '@/lib/webauthn'
import type { Status } from '@/types'

const icon = (color: string, Glyph: typeof IconCpu) => (
    <IconTile color={color}>
        <Glyph className='size-[17px]' stroke={2.2} />
    </IconTile>
)

const chevron = (
    <IconChevronRight
        className='size-4 shrink-0 text-label-tertiary'
        stroke={2.6}
    />
)

export function SettingsPage() {
    const qc = useQueryClient()
    const { logout } = useAuth()
    const status = useStatus().data
    const update = useUpdate().data

    const watchdog = useMutation({
        mutationFn: (active: boolean) =>
            api.post('/api/watchdog/toggle', { active }),
        onMutate: active =>
            qc.setQueryData<Status>(['status'], old =>
                old ? { ...old, watchdog_active: active } : old,
            ),
        onSettled: () => qc.invalidateQueries({ queryKey: ['status'] }),
    })

    const passkeys = useQuery({
        queryKey: ['passkeys'],
        queryFn: () => api.get<{ passkeys: string[] }>('/api/account/passkey'),
    })
    const addPasskey = useMutation({
        mutationFn: () => registerPasskey(),
        onSettled: () => qc.invalidateQueries({ queryKey: ['passkeys'] }),
    })
    const removePasskey = useMutation({
        mutationFn: (id: string) => api.del('/api/account/passkey', { id }),
        onSettled: () => qc.invalidateQueries({ queryKey: ['passkeys'] }),
    })
    const passkey = passkeys.data?.passkeys?.[0]

    const servers = useServers().data ?? []
    const [sheet, setSheet] = useState<'countries' | 'refresh' | null>(null)
    const settings = useQuery({
        queryKey: ['panel-settings'],
        queryFn: () => api.get<PanelSettings>('/api/panel/settings'),
    })
    const saveSettings = useMutation({
        mutationFn: (next: PanelSettings) =>
            api.put<PanelSettings>('/api/panel/settings', next),
        onSuccess: data => qc.setQueryData(['panel-settings'], data),
    })
    const avoid = settings.data?.avoid_countries ?? []
    const refresh = settings.data?.subscription_refresh_minutes ?? 0

    return (
        <>
            <PageTitle title='Настройки' />

            <div className='grid grid-cols-1 gap-5 lg:grid-cols-2 lg:items-start'>
                <Group
                    title='Автоматика'
                    footer='Панель проверяет связь через VPN каждые 2 минуты. Если сервер перестал работать, она переключится на другой — при ручном выборе на сервер той же страны.'
                >
                    <Row
                        icon={icon('#30d158', IconArrowsExchange)}
                        label='Менять упавший сервер'
                        trailing={
                            <Switch
                                checked={status?.watchdog_active ?? false}
                                onChange={active => watchdog.mutate(active)}
                                label='Менять упавший сервер'
                                disabled={!status || watchdog.isPending}
                            />
                        }
                    />
                    <Row
                        icon={icon('#ff453a', IconBan)}
                        label='Не выбирать страны'
                        detail={avoid.length ? avoid.join(', ') : 'Нет'}
                        onClick={() => setSheet('countries')}
                        disabled={!settings.data}
                        trailing={chevron}
                    />
                    <Row
                        icon={icon('#ff9f0a', IconRefresh)}
                        label='Обновлять подписку'
                        detail={intervalLabel(refresh)}
                        onClick={() => setSheet('refresh')}
                        disabled={!settings.data}
                        trailing={chevron}
                    />
                </Group>

                <CountriesSheet
                    open={sheet === 'countries'}
                    onClose={() => setSheet(null)}
                    value={avoid}
                    candidates={servers.map(serverCountry).filter(Boolean)}
                    onSave={avoid_countries =>
                        saveSettings.mutateAsync({
                            avoid_countries,
                            subscription_refresh_minutes: refresh,
                        })
                    }
                />
                <RefreshSheet
                    open={sheet === 'refresh'}
                    onClose={() => setSheet(null)}
                    value={refresh}
                    onSave={subscription_refresh_minutes =>
                        saveSettings.mutateAsync({
                            avoid_countries: avoid,
                            subscription_refresh_minutes,
                        })
                    }
                />

                <Group title='XKeen'>
                    <Row
                        icon={icon('#6366f1', IconCpu)}
                        label='Ядро'
                        detail={
                            status
                                ? `${status.core === 'mihomo' ? 'Mihomo' : 'Xray'} · ${status.mode}`
                                : '—'
                        }
                    />
                    <Row
                        icon={icon('#0a84ff', IconAdjustmentsHorizontal)}
                        label='Порты и исключения'
                        detail={status?.xkeen_version}
                        to='/settings/xkeen'
                    />
                </Group>

                <Group
                    title='Панель'
                    footer={
                        addPasskey.error ? (
                            <span className='text-destructive'>
                                {addPasskey.error.message}
                            </span>
                        ) : (
                            'Face ID и Touch ID работают только при входе через HTTPS-домен роутера.'
                        )
                    }
                >
                    <Row
                        icon={icon('#8e8e93', IconDownload)}
                        label='Обновления'
                        detail={
                            update?.available && update.latest ? (
                                <span className='text-tint'>
                                    {update.latest.tag}
                                </span>
                            ) : (
                                status?.panel_version
                            )
                        }
                        to='/settings/update'
                    />
                    {passkey ? (
                        <Row
                            icon={icon('#30d158', IconFingerprint)}
                            label='Вход по Face ID'
                            hint={`ключ ${passkey.slice(0, 8)}…`}
                            trailing={
                                <button
                                    type='button'
                                    onClick={() =>
                                        removePasskey.mutate(passkey)
                                    }
                                    disabled={removePasskey.isPending}
                                    className='min-h-9 cursor-pointer px-1 text-[15px] text-destructive'
                                >
                                    Удалить
                                </button>
                            }
                        />
                    ) : (
                        <Row
                            icon={icon('#30d158', IconFingerprint)}
                            label={
                                addPasskey.isPending
                                    ? 'Создаю ключ…'
                                    : 'Включить вход по Face ID'
                            }
                            tone='tint'
                            onClick={() => addPasskey.mutate()}
                            disabled={
                                !passkeySupported() || addPasskey.isPending
                            }
                        />
                    )}
                    <Row
                        icon={icon('#5856d6', IconWand)}
                        label='Мастер настройки'
                        to='/welcome'
                    />
                    <Row
                        icon={icon('#ff375f', IconActivity)}
                        label='Диагностика и журнал'
                        to='/settings/diagnostics'
                    />
                </Group>

                <Group>
                    <Row label='Выйти' tone='destructive' onClick={logout} />
                </Group>
            </div>
        </>
    )
}
