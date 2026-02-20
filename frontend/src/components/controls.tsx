import { useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { IconRefresh, IconPlayerPlay } from '@tabler/icons-react'

export function Controls({
    watchdogActive,
    onRestart,
    onUpdate,
    onToggleWatchdog,
    loading,
}: {
    watchdogActive: boolean
    onRestart: () => void
    onUpdate: () => void
    onToggleWatchdog: (active: boolean) => void
    loading: boolean
}) {
    const [confirming, setConfirming] = useState(false)

    const handleRestart = () => {
        if (confirming) {
            onRestart()
            setConfirming(false)
        } else {
            setConfirming(true)
            setTimeout(() => setConfirming(false), 3000)
        }
    }

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base'>Управление</CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
                <Button
                    variant={confirming ? 'destructive' : 'outline'}
                    className='w-full'
                    onClick={handleRestart}
                    disabled={loading}
                >
                    <IconPlayerPlay className='size-4' />
                    {confirming
                        ? 'Подтвердить перезапуск'
                        : 'Перезапустить XKeen'}
                </Button>

                <Button
                    variant='outline'
                    className='w-full'
                    onClick={onUpdate}
                    disabled={loading}
                >
                    <IconRefresh className='size-4' />
                    Обновить подписку (CLI)
                </Button>

                <div className='flex items-center justify-between pt-1'>
                    <Label htmlFor='watchdog-toggle'>Watchdog</Label>
                    <Switch
                        id='watchdog-toggle'
                        checked={watchdogActive}
                        onCheckedChange={onToggleWatchdog}
                        disabled={loading}
                    />
                </div>
            </CardContent>
        </Card>
    )
}
