import { Link } from 'react-router';
import { workspaceRoutes } from '../../app/workspaceRoutes';
import { Page } from '../Page/Page';
import styles from './Workspaces.module.css';

export function Workspaces() {
    return (
        <Page title="Разделы приложения">
            <nav className={styles.list} aria-label="Разделы приложения">
                {workspaceRoutes.map(({ path, title }) => (
                    <Link className={styles.link} key={path} to={path}>
                        {title}
                    </Link>
                ))}
            </nav>
        </Page>
    );
}
