import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getToken, clearToken } from '@/lib/api'
import type { Status } from '@/types'

export function useEventSource() {
    const qc = useQueryClient()
    const esRef = useRef<EventSource | null>(null)
    const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
    const attempt = useRef(0)

    useEffect(() => {
        function connect() {
            const token = getToken()
            if (!token) return

            const es = new EventSource(`/api/events?token=${token}`)
            esRef.current = es

            es.onopen = () => {
                attempt.current = 0
            }

            es.addEventListener('status', e => {
                const status: Status = JSON.parse(e.data)
                qc.setQueryData(['status'], status)
            })

            es.addEventListener('log', e => {
                const line: string = JSON.parse(e.data)
                qc.setQueryData<string[]>(['logs'], old => {
                    const logs = old ?? []
                    const updated = [...logs, line]
                    // Хранить максимум 200 строк на клиенте
                    return updated.length > 200
                        ? updated.slice(-200)
                        : updated
                })
            })

            es.addEventListener('restart', e => {
                const { restarting } = JSON.parse(e.data)
                qc.setQueryData<Status>(['status'], old =>
                    old ? { ...old, restarting } : old,
                )
            })

            // Автообновление подписки на сервере — обновить данные в UI
            es.addEventListener('subscription', () => {
                qc.invalidateQueries({ queryKey: ['subscription'] })
                qc.invalidateQueries({ queryKey: ['servers'] })
            })

            es.onerror = () => {
                es.close()
                esRef.current = null

                // Проверить — может токен протух
                if (!getToken()) {
                    clearToken()
                    window.location.href = '/login'
                    return
                }

                // Экспоненциальный backoff: 3с → 6с → 12с ... cap 30с,
                // чтобы долгий обрыв не долбил роутер
                const delay = Math.min(30000, 3000 * 2 ** attempt.current)
                attempt.current += 1
                reconnectTimer.current = setTimeout(connect, delay)
            }
        }

        connect()

        return () => {
            clearTimeout(reconnectTimer.current)
            esRef.current?.close()
            esRef.current = null
        }
    }, [qc])
}
