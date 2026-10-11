import { DeviceGroups, NewDevicesGroup } from '@/components/deviceList'
import { Group, PageTitle, Row } from '@/components/ios'
import { useDevices } from '@/hooks/usePanelData'

export function DevicesPage() {
    const query = useDevices()
    const data = query.data

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

    const inVpn = data?.devices.filter(d => d.vpn).length ?? 0

    return (
        <>
            <PageTitle
                title='Устройства'
                subtitle={
                    data ? `${inVpn} из ${data.devices.length} через VPN` : ' '
                }
            />

            {data && !data.policy_found && (
                <Group footer='Без политики XKeen проксирует все устройства роутера. Создайте её в мастере настройки — тогда здесь можно будет выбирать устройства.'>
                    <Row label='Политика xkeen не найдена' tone='destructive' />
                    <Row label='Открыть мастер настройки' to='/welcome' />
                </Group>
            )}

            {data?.policy_found && <NewDevicesGroup data={data} />}
            {data && <DeviceGroups data={data} columns />}
        </>
    )
}
