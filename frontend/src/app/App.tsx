import { Link, Route, Routes } from 'react-router';
import { AppLayout } from '../components/AppLayout/AppLayout';
import { Page } from '../components/Page/Page';
import { WorkspacePage } from '../components/WorkspacePage/WorkspacePage';
import { Workspaces } from '../components/Workspaces/Workspaces';
import { workspaceRoutes } from './workspaceRoutes';

export function App() {
    return (
        <Routes>
            <Route element={<AppLayout />}>
                <Route index element={<Workspaces />} />
                {workspaceRoutes.map((workspace) => (
                    <Route key={workspace.path} path={workspace.path} element={<WorkspacePage workspace={workspace} />} />
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
