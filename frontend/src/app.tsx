import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { SetupPage } from '@/pages/setupPage'
import { LoginPage } from '@/pages/loginPage'
import { DashboardPage } from '@/pages/dashboardPage'
import { ProtectedRoute } from '@/components/protectedRoute'
import type { AuthStatus } from '@/types'

export function App() {
    const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null)
    const [loading, setLoading] = useState(true)

    useEffect(() => {
        fetch('/api/auth/status')
            .then(r => r.json())
            .then(data => setAuthStatus(data))
            .catch(() => setAuthStatus({ setup_required: false }))
            .finally(() => setLoading(false))
    }, [])

    if (loading) {
        return (
            <div className='min-h-screen flex items-center justify-center'>
                <div className='text-muted-foreground text-sm'>Загрузка...</div>
            </div>
        )
    }

    return (
        <BrowserRouter>
            <Routes>
                {authStatus?.setup_required ? (
                    <>
                        <Route path='/setup' element={<SetupPage />} />
                        <Route path='*' element={<Navigate to='/setup' />} />
                    </>
                ) : (
                    <>
                        <Route path='/login' element={<LoginPage />} />
                        <Route
                            path='/'
                            element={
                                <ProtectedRoute>
                                    <DashboardPage />
                                </ProtectedRoute>
                            }
                        />
                        <Route path='*' element={<Navigate to='/' />} />
                    </>
                )}
            </Routes>
        </BrowserRouter>
    )
}
