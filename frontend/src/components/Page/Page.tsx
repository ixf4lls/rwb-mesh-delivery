import type { ReactNode } from 'react';
import styles from './Page.module.css';

type PageProps = {
    title: string;
    description?: string;
    children?: ReactNode;
};

export function Page({ title, description, children }: PageProps) {
    return (
        <main className={styles.page} id="main-content" tabIndex={-1}>
            <header className={styles.header}>
                <h1 className={styles.title}>{title}</h1>
                {description && <p className={styles.description}>{description}</p>}
            </header>
            {children}
        </main>
    );
}
