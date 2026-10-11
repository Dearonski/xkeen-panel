import { type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { IconChevronLeft, IconChevronRight } from '@tabler/icons-react'
import { cn } from '@/lib/utils'

export function PageTitle({
    title,
    subtitle,
    action,
    back,
}: {
    title: string
    subtitle?: ReactNode
    action?: ReactNode
    back?: { to: string; label: string }
}) {
    return (
        <header className='flex flex-col gap-2 px-1'>
            {back && (
                <Link
                    to={back.to}
                    className='-ml-1 flex min-h-11 w-fit items-center gap-0.5 text-[17px] tracking-[-0.43px] text-tint'
                >
                    <IconChevronLeft className='size-6' stroke={2.2} />
                    {back.label}
                </Link>
            )}
            <div className='flex items-end justify-between gap-3'>
                <div className='min-w-0'>
                    <h1 className='text-[34px] font-bold leading-tight tracking-[0.37px]'>
                        {title}
                    </h1>
                    {subtitle && (
                        <p className='mt-0.5 text-[15px] tracking-[-0.24px] text-muted-foreground'>
                            {subtitle}
                        </p>
                    )}
                </div>
                {action}
            </div>
        </header>
    )
}

export function Group({
    title,
    footer,
    children,
    className,
}: {
    title?: ReactNode
    footer?: ReactNode
    children: ReactNode
    className?: string
}) {
    return (
        <section className={cn('flex min-w-0 flex-col gap-1.5', className)}>
            {title && (
                <h2 className='px-4 text-[13px] uppercase tracking-[-0.08px] text-muted-foreground'>
                    {title}
                </h2>
            )}
            <div className='divide-y divide-white/10 rounded-card bg-card px-4'>
                {children}
            </div>
            {footer && (
                <p className='px-4 text-[13px] leading-snug text-muted-foreground'>
                    {footer}
                </p>
            )}
        </section>
    )
}

export function IconTile({
    color,
    children,
}: {
    color: string
    children: ReactNode
}) {
    return (
        <span
            className='flex size-[30px] shrink-0 items-center justify-center rounded-[8px] text-white'
            style={{ background: color }}
        >
            {children}
        </span>
    )
}

const rowClass =
    'flex min-h-[52px] w-full items-center gap-3 py-2 text-left text-[17px] tracking-[-0.43px]'

export function Row({
    icon,
    label,
    hint,
    detail,
    trailing,
    to,
    onClick,
    disabled,
    tone,
}: {
    icon?: ReactNode
    label: ReactNode
    hint?: ReactNode
    detail?: ReactNode
    trailing?: ReactNode
    to?: string
    onClick?: () => void
    disabled?: boolean
    tone?: 'tint' | 'destructive'
}) {
    const body = (
        <>
            {icon}
            <span className='min-w-0 flex-1'>
                <span
                    className={cn(
                        'block',
                        tone === 'tint' && 'text-tint',
                        tone === 'destructive' && 'text-destructive',
                    )}
                >
                    {label}
                </span>
                {hint && (
                    <span className='mt-0.5 block text-[13px] tracking-normal text-muted-foreground'>
                        {hint}
                    </span>
                )}
            </span>
            {detail && (
                <span className='shrink-0 text-muted-foreground'>{detail}</span>
            )}
            {trailing}
            {to && (
                <IconChevronRight
                    className='size-4 shrink-0 text-label-tertiary'
                    stroke={2.6}
                />
            )}
        </>
    )

    if (to) {
        return (
            <Link to={to} className={rowClass}>
                {body}
            </Link>
        )
    }
    if (onClick) {
        return (
            <button
                type='button'
                onClick={onClick}
                disabled={disabled}
                className={cn(
                    rowClass,
                    'cursor-pointer select-none disabled:opacity-50',
                )}
            >
                {body}
            </button>
        )
    }
    return <div className={rowClass}>{body}</div>
}

export function Switch({
    checked,
    onChange,
    label,
    disabled,
}: {
    checked: boolean
    onChange: (next: boolean) => void
    label: string
    disabled?: boolean
}) {
    return (
        <button
            type='button'
            role='switch'
            aria-checked={checked}
            aria-label={label}
            disabled={disabled}
            onClick={() => onChange(!checked)}
            className={cn(
                'flex h-[31px] w-[51px] shrink-0 cursor-pointer items-center rounded-full p-0.5 transition-colors duration-200 disabled:opacity-50',
                checked
                    ? 'justify-end bg-primary'
                    : 'justify-start bg-[rgb(120_120_128/0.32)]',
            )}
        >
            <span className='size-[27px] rounded-full bg-white shadow-[0_3px_8px_rgb(0_0_0/0.25)]' />
        </button>
    )
}

export function Segmented<T extends string>({
    options,
    value,
    onChange,
    label,
}: {
    options: { value: T; label: string }[]
    value: T
    onChange: (value: T) => void
    label: string
}) {
    return (
        <div
            role='radiogroup'
            aria-label={label}
            className='grid gap-0.5 rounded-xl bg-fill p-0.5'
            style={{
                gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))`,
            }}
        >
            {options.map(o => (
                <button
                    key={o.value}
                    type='button'
                    role='radio'
                    aria-checked={o.value === value}
                    onClick={() => onChange(o.value)}
                    className={cn(
                        'min-h-9 cursor-pointer rounded-[10px] px-2 text-[15px] tracking-[-0.24px]',
                        o.value === value
                            ? 'bg-[#4f46e5] font-semibold shadow-sm'
                            : 'font-medium',
                    )}
                >
                    {o.label}
                </button>
            ))}
        </div>
    )
}

export function CircleButton({
    label,
    onClick,
    children,
    disabled,
}: {
    label: string
    onClick: () => void
    children: ReactNode
    disabled?: boolean
}) {
    return (
        <button
            type='button'
            aria-label={label}
            onClick={onClick}
            disabled={disabled}
            className='glass glass-tint flex size-11 shrink-0 cursor-pointer items-center justify-center rounded-full disabled:opacity-50'
        >
            {children}
        </button>
    )
}
