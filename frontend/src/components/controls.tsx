import { useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
    IconPlayerPlay,
    IconPlayerStop,
    IconRefresh,
    IconStethoscope,
} from '@tabler/icons-react'
import type { SelfTestResult } from '@/types'

export function Controls({
    watchdogActive,
    coreRunning,
    onRestart,
    onStart,
    onStop,
    onSelfTest,
    onToggleWatchdog,
    loading,
}: {
    watchdogActive: boolean
    coreRunning: boolean
    onRestart: () => void
    onStart: () => void
    onStop: () => void
    onSelfTest: () => Promise<SelfTestResult>
    onToggleWatchdog: (active: boolean) => void
    loading: boolean
}) {
    const [confirming, setConfirming] = useState<'restart' | 'stop' | null>(
        null,
    )
    const [testing, setTesting] = useState(false)
    const [testResult, setTestResult] = useState<SelfTestResult | null>(null)

    const confirm = (action: 'restart' | 'stop', run: () => void) => {
        if (confirming === action) {
            run()
            setConfirming(null)
        } else {
            setConfirming(action)
            setTimeout(() => setConfirming(null), 3000)
        }
    }

    const handleSelfTest = async () => {
        setTesting(true)
        setTestResult(null)
        try {
            setTestResult(await onSelfTest())
        } catch (e) {
            setTestResult({
                success: false,
                core: '',
                output: '',
                error: e instanceof Error ? e.message : 'ошибка проверки',
            })
        } finally {
            setTesting(false)
        }
    }

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base'>Управление</CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
                <Button
                    variant={
                        confirming === 'restart' ? 'destructive' : 'outline'
                    }
                    className='w-full'
                    onClick={() => confirm('restart', onRestart)}
                    disabled={loading}
                >
                    <IconRefresh className='size-4' />
                    {confirming === 'restart'
                        ? 'Подтвердить перезапуск'
                        : 'Перезапустить XKeen'}
                </Button>

                {coreRunning ? (
                    <Button
                        variant={
                            confirming === 'stop' ? 'destructive' : 'outline'
                        }
                        className='w-full'
                        onClick={() => confirm('stop', onStop)}
                        disabled={loading}
                    >
                        <IconPlayerStop className='size-4' />
                        {confirming === 'stop'
                            ? 'Подтвердить остановку'
                            : 'Остановить'}
                    </Button>
                ) : (
                    <Button
                        variant='outline'
                        className='w-full'
                        onClick={onStart}
                        disabled={loading}
                    >
                        <IconPlayerPlay className='size-4' />
                        Запустить
                    </Button>
                )}

                <Button
                    variant='outline'
                    className='w-full'
                    onClick={handleSelfTest}
                    disabled={loading || testing}
                >
                    <IconStethoscope className='size-4' />
                    {testing ? 'Проверка...' : 'Проверить конфигурацию'}
                </Button>

                {testResult && (
                    <p
                        className={`text-xs whitespace-pre-wrap ${
                            testResult.success
                                ? 'text-emerald-400'
                                : 'text-red-400'
                        }`}
                    >
                        {testResult.success
                            ? `Конфигурация ${testResult.core} корректна`
                            : testResult.output || testResult.error}
                    </p>
                )}

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
