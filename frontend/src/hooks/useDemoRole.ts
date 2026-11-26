import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useDispatch } from 'react-redux';
import { meshApi } from '../api/meshApi';
import { demoRoles } from '../app/workspaceRoutes';
import type { DemoRole } from '../app/workspaceRoutes';

export function useDemoRole() {
    const [currentRole, setCurrentRole] = useState<DemoRole>(() => demoRoles.find(({ id }) => id === localStorage.getItem('mesh-demo-role')) ?? demoRoles[2]);
    const [dialogOpen, setDialogOpen] = useState(false);
    const dispatch = useDispatch();
    const navigate = useNavigate();

    function selectRole(role: DemoRole) {
        localStorage.setItem('mesh-demo-role', role.id);
        dispatch(meshApi.util.resetApiState());
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

