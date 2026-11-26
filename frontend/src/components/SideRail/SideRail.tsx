import { Link, NavLink } from 'react-router';
import { navigationRoutes } from '../../app/workspaceRoutes';
import { Button } from '../Button/Button';
import { Icon } from '../Icon/Icon';
import styles from './SideRail.module.css';

type SideRailProps = {
    roleId: string;
    onOpenRoleDialog: () => void;
};

export function SideRail({ onOpenRoleDialog, roleId }: SideRailProps) {
    return (
        <div className={styles.rail}>
            <Link to="/" className={styles.logo} aria-label="RWB Mesh Delivery — модель доставки">
                <Icon name="box" />
                <span>mesh</span>
            </Link>
            <nav className={styles.navigation} aria-label="Основное меню">
                {navigationRoutes.filter(({ path }) => roleId === 'dispatcher' || path === (roleId === 'seller' ? '/seller' : '/pvz')).map(({ path, title, shortTitle, icon }) => (
                    <NavLink key={path} to={path} end className={styles.item} aria-label={title}>
                        <Icon name={icon} className={styles.glyph} />
                        <span className={styles.tooltip} aria-hidden="true">{title}</span>
                        <span className={styles.mobileLabel} aria-hidden="true">{shortTitle}</span>
                    </NavLink>
                ))}
            </nav>
            <Button variant="secondary" className={styles.profile} onClick={onOpenRoleDialog}
                aria-label="Выбрать демонстрационную роль" aria-haspopup="dialog">
                <Icon name="profile" />
                <span className={styles.tooltip} aria-hidden="true">Выбрать роль</span>
                <span className={styles.mobileLabel} aria-hidden="true">Роль</span>
            </Button>
        </div>
    );
}


