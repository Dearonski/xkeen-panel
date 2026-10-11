import { useRef, type ReactNode } from 'react'
import { useGlass } from '@/hooks/useGlass'
import { cn } from '@/lib/utils'

const CHANNELS = [
    '1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0',
    '0 0 0 0 0 0 1 0 0 0 0 0 0 0 0 0 0 0 1 0',
    '0 0 0 0 0 0 0 0 0 0 0 0 1 0 0 0 0 0 1 0',
]

// Refracts a copy of the fixed page background: backdrop-filter: url() paints nothing in Safari
export function Glass({
    children,
    className,
    strength = 22,
}: {
    children: ReactNode
    className?: string
    strength?: number
}) {
    const ref = useRef<HTMLDivElement>(null)
    const lens = useGlass(ref)

    return (
        <div
            ref={ref}
            className={cn('glass relative isolate overflow-hidden', className)}
        >
            {lens && (
                <>
                    <svg width='0' height='0' aria-hidden className='absolute'>
                        <filter
                            id={lens.id}
                            x='0'
                            y='0'
                            width='1'
                            height='1'
                            colorInterpolationFilters='sRGB'
                        >
                            <feGaussianBlur
                                in='SourceGraphic'
                                stdDeviation='2'
                                edgeMode='duplicate'
                                result='blurred'
                            />
                            <feImage
                                href={lens.map}
                                preserveAspectRatio='none'
                                result='map'
                            />
                            {[1, 1.06, 1.12].map((k, i) => (
                                <feDisplacementMap
                                    key={k}
                                    in='blurred'
                                    in2='map'
                                    scale={strength * 2 * k}
                                    xChannelSelector='R'
                                    yChannelSelector='G'
                                    result={`shift${i}`}
                                />
                            ))}
                            {CHANNELS.map((values, i) => (
                                <feColorMatrix
                                    key={values}
                                    in={`shift${i}`}
                                    type='matrix'
                                    values={values}
                                    result={`channel${i}`}
                                />
                            ))}
                            <feComposite
                                in='channel0'
                                in2='channel1'
                                operator='arithmetic'
                                k2='1'
                                k3='1'
                                result='rg'
                            />
                            <feComposite
                                in='rg'
                                in2='channel2'
                                operator='arithmetic'
                                k2='1'
                                k3='1'
                                result='rgb'
                            />
                            <feColorMatrix
                                in='rgb'
                                type='saturate'
                                values='1.6'
                            />
                        </filter>
                    </svg>
                    <div
                        aria-hidden
                        className='glass-lens bg-page pointer-events-none absolute inset-0 -z-10 rounded-[inherit] opacity-80'
                        style={{
                            filter: `url(#${lens.id})`,
                            backgroundSize: `${lens.vw}px ${lens.vh}px`,
                            backgroundPosition: `${-lens.x}px ${-lens.y}px`,
                        }}
                    />
                </>
            )}
            <div
                aria-hidden
                className='glass-tint pointer-events-none absolute inset-0 -z-10 rounded-[inherit]'
            />
            {children}
        </div>
    )
}
