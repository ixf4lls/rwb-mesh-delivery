import type { InputHTMLAttributes } from 'react';
import { Button } from '../Button/Button';
import { Icon } from '../Icon/Icon';
import styles from './SearchField.module.css';

type SearchFieldProps = InputHTMLAttributes<HTMLInputElement> & {
    submitLabel: string;
};

export function SearchField({ submitLabel, ...props }: SearchFieldProps) {
    return (
        <div className={styles.field}>
            <input {...props} type="search" className={styles.input} />
            <Button className={styles.submit} variant="ghost" type="submit" aria-label={submitLabel}>
                <Icon name="search" />
            </Button>
        </div>
    );
}
