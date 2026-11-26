import type { ReactNode } from 'react';
import styles from './Page.module.css';

type PageProps = {
    title: string;
    children?: ReactNode;
};

export function Page({ title, children }: PageProps) {
    return (
        <main className={styles.page} id="main-content" tabIndex={-1}>
            <header className={styles.header}>
                <h1 className={styles.title}>{title}</h1>
            </header>
            {children}
        </main>
    );
}
