import {
    useLayoutEffect,
    useState,
    type CSSProperties,
    type RefObject,
} from 'react'

// Follows the active item of a menu so the highlight can glide between them
export function useIndicator(
    ref: RefObject<HTMLElement | null>,
    index: number,
): CSSProperties {
    const [box, setBox] = useState<DOMRect | null>(null)
    const [animate, setAnimate] = useState(false)

    useLayoutEffect(() => {
        const nav = ref.current
        if (!nav) return

        const measure = () => {
            const item = nav.querySelectorAll<HTMLElement>('[data-tab]')[index]
            setBox(
                item
                    ? new DOMRect(
                          item.offsetLeft,
                          item.offsetTop,
                          item.offsetWidth,
                          item.offsetHeight,
                      )
                    : null,
            )
        }

        measure()
        const observer = new ResizeObserver(measure)
        observer.observe(nav)
        return () => observer.disconnect()
    }, [ref, index])

    // The first placement must not slide in from the corner
    useLayoutEffect(() => {
        if (!box || animate) return
        const frame = requestAnimationFrame(() => setAnimate(true))
        return () => cancelAnimationFrame(frame)
    }, [box, animate])

    if (!box) return { opacity: 0 }

    return {
        transform: `translate(${box.x}px, ${box.y}px)`,
        width: box.width,
        height: box.height,
        transition: animate
            ? 'transform 320ms cubic-bezier(0.34, 1.18, 0.64, 1), width 320ms cubic-bezier(0.34, 1.18, 0.64, 1), height 320ms cubic-bezier(0.34, 1.18, 0.64, 1)'
            : 'none',
    }
}
