import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { Server, Status } from '@/types'

export function useServerActions() {
    const qc = useQueryClient()
    const refreshAll = () => {
        qc.invalidateQueries({ queryKey: ['servers'] })
        qc.invalidateQueries({ queryKey: ['status'] })
        qc.invalidateQueries({ queryKey: ['pool'] })
        qc.invalidateQueries({ queryKey: ['subscription'] })
    }

    const select = useMutation({
        mutationFn: (id: number) =>
            api.post<{ restarting?: boolean }>('/api/servers/select', { id }),
        onMutate: id =>
            qc.setQueryData<Server[]>(['servers'], old =>
                old?.map(s => ({ ...s, active: s.id === id })),
            ),
        onSuccess: res => {
            if (res.restarting) {
                qc.setQueryData<Status>(['status'], old =>
                    old ? { ...old, restarting: true } : old,
                )
            }
        },
        onSettled: refreshAll,
    })

    const auto = useMutation({
        mutationFn: () => api.post('/api/pool/auto'),
        onSettled: refreshAll,
    })

    // Turning auto off keeps whatever node traffic uses now, only as a manual pick
    const pin = useMutation({
        mutationFn: (tag: string) => api.post('/api/pool/pin', { tag }),
        onSettled: refreshAll,
    })

    const refreshSubscription = useMutation({
        mutationFn: () => api.post('/api/subscription/refresh'),
        onSettled: refreshAll,
    })

    const setSubscription = useMutation({
        mutationFn: (url: string) => api.post('/api/subscription', { url }),
        onSettled: refreshAll,
    })

    const setCountry = useMutation({
        mutationFn: ({ id, country }: { id: number; country: string }) =>
            api.post('/api/servers/country', { id, country }),
        onSettled: () => qc.invalidateQueries({ queryKey: ['servers'] }),
    })

    return {
        select,
        auto,
        pin,
        refreshSubscription,
        setSubscription,
        setCountry,
    }
}
