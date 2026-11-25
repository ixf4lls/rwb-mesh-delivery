import type { ButtonHTMLAttributes } from 'react';
import styles from './Button.module.css';

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: 'primary' | 'secondary' | 'ghost';
};

export function Button({ variant = 'primary', type = 'button', className, ...props }: ButtonProps) {
    const classes = [styles.button, styles[variant], className].filter(Boolean).join(' ');

    return <button className={classes} type={type} {...props} />;
}
