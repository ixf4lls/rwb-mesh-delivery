import { Outlet } from 'react-router';
import { useDemoRole } from '../../hooks/useDemoRole';
import { RoleDialog } from '../RoleDialog/RoleDialog';
import { SideRail } from '../SideRail/SideRail';
import styles from './AppLayout.module.css';

export function AppLayout() {
    const role = useDemoRole();

    return (
        <div className={styles.layout}>
            <a className={styles.skip} href="#main-content">Перейти к содержимому</a>
            <SideRail onOpenRoleDialog={role.openDialog} />
            <div className={styles.content}>
                <Outlet />
            </div>
            <RoleDialog open={role.dialogOpen} currentRole={role.currentRole} onClose={role.closeDialog} onSelect={role.selectRole} />
        </div>
    );
}
