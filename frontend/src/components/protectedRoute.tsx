import { Navigate } from 'react-router-dom'
import { type ReactNode } from 'react'
import { getToken } from '@/lib/api'

export function ProtectedRoute({ children }: { children: ReactNode }) {
    if (!getToken()) {
        return <Navigate to='/login' replace />
    }
    return <>{children}</>
}
