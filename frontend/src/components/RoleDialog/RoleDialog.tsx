import { demoRoles } from '../../app/workspaceRoutes';
import type { DemoRole } from '../../app/workspaceRoutes';
import { Button } from '../Button/Button';
import { Dialog } from '../Dialog/Dialog';
import { Icon } from '../Icon/Icon';
import styles from './RoleDialog.module.css';

type RoleDialogProps = {
    open: boolean;
    currentRole: DemoRole;
    onClose: () => void;
    onSelect: (role: DemoRole) => void;
};

export function RoleDialog({ open, currentRole, onClose, onSelect }: RoleDialogProps) {
    return (
        <Dialog title="Демонстрационная роль" open={open} onClose={onClose}>
            <p className={styles.description}>Выберите роль, чтобы открыть её рабочий раздел.</p>
            <ul className={styles.roles}>
                {demoRoles.map((role) => (
                    <li key={role.id}>
                        <Button variant="ghost" className={styles.role} onClick={() => onSelect(role)}>
                            <Icon name={role.workspace.icon} />
                            <span className={styles.label}>{role.title}</span>
                            {role.id === currentRole.id && <span className={styles.current}>Текущая</span>}
                        </Button>
                    </li>
                ))}
            </ul>
            <Button variant="secondary" onClick={onClose}>Закрыть</Button>
        </Dialog>
    );
}
