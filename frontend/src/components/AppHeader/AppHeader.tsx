import type { DemoRole } from '../../app/workspaceRoutes';
import { Button } from '../Button/Button';
import { Icon } from '../Icon/Icon';
import styles from './AppHeader.module.css';

type AppHeaderProps = {
    currentRole: DemoRole;
    onOpenRoleDialog: () => void;
};

export function AppHeader({ currentRole, onOpenRoleDialog }: AppHeaderProps) {
    return (
        <header className={styles.header}>
            <Button variant="secondary" className={styles.role} onClick={onOpenRoleDialog} aria-haspopup="dialog"
                aria-label={`Сменить демонстрационную роль. Текущая роль: ${currentRole.title}`}>
                <Icon name="settings" className={styles.settingsIcon} />
                <span className={styles.roleLabel}>
                    <span className={styles.caption}>Деморежим</span>
                    <span>{currentRole.title}</span>
                </span>
            </Button>
        </header>
    );
}
