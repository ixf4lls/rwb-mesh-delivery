import type { ReactNode } from 'react';
import styles from './FormField.module.css';

export function FormField({ label, children }: { label: string; children: ReactNode }) {
    return (
        <label className={styles.field}>
            <span>{label}</span>
            {children}
        </label>
    );
}
