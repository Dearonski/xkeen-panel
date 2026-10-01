import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs))
}

export function codeToFlag(cc?: string) {
    if (!cc || cc.length !== 2) return ''
    const base = 0x1f1e6
    const up = cc.toUpperCase()
    return String.fromCodePoint(
        base + up.charCodeAt(0) - 65,
        base + up.charCodeAt(1) - 65,
    )
}

// Subscription names often already start with a flag emoji
export function flagFor(name: string, country?: string) {
    const nameHasFlag = /\p{Regional_Indicator}\p{Regional_Indicator}/u.test(
        name,
    )
    return nameHasFlag ? '' : codeToFlag(country)
}

export function latencyClass(ms: number) {
    if (ms < 200) return 'text-emerald-400'
    if (ms < 500) return 'text-amber-400'
    return 'text-red-400'
}
