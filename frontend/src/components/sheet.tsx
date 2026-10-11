import {
    useEffect,
    useRef,
    useState,
    type PointerEvent,
    type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'

const EXIT_MS = 220
const DISMISS_DISTANCE = 120
const DISMISS_VELOCITY = 0.6

// Rendered in a portal: the animated page wrapper is transformed, which would trap position: fixed
export function Sheet({
    open,
    onClose,
    title,
    done,
    children,
}: {
    open: boolean
    onClose: () => void
    title: string
    done?: { label: string; onClick: () => void; disabled?: boolean }
    children: ReactNode
}) {
    const [mounted, setMounted] = useState(open)
    const [drag, setDrag] = useState<number | null>(null)
    const [swiped, setSwiped] = useState(false)
    const [entered, setEntered] = useState(false)
    const panel = useRef<HTMLDivElement>(null)
    const start = useRef({ y: 0, t: 0 })
    const closing = mounted && !open

    useEffect(() => {
        if (open) {
            setMounted(true)
            setSwiped(false)
            setEntered(false)
            const timer = setTimeout(() => setEntered(true), 320)
            return () => clearTimeout(timer)
        }
        const timer = setTimeout(() => setMounted(false), EXIT_MS)
        return () => clearTimeout(timer)
    }, [open])

    useEffect(() => {
        if (!open) return
        const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
        document.addEventListener('keydown', onKey)
        const overflow = document.body.style.overflow
        document.body.style.overflow = 'hidden'
        return () => {
            document.removeEventListener('keydown', onKey)
            document.body.style.overflow = overflow
        }
    }, [open, onClose])

    if (!mounted) return null

    const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
        if (window.matchMedia('(min-width: 1024px)').matches) return
        if ((e.target as HTMLElement).closest('button')) return
        start.current = { y: e.clientY, t: performance.now() }
        e.currentTarget.setPointerCapture(e.pointerId)
        setDrag(0)
    }

    const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
        if (drag === null) return
        setDrag(Math.max(0, e.clientY - start.current.y))
    }

    const onPointerUp = (e: PointerEvent<HTMLDivElement>) => {
        if (drag === null) return
        const dy = Math.max(0, e.clientY - start.current.y)
        const velocity = dy / Math.max(1, performance.now() - start.current.t)
        setDrag(null)
        if (dy > DISMISS_DISTANCE || velocity > DISMISS_VELOCITY) {
            setSwiped(true)
            onClose()
        }
    }

    const height = panel.current?.offsetHeight || 1
    const progress = swiped ? 1 : Math.min(1, (drag ?? 0) / height)
    // The enter keyframes run once; a swiped sheet finishes from where the finger left it
    const entering = !entered && !closing
    const exiting = closing && !swiped

    return createPortal(
        <div
            className={cn(
                'fixed inset-0 z-50 flex items-end justify-center lg:items-center',
                closing && 'pointer-events-none',
            )}
        >
            <div
                aria-hidden
                onClick={onClose}
                style={{
                    opacity: 1 - progress,
                    transition:
                        drag === null
                            ? `opacity ${EXIT_MS}ms ease-out`
                            : 'none',
                }}
                className={cn(
                    'absolute inset-0 bg-black/55 backdrop-blur-[2px] fill-mode-forwards',
                    entering &&
                        'motion-safe:animate-in motion-safe:fade-in motion-safe:duration-200',
                    exiting &&
                        'motion-safe:animate-out motion-safe:fade-out motion-safe:duration-200',
                )}
            />
            <div
                ref={panel}
                role='dialog'
                aria-modal='true'
                aria-label={title}
                style={{
                    transform: swiped
                        ? 'translateY(100%)'
                        : drag
                          ? `translateY(${drag}px)`
                          : undefined,
                    transition:
                        drag === null
                            ? `transform ${EXIT_MS}ms cubic-bezier(0.32, 0.72, 0, 1)`
                            : 'none',
                }}
                className={cn(
                    'relative flex max-h-[92dvh] w-full flex-col overflow-y-auto rounded-t-[26px] border-t border-white/10 bg-[#0d0c22] pb-[max(24px,env(safe-area-inset-bottom))] shadow-[0_-8px_40px_rgb(0_0_0/0.5)] fill-mode-forwards lg:max-w-lg lg:rounded-[26px] lg:pb-6',
                    entering &&
                        'motion-safe:animate-in motion-safe:slide-in-from-bottom motion-safe:duration-300 lg:motion-safe:slide-in-from-bottom-0 lg:motion-safe:fade-in lg:motion-safe:zoom-in-95 lg:motion-safe:duration-200',
                    exiting &&
                        'motion-safe:animate-out motion-safe:slide-out-to-bottom motion-safe:duration-200 lg:motion-safe:slide-out-to-bottom-0 lg:motion-safe:fade-out lg:motion-safe:zoom-out-95',
                )}
            >
                <div
                    onPointerDown={onPointerDown}
                    onPointerMove={onPointerMove}
                    onPointerUp={onPointerUp}
                    onPointerCancel={onPointerUp}
                    className='touch-none select-none'
                >
                    <div className='flex justify-center pb-1 pt-2 lg:hidden'>
                        <div className='h-[5px] w-9 rounded-full bg-white/25' />
                    </div>
                    <header className='grid grid-cols-[1fr_auto_1fr] items-center px-4 pb-2 lg:pt-4'>
                        <button
                            type='button'
                            onClick={onClose}
                            className='min-h-11 cursor-pointer justify-self-start px-1 text-[17px] text-tint'
                        >
                            Отмена
                        </button>
                        <h2 className='text-[17px] font-semibold tracking-[-0.43px]'>
                            {title}
                        </h2>
                        {done && (
                            <button
                                type='button'
                                onClick={done.onClick}
                                disabled={done.disabled}
                                className='min-h-11 cursor-pointer justify-self-end px-1 text-[17px] font-semibold text-tint disabled:cursor-default disabled:text-label-tertiary'
                            >
                                {done.label}
                            </button>
                        )}
                    </header>
                </div>
                <div className='flex flex-col gap-5 px-4 pt-2'>{children}</div>
            </div>
        </div>,
        document.body,
    )
}
