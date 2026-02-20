import { useCallback, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getToken, clearToken } from '@/lib/api'
import type { Server } from '@/types'

export function useStreamLatency() {
    const qc = useQueryClient()
    const [checking, setChecking] = useState(false)
    const esRef = useRef<EventSource | null>(null)

    const check = useCallback(() => {
        if (esRef.current) return

        const token = getToken()
        if (!token) return

        setChecking(true)
        const es = new EventSource(`/api/servers/check?token=${token}`)
        esRef.current = es

        es.addEventListener('latency', e => {
            const { id, latency_ms } = JSON.parse(e.data)
            qc.setQueryData<Server[]>(['servers'], old =>
                old?.map(s =>
                    s.id === id ? { ...s, latency_ms } : s,
                ),
            )
        })

        es.addEventListener('done', () => {
            setChecking(false)
            es.close()
            esRef.current = null
        })

        es.addEventListener('close', () => {
            es.close()
            esRef.current = null
        })

        es.onerror = () => {
            setChecking(false)
            es.close()
            esRef.current = null

            if (!getToken()) {
                clearToken()
                window.location.href = '/login'
            }
        }
    }, [qc])

    return { check, checking }
}
