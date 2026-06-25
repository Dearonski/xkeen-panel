import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { registerPasskey, passkeySupported } from '@/lib/webauthn'
import { IconFingerprint, IconTrash } from '@tabler/icons-react'

export function PasskeyCard() {
    const qc = useQueryClient()
    const [error, setError] = useState('')
    const supported = passkeySupported()

    const list = useQuery({
        queryKey: ['passkeys'],
        queryFn: () => api.get<{ passkeys: string[] }>('/api/account/passkey'),
    })

    const add = useMutation({
        mutationFn: () => registerPasskey(),
        onSuccess: () => {
            setError('')
            qc.invalidateQueries({ queryKey: ['passkeys'] })
            qc.invalidateQueries({ queryKey: ['authStatus'] })
        },
        onError: (e: Error) => setError(e.message),
    })

    const remove = useMutation({
        mutationFn: (id: string) => api.del('/api/account/passkey', { id }),
        onSuccess: () => {
            qc.invalidateQueries({ queryKey: ['passkeys'] })
            qc.invalidateQueries({ queryKey: ['authStatus'] })
        },
    })

    const keys = list.data?.passkeys ?? []
    const hasKey = keys.length > 0

    return (
        <Card>
            <CardHeader className='pb-3'>
                <CardTitle className='text-base flex items-center gap-2'>
                    <IconFingerprint className='size-4' />
                    Passkey
                </CardTitle>
            </CardHeader>
            <CardContent className='space-y-3'>
                <p className='text-xs text-muted-foreground'>
                    Вход по Face ID / отпечатку, без пароля и TOTP.
                </p>

                {error && (
                    <div className='text-xs text-destructive'>{error}</div>
                )}

                {hasKey ? (
                    <div className='flex items-center justify-between gap-2 text-xs'>
                        <span className='font-mono text-muted-foreground truncate'>
                            passkey · {keys[0].slice(0, 10)}…
                        </span>
                        <button
                            type='button'
                            onClick={() => remove.mutate(keys[0])}
                            disabled={remove.isPending}
                            className='text-muted-foreground hover:text-destructive shrink-0'
                        >
                            <IconTrash className='size-4' />
                        </button>
                    </div>
                ) : (
                    <Button
                        size='sm'
                        variant='outline'
                        onClick={() => add.mutate()}
                        disabled={!supported || add.isPending}
                    >
                        {add.isPending ? 'Создание...' : 'Добавить passkey'}
                    </Button>
                )}
            </CardContent>
        </Card>
    )
}
