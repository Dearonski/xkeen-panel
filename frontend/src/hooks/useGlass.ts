import { useEffect, useId, useState, type RefObject } from 'react'
import { displacementMap } from '@/lib/glass'

export type Lens = {
    id: string
    map: string
    x: number
    y: number
    vw: number
    vh: number
}

let generation = 0

export function useGlass(ref: RefObject<HTMLElement | null>) {
    const baseId = useId().replace(/[^a-zA-Z0-9_-]/g, '')
    const [lens, setLens] = useState<Lens | null>(null)

    useEffect(() => {
        const el = ref.current
        if (!el) return
        if (matchMedia('(prefers-reduced-transparency: reduce)').matches) return

        const measure = () => {
            const rect = el.getBoundingClientRect()
            const w = Math.round(rect.width)
            const h = Math.round(rect.height)
            if (!w || !h) return
            const r = Math.min(
                parseFloat(getComputedStyle(el).borderTopLeftRadius) || 0,
                w / 2,
                h / 2,
            )
            // WebKit caches filter output per id and ignores attribute changes
            generation += 1
            setLens({
                id: `glass${baseId}${generation}`,
                map: displacementMap(w, h, r),
                x: rect.left + el.clientLeft,
                y: rect.top + el.clientTop,
                vw: window.innerWidth,
                vh: window.innerHeight,
            })
        }

        let timer = 0
        const schedule = () => {
            clearTimeout(timer)
            timer = window.setTimeout(measure, 150)
        }

        measure()
        const observer = new ResizeObserver(schedule)
        observer.observe(el)
        window.addEventListener('resize', schedule)

        return () => {
            clearTimeout(timer)
            observer.disconnect()
            window.removeEventListener('resize', schedule)
        }
    }, [ref, baseId])

    return lens
}
