import { useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { IconArrowsShuffle, IconRefresh } from '@tabler/icons-react'
import type { PoolStatus, Status } from '@/types'

export function XKeenCard({
    status,
    pool,
    onEnablePool,
    onDisablePool,
    onSyncPool,
    onSyncMihomo,
    loading,
}: {
    status: Status | undefined
    pool: PoolStatus | undefined
    onEnablePool: () => void
    onDisablePool: () => void
    onSyncPool: () => void
    onSyncMihomo: () => void
    loading: boolean
}) {
    const [confirming, setConfirming] = useState(false)

    const isPool = pool?.mode === 'pool'
    const isMihomo = status?.core === 'mihomo'

    const confirmEnable = () => {
        if (confirming) {
            onEnablePool()
            setConfirming(false)
        } else {
            setConfirming(true)
            setTimeout(() => setConfirming(false), 5000)
        }
    }

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base flex items-center justify-between'>
                    XKeen
                    {status?.xkeen_version && (
                        <Badge variant='outline'>{status.xkeen_version}</Badge>
                    )}
                </CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
                <dl className='text-xs space-y-1'>
                    <div className='flex justify-between'>
                        <dt className='text-muted-foreground'>Ядро</dt>
                        <dd>{status?.core ?? '—'}</dd>
                    </div>
                    <div className='flex justify-between'>
                        <dt className='text-muted-foreground'>Режим</dt>
                        <dd>{status?.mode ?? '—'}</dd>
                    </div>
                    <div className='flex justify-between'>
                        <dt className='text-muted-foreground'>Конфиг</dt>
                        <dd>
                            {isPool
                                ? `пул из ${pool?.pool_tags?.length ?? 0} нод`
                                : 'один сервер'}
                        </dd>
                    </div>
                    {isPool && pool?.current_tag && (
                        <div className='flex justify-between'>
                            <dt className='text-muted-foreground'>
                                Активная нода
                            </dt>
                            <dd>{pool.current_tag}</dd>
                        </div>
                    )}
                </dl>

                {isMihomo ? (
                    <>
                        <Button
                            variant='outline'
                            className='w-full'
                            onClick={onSyncMihomo}
                            disabled={loading}
                        >
                            <IconRefresh className='size-4' />
                            Синхронизировать proxies с подпиской
                        </Button>
                        <p className='text-xs text-muted-foreground'>
                            Панель приводит секцию <code>proxies</code> в
                            config.yaml к подписке и обновляет списки в
                            proxy-groups. Активную ноду выбирает сама группа
                            (url-test / select), а не панель.
                        </p>
                    </>
                ) : isPool ? (
                    <>
                        <Button
                            variant='outline'
                            className='w-full'
                            onClick={onSyncPool}
                            disabled={loading}
                        >
                            <IconRefresh className='size-4' />
                            Синхронизировать пул с подпиской
                        </Button>
                        <Button
                            variant='outline'
                            className='w-full'
                            onClick={onDisablePool}
                            disabled={loading}
                        >
                            Вернуться к одному серверу
                        </Button>
                        {pool?.api_available === false && (
                            <p className='text-xs text-amber-400'>
                                Ручной выбор ноды недоступен: в конфиге Xray нет
                                блока api. Его добавляет{' '}
                                <code>xkeen -sb on</code>. Без него ноду
                                выбирает балансировщик по пингу.
                            </p>
                        )}
                    </>
                ) : (
                    <>
                        <Button
                            variant={confirming ? 'destructive' : 'outline'}
                            className='w-full'
                            onClick={confirmEnable}
                            disabled={loading || status?.core !== 'xray'}
                        >
                            <IconArrowsShuffle className='size-4' />
                            {confirming
                                ? 'Подтвердить: переписать маршрутизацию'
                                : 'Собрать пул из подписки'}
                        </Button>
                        <p className='text-xs text-muted-foreground'>
                            Xray будет сам выбирать сервер по пингу и уводить
                            трафик с упавшего без перезапуска. Файлы
                            конфигурации перезаписываются — комментарии в них
                            будут потеряны, рядом останутся копии{' '}
                            <code>.bak</code>.
                        </p>
                    </>
                )}
            </CardContent>
        </Card>
    )
}
