import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function TOTPSetup({
    qrCode,
    secret,
    onConfirm,
    loading,
}: {
    qrCode: string
    secret: string
    onConfirm: (code: string) => void
    loading: boolean
}) {
    const [code, setCode] = useState('')

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault()
        onConfirm(code)
    }

    return (
        <div className='space-y-6'>
            <div className='text-center'>
                <h3 className='text-lg font-semibold'>
                    Настройка двухфакторной аутентификации
                </h3>
                <p className='text-sm text-muted-foreground mt-1'>
                    Отсканируйте QR-код в Google Authenticator или Aegis
                </p>
            </div>
            <div className='flex justify-center'>
                <img
                    src={qrCode}
                    alt='TOTP QR Code'
                    className='w-64 h-64 rounded-xl bg-white p-2'
                />
            </div>
            <div className='bg-muted rounded-lg p-3'>
                <p className='text-xs text-muted-foreground mb-1'>
                    Секрет для ручного ввода:
                </p>
                <p className='font-mono text-sm break-all select-all'>
                    {secret}
                </p>
            </div>
            <form onSubmit={handleSubmit} className='space-y-4'>
                <div className='space-y-2'>
                    <Label>Код подтверждения</Label>
                    <Input
                        type='text'
                        inputMode='numeric'
                        pattern='[0-9]*'
                        maxLength={6}
                        value={code}
                        onChange={e =>
                            setCode(e.target.value.replace(/\D/g, ''))
                        }
                        placeholder='000000'
                        className='text-center text-2xl tracking-[0.5em] placeholder:tracking-[0.5em]'
                        autoFocus
                    />
                </div>
                <Button
                    type='submit'
                    disabled={code.length !== 6 || loading}
                    className='w-full'
                >
                    {loading ? 'Проверка...' : 'Подтвердить'}
                </Button>
            </form>
        </div>
    )
}
