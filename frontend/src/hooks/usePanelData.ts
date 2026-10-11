import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type {
    DevicesResponse,
    PoolStatus,
    Server,
    Status,
    SubscriptionInfo,
    UpdateState,
} from '@/types'

export function useStatus() {
    return useQuery({
        queryKey: ['status'],
        queryFn: () => api.get<Status>('/api/status'),
    })
}

export function useServers() {
    return useQuery({
        queryKey: ['servers'],
        queryFn: () =>
            api
                .get<{ servers: Server[] }>('/api/servers')
                .then(d => d.servers ?? []),
    })
}

export function usePool() {
    return useQuery({
        queryKey: ['pool'],
        queryFn: () => api.get<PoolStatus>('/api/pool'),
    })
}

export function useSubscription() {
    return useQuery({
        queryKey: ['subscription'],
        queryFn: () => api.get<SubscriptionInfo>('/api/subscription'),
    })
}

export function useLogs() {
    return useQuery({
        queryKey: ['logs'],
        queryFn: () =>
            api
                .get<{ lines: string[] }>('/api/logs?lines=200')
                .then(d => d.lines ?? []),
    })
}

export function useDevices() {
    return useQuery({
        queryKey: ['devices'],
        queryFn: () => api.get<DevicesResponse>('/api/devices'),
        refetchInterval: 60000,
    })
}

export function useUpdate() {
    return useQuery({
        queryKey: ['update'],
        queryFn: () => api.get<UpdateState>('/api/update'),
    })
}
