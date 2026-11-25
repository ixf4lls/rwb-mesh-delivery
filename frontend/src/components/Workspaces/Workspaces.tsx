import { Link } from 'react-router';
import { workspaceRoutes } from '../../app/workspaceRoutes';
import { Icon } from '../Icon/Icon';
import { Page } from '../Page/Page';
import styles from './Workspaces.module.css';

export function Workspaces() {
    return (
        <Page title="Управление доставкой" description="Кабинеты ПВЗ и продавца, маршруты и сравнение вариантов доставки.">
            <div className={styles.list}>
                {workspaceRoutes.map(({ path, title, icon, description }) => (
                    <Link className={styles.card} key={path} to={path}>
                        <span className={styles.glyph}><Icon name={icon} /></span>
                        <h2>{title}</h2>
                        <p>{description}</p>
                        <span className={styles.action}>Открыть <span aria-hidden="true">→</span></span>
                    </Link>
                ))}
            </div>
        </Page>
    );
}
