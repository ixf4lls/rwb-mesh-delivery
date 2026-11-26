import { NoticeInbox } from '../NoticeInbox/NoticeInbox';
import { Navigate, Outlet, useLocation } from 'react-router';
import { useDemoRole } from '../../hooks/useDemoRole';
import { RoleDialog } from '../RoleDialog/RoleDialog';
import { SideRail } from '../SideRail/SideRail';
import styles from './AppLayout.module.css';

export function AppLayout() {
    const role = useDemoRole();

    const location = useLocation();
    const allowed = role.currentRole.id === 'dispatcher' || location.pathname === role.currentRole.workspace.path;

    return (
        <div className={styles.layout}>
            <a className={styles.skip} href="#main-content">Перейти к содержимому</a>
            <SideRail roleId={role.currentRole.id} onOpenRoleDialog={role.openDialog} />
            <div className={styles.content}>
                <NoticeInbox />
                {allowed ? <Outlet key={role.currentRole.id} /> : <Navigate to={role.currentRole.workspace.path} replace />}
            </div>
            <RoleDialog open={role.dialogOpen} currentRole={role.currentRole} onClose={role.closeDialog} onSelect={role.selectRole} />
        </div>
    );
}

