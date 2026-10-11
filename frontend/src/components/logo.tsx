export function Logo({ size = 32 }: { size?: number }) {
    return (
        <svg
            width={size}
            height={size}
            viewBox='0 0 512 512'
            fill='none'
            role='img'
            aria-label='XKeen'
        >
            <path
                d='M256 42L72 130v120c0 110 78 213 184 240 106-27 184-130 184-240V130L256 42z'
                stroke='#818cf8'
                strokeWidth='36'
                strokeLinejoin='round'
            />
            <polygon
                points='276,120 208,272 252,272 236,410 304,248 260,248'
                fill='#818cf8'
            />
        </svg>
    )
}
