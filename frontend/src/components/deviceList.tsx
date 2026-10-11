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
import { Group, IconTile, Row, Switch } from '@/components/ios'
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

export function DeviceGroups({
    data,
    columns,
}: {
    data: DevicesResponse
    columns?: boolean
}) {
    const qc = useQueryClient()
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
                        disabled={!data.policy_found || toggle.isPending}
                    />
                }
            />
        )
    }

    const online = data.devices.filter(d => d.active)
    const offline = data.devices.filter(d => !d.active)

    return (
        <>
            {toggle.error && (
                <p className='px-4 text-[13px] text-destructive'>
                    {toggle.error.message}
                </p>
            )}
            <div
                className={
                    columns
                        ? 'grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start'
                        : 'flex flex-col gap-4'
                }
            >
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

export function NewDevicesGroup({ data }: { data: DevicesResponse }) {
    const qc = useQueryClient()
    // Devices following the segment flip with it, so the list is refetched rather than patched
    const newDevices = useMutation({
        mutationFn: (vpn: boolean) => api.post('/api/devices/new', { vpn }),
        onMutate: vpn =>
            qc.setQueryData<DevicesResponse>(['devices'], old =>
                old ? { ...old, new_devices_vpn: vpn } : old,
            ),
        onSettled: () => qc.invalidateQueries({ queryKey: ['devices'] }),
    })

    return (
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
    )
}
