import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
    IconDeviceDesktop,
    IconDeviceLaptop,
    IconDeviceMobile,
    IconDeviceTv,
    IconDeviceWatch,
    IconHome,
    type Icon,
} from '@tabler/icons-react'
import { Group, IconTile, PageTitle, Row, Switch } from '@/components/ios'
import { useDevices } from '@/hooks/usePanelData'
import { api } from '@/lib/api'
import type { Device, DevicesResponse } from '@/types'

const KINDS: { re: RegExp; icon: Icon; color: string }[] = [
    { re: /watch|часы/i, icon: IconDeviceWatch, color: '#8e8e93' },
    {
        re: /iphone|phone|телефон|фон|android|pixel|galaxy/i,
        icon: IconDeviceMobile,
        color: '#30d158',
    },
    {
        re: /mac|book|laptop|air|ноут/i,
        icon: IconDeviceLaptop,
        color: '#0a84ff',
    },
    {
        re: /tv|телевиз|ящик|box|приставк/i,
        icon: IconDeviceTv,
        color: '#6366f1',
    },
    {
        re: /хаб|hub|станц|пол|lamp|ламп|розетк|датчик/i,
        icon: IconHome,
        color: '#ff9f0a',
    },
]

function kindOf(name: string) {
    return (
        KINDS.find(k => k.re.test(name)) ?? {
            icon: IconDeviceDesktop,
            color: '#64d2ff',
        }
    )
}

export function DevicesPage() {
    const qc = useQueryClient()
    const query = useDevices()
    const data = query.data

    const toggle = useMutation({
        mutationFn: ({ mac, vpn }: { mac: string; vpn: boolean }) =>
            api.post('/api/devices/vpn', { mac, vpn }),
        onMutate: ({ mac, vpn }) =>
            qc.setQueryData<DevicesResponse>(['devices'], old =>
                old
                    ? {
                          ...old,
                          devices: old.devices.map(d =>
                              d.mac === mac
                                  ? { ...d, vpn, via: vpn ? 'host' : undefined }
                                  : d,
                          ),
                      }
                    : old,
            ),
        onSettled: () => qc.invalidateQueries({ queryKey: ['devices'] }),
    })

    // Devices following the segment flip with it, so the list is refetched rather than patched
    const newDevices = useMutation({
        mutationFn: (vpn: boolean) => api.post('/api/devices/new', { vpn }),
        onMutate: vpn =>
            qc.setQueryData<DevicesResponse>(['devices'], old =>
                old ? { ...old, new_devices_vpn: vpn } : old,
            ),
        onSettled: () => qc.invalidateQueries({ queryKey: ['devices'] }),
    })

    if (query.error) {
        return (
            <>
                <PageTitle title='Устройства' />
                <Group footer='Панель читает список устройств через API роутера на 127.0.0.1:79 — тот же, что использует XKeen.'>
                    <Row
                        label='Не удалось получить список устройств'
                        hint={query.error.message}
                    />
                </Group>
            </>
        )
    }

    const devices = data?.devices ?? []
    const inVpn = devices.filter(d => d.vpn).length
    const online = devices.filter(d => d.active)
    const offline = devices.filter(d => !d.active)
    const policyMissing = data && !data.policy_found

    const row = (d: Device) => {
        const kind = kindOf(d.name)
        const KindIcon = kind.icon
        return (
            <Row
                key={d.mac}
                icon={
                    <span className={d.active ? '' : 'opacity-45'}>
                        <IconTile color={kind.color}>
                            <KindIcon className='size-[17px]' stroke={2.2} />
                        </IconTile>
                    </span>
                }
                label={<span className='block break-words'>{d.name}</span>}
                hint={
                    d.vpn
                        ? d.via === 'segment'
                            ? 'Через VPN, как вся сеть'
                            : 'Через VPN'
                        : 'Напрямую'
                }
                trailing={
                    <Switch
                        checked={d.vpn}
                        onChange={vpn => toggle.mutate({ mac: d.mac, vpn })}
                        label={`VPN для ${d.name}`}
                        disabled={policyMissing || toggle.isPending}
                    />
                }
            />
        )
    }

    return (
        <>
            <PageTitle
                title='Устройства'
                subtitle={
                    data ? `${inVpn} из ${devices.length} через VPN` : ' '
                }
            />

            {policyMissing && (
                <Group footer='Без политики XKeen проксирует все устройства роутера. Создайте в веб-интерфейсе Keenetic политику с описанием xkeen — тогда здесь можно будет выбирать устройства.'>
                    <Row label='Политика xkeen не найдена' tone='destructive' />
                </Group>
            )}

            {data && !policyMissing && (
                <Group
                    footer={
                        newDevices.error ? (
                            <span className='text-destructive'>
                                {newDevices.error.message}
                            </span>
                        ) : (
                            'Всё, что впервые подключится к домашней сети, и устройства, у которых в Keenetic выбрана политика сегмента («как вся сеть»).'
                        )
                    }
                >
                    <Row
                        label='Новые устройства через VPN'
                        trailing={
                            <Switch
                                checked={data.new_devices_vpn}
                                onChange={vpn => newDevices.mutate(vpn)}
                                label='Новые устройства через VPN'
                                disabled={newDevices.isPending}
                            />
                        }
                    />
                </Group>
            )}

            {toggle.error && (
                <p className='px-4 text-[13px] text-destructive'>
                    {toggle.error.message}
                </p>
            )}

            <div className='grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start'>
                {online.length > 0 && (
                    <Group title='В сети'>{online.map(row)}</Group>
                )}
                {offline.length > 0 && (
                    <Group title='Не в сети'>{offline.map(row)}</Group>
                )}
            </div>
        </>
    )
}
