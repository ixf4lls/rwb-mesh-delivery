import { useState } from 'react';
import { useNavigate } from 'react-router';
import { demoRoles } from '../app/workspaceRoutes';
import type { DemoRole } from '../app/workspaceRoutes';

export function useDemoRole() {
    const [currentRole, setCurrentRole] = useState<DemoRole>(demoRoles[0]);
    const [dialogOpen, setDialogOpen] = useState(false);
    const navigate = useNavigate();

    function selectRole(role: DemoRole) {
        setCurrentRole(role);
        setDialogOpen(false);
        navigate(role.workspace.path);
    }

    return {
        currentRole,
        dialogOpen,
        openDialog: () => setDialogOpen(true),
        closeDialog: () => setDialogOpen(false),
        selectRole,
    };
}
