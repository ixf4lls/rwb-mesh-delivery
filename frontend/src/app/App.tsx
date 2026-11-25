import { lazy, Suspense } from 'react';
import { Link, Route, Routes } from 'react-router';
import { AppLayout } from '../components/AppLayout/AppLayout';
import { Page } from '../components/Page/Page';
import { PvzWorkspace } from '../components/PvzWorkspace/PvzWorkspace';
import { QueryNotice } from '../components/QueryNotice/QueryNotice';
import { SellerWorkspace } from '../components/SellerWorkspace/SellerWorkspace';
import { Workspaces } from '../components/Workspaces/Workspaces';
import { workspaceRoutes } from './workspaceRoutes';

const DeliveryWorkspace = lazy(() => import('../components/DeliveryWorkspace/DeliveryWorkspace'));

export function App() {
    return (
        <Routes>
            <Route element={<AppLayout />}>
                <Route index element={<Workspaces />} />
                {workspaceRoutes.map((workspace) => (
                    <Route key={workspace.path} path={workspace.path} element={(
                        <Page title={workspace.title} description={workspace.description}>
                            {workspace.path === '/pvz' ? <PvzWorkspace /> : workspace.path === '/seller' ? <SellerWorkspace /> : (
                                <Suspense fallback={<QueryNotice message="Открываем модель доставки…" />}><DeliveryWorkspace /></Suspense>
                            )}
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
