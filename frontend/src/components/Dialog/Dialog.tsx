import { useId } from 'react';
import type { ReactNode } from 'react';
import { useNativeDialog } from '../../hooks/useNativeDialog';
import styles from './Dialog.module.css';

type DialogProps = {
    open: boolean;
    title: string;
    onClose: () => void;
    children: ReactNode;
};

export function Dialog({ open, title, onClose, children }: DialogProps) {
    const titleId = useId();
    const dialogRef = useNativeDialog(open);

    return (
        <dialog ref={dialogRef} className={styles.dialog} aria-labelledby={titleId} onClose={onClose}>
            <h2 id={titleId}>{title}</h2>
            {children}
        </dialog>
    );
}
