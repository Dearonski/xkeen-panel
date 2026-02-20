import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    CardDescription,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuth } from '@/hooks/useAuth'

export function LoginPage() {
    const { login } = useAuth()
    const [username, setUsername] = useState('')
    const [password, setPassword] = useState('')
    const [totpCode, setTotpCode] = useState('')
    const [error, setError] = useState('')

    const loginMutation = useMutation({
        mutationFn: () => login(username, password, totpCode),
        onError: (err: Error) => setError(err.message),
    })

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault()
        setError('')
        loginMutation.mutate()
    }

    return (
        <div className='min-h-screen flex items-center justify-center p-4'>
            <div className='w-full max-w-md'>
                <Card>
                    <CardHeader className='text-center'>
                        <CardTitle className='text-2xl'>XKeen Panel</CardTitle>
                        <CardDescription>
                            Вход в панель управления
                        </CardDescription>
                    </CardHeader>
                    <CardContent>
                        {error && (
                            <div className='mb-4 p-3 bg-destructive/10 border border-destructive/30 rounded-lg text-sm text-destructive'>
                                {error}
                            </div>
                        )}

                        <form
                            onSubmit={handleSubmit}
                            className='space-y-4'
                            autoComplete='on'
                        >
                            <div className='space-y-2'>
                                <Label htmlFor='login-username'>Логин</Label>
                                <Input
                                    id='login-username'
                                    name='username'
                                    autoComplete='username'
                                    value={username}
                                    onChange={e => setUsername(e.target.value)}
                                    required
                                    autoFocus
                                />
                            </div>
                            <div className='space-y-2'>
                                <Label htmlFor='login-password'>Пароль</Label>
                                <Input
                                    id='login-password'
                                    name='password'
                                    type='password'
                                    autoComplete='current-password'
                                    value={password}
                                    onChange={e => setPassword(e.target.value)}
                                    required
                                />
                            </div>
                            <div className='space-y-2'>
                                <Label htmlFor='login-totp'>TOTP-код</Label>
                                <Input
                                    id='login-totp'
                                    name='totp'
                                    type='text'
                                    inputMode='numeric'
                                    autoComplete='one-time-code'
                                    pattern='[0-9]*'
                                    maxLength={6}
                                    value={totpCode}
                                    onChange={e =>
                                        setTotpCode(
                                            e.target.value.replace(/\D/g, ''),
                                        )
                                    }
                                    placeholder='000000'
                                    required
                                    className='text-center text-xl tracking-[0.3em] placeholder:tracking-[0.3em]'
                                />
                            </div>
                            <Button
                                type='submit'
                                disabled={
                                    loginMutation.isPending ||
                                    totpCode.length !== 6
                                }
                                className='w-full'
                            >
                                {loginMutation.isPending ? 'Вход...' : 'Войти'}
                            </Button>
                        </form>
                    </CardContent>
                </Card>
            </div>
        </div>
    )
}
