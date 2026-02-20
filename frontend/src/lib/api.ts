const TOKEN_KEY = 'xkeen_token'

export const getToken = () => localStorage.getItem(TOKEN_KEY)
export const setToken = (token: string) =>
    localStorage.setItem(TOKEN_KEY, token)
export const clearToken = () => localStorage.removeItem(TOKEN_KEY)

class ApiError extends Error {
    status: number
    constructor(message: string, status: number) {
        super(message)
        this.status = status
    }
}

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
    const token = getToken()
    const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        ...(options.headers as Record<string, string>),
    }

    if (token) {
        headers['Authorization'] = `Bearer ${token}`
    }

    const resp = await fetch(url, { ...options, headers })

    if (resp.status === 401) {
        clearToken()
        window.location.href = '/login'
        throw new ApiError('Сессия истекла', 401)
    }

    const data = await resp.json()

    if (!resp.ok) {
        throw new ApiError(data.error || 'Ошибка запроса', resp.status)
    }

    return data as T
}

export const api = {
    get: <T>(url: string) => request<T>(url),
    post: <T>(url: string, body?: unknown) =>
        request<T>(url, {
            method: 'POST',
            body: body ? JSON.stringify(body) : undefined,
        }),
}
