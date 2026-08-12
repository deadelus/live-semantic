import type { ButtonHTMLAttributes } from 'react'
import './Button.css'

// Matches docs/gui/mockups/ screen 1h's "Boutons" component set exactly
// (5 variants) — primary is the only filled one (accent color), the rest
// are outlined/text-only in increasing subtlety.
export type ButtonVariant = 'primary' | 'secondary' | 'neutral' | 'discrete' | 'danger'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
}

export function Button({ variant = 'neutral', className, ...rest }: ButtonProps) {
  return <button className={`ls-button ls-button--${variant} ${className ?? ''}`} {...rest} />
}
