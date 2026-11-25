import { Link, Route, Routes } from 'react-router';
import { AppLayout } from '../components/AppLayout/AppLayout';
import { Page } from '../components/Page/Page';
import { PvzWorkspace } from '../components/PvzWorkspace/PvzWorkspace';
import { SellerWorkspace } from '../components/SellerWorkspace/SellerWorkspace';
import { WorkspacePage } from '../components/WorkspacePage/WorkspacePage';
import { Workspaces } from '../components/Workspaces/Workspaces';
import { workspaceRoutes } from './workspaceRoutes';

export function App() {
    return (
        <Routes>
            <Route element={<AppLayout />}>
                <Route index element={<Workspaces />} />
                {workspaceRoutes.map((workspace) => (
                    <Route key={workspace.path} path={workspace.path} element={workspace.path === '/delivery-model' ? (
                        <WorkspacePage workspace={workspace} />
                    ) : (
                        <Page title={workspace.title} description={workspace.description}>
                            {workspace.path === '/pvz' ? <PvzWorkspace /> : <SellerWorkspace />}
                        </Page>
                    )} />
                ))}
                <Route path="*" element={(
                    <Page title="Страница не найдена">
                        <Link to="/">Вернуться к обзору</Link>
                    </Page>
                )} />
            </Route>
        </Routes>
    );
}
