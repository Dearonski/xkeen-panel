import { useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { getToken, setToken, clearToken, api } from '@/lib/api'

export const useAuth = () => {
    const navigate = useNavigate()

    const login = useCallback(
        async (username: string, password: string, totpCode: string) => {
            const data = await api.post<{ token: string }>('/api/auth/login', {
                username,
                password,
                totp_code: totpCode,
            })
            setToken(data.token)
            navigate('/')
        },
        [navigate],
    )

    const logout = useCallback(() => {
        clearToken()
        navigate('/login')
    }, [navigate])

    return {
        token: getToken(),
        isAuthenticated: !!getToken(),
        login,
        logout,
    }
}
