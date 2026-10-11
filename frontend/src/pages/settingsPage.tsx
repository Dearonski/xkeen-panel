import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
    IconActivity,
    IconAdjustmentsHorizontal,
    IconArrowsExchange,
    IconCpu,
    IconDownload,
    IconFingerprint,
} from '@tabler/icons-react'
import { Group, IconTile, PageTitle, Row, Switch } from '@/components/ios'
import { useAuth } from '@/hooks/useAuth'
import { useStatus, useUpdate } from '@/hooks/usePanelData'
import { api } from '@/lib/api'
import { passkeySupported, registerPasskey } from '@/lib/webauthn'
import type { Status } from '@/types'

const icon = (color: string, Glyph: typeof IconCpu) => (
    <IconTile color={color}>
        <Glyph className='size-[17px]' stroke={2.2} />
    </IconTile>
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
                </Group>

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
