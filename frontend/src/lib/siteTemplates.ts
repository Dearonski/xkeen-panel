export type SiteTemplate = {
    id: 'tiktok' | 'claude' | 'sentry'
    name: string
    tagline: string
    about: string
    domains: string
    ips: string
    checkURL: string
    countryPattern: string
}

export const siteTemplates: SiteTemplate[] = [
    {
        id: 'tiktok',
        name: 'TikTok',
        tagline: 'Лента без заморозки',
        about: 'Панель откроет TikTok через каждый сервер и спросит, в какой стране он вас видит. TikTok пойдёт только через серверы, где это не страна из исключений автовыбора.',
        domains:
            'tiktok.com tiktokv.com tiktokv.us tiktokcdn.com tiktokcdn-us.com ttwstatic.com byteoversea.com ibytedtos.com ibyteimg.com',
        ips: '',
        checkURL: 'https://www.tiktok.com/explore',
        countryPattern: '"region":"([A-Z]{2})"',
    },
    {
        id: 'claude',
        name: 'Claude',
        tagline: 'Один постоянный IP',
        about: 'Панель найдёт серверы, через которые открывается Claude, и покажет их адреса. Закрепите сайт за одним из них: если адрес сменится, панель заблокирует сайт, а не переключит на другой сервер.',
        domains: 'anthropic.com claude.ai claude.com',
        ips: '160.79.104.0/23 2607:6bc0::/48',
        checkURL: 'https://console.anthropic.com/',
        countryPattern: '',
    },
    {
        id: 'sentry',
        name: 'Sentry',
        tagline: 'Вход без ошибки 403',
        about: 'Sentry не пускает часть выходных адресов. Панель откроет страницу входа через каждый сервер и пустит Sentry только через те, где она открывается.',
        domains: 'sentry.io sentry-cdn.com sentry.dev',
        ips: '',
        checkURL: 'https://sentry.io/auth/login/',
        countryPattern: '',
    },
]
