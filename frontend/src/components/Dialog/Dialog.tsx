import { useId } from 'react';
import type { ReactNode } from 'react';
import { useNativeDialog } from '../../hooks/useNativeDialog';
import styles from './Dialog.module.css';

type DialogProps = {
    open: boolean;
    busy?: boolean;
    title: string;
    onClose: () => void;
    children: ReactNode;
};

export function Dialog({ open, busy = false, title, onClose, children }: DialogProps) {
    const titleId = useId();
    const dialogRef = useNativeDialog(open);

    return (
        <dialog ref={dialogRef} className={styles.dialog} aria-labelledby={titleId} onClose={onClose}
            onCancel={(event) => { if (busy) event.preventDefault(); }}>
            <h2 id={titleId}>{title}</h2>
            {children}
        </dialog>
    );
}
