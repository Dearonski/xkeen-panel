import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'

export function useSiteActions() {
    const qc = useQueryClient()
    const refresh = () => qc.invalidateQueries({ queryKey: ['sites'] })

    const create = useMutation({
        mutationFn: (site: {
            domains: string
            ips: string
            check_url: string
        }) => api.post<{ id: number }>('/api/sites', site),
        onSettled: refresh,
    })

    const check = useMutation({
        mutationFn: (id: number) => api.post(`/api/sites/${id}/check`),
        onSettled: refresh,
    })

    const remove = useMutation({
        mutationFn: (id: number) => api.del(`/api/sites/${id}`),
        onSettled: refresh,
    })

    const lock = useMutation({
        mutationFn: ({ id, node }: { id: number; node: string }) =>
            api.post(`/api/sites/${id}/lock`, { node }),
        onSettled: refresh,
    })

    const unlock = useMutation({
        mutationFn: (id: number) => api.del(`/api/sites/${id}/lock`),
        onSettled: refresh,
    })

    return { create, check, remove, lock, unlock }
}
