import { lazy, Suspense } from 'react';
import { Link, Navigate, Route, Routes } from 'react-router';
import { AppLayout } from '../components/AppLayout/AppLayout';
import { Page } from '../components/Page/Page';
import { PvzWorkspace } from '../components/PvzWorkspace/PvzWorkspace';
import { QueryNotice } from '../components/QueryNotice/QueryNotice';
import { SellerWorkspace } from '../components/SellerWorkspace/SellerWorkspace';
import { workspaceRoutes } from './workspaceRoutes';

const DeliveryWorkspace = lazy(() => import('../components/DeliveryWorkspace/DeliveryWorkspace'));
const AnalyticsWorkspace = lazy(() => import('../components/AnalyticsWorkspace/AnalyticsWorkspace'));

const workspaces = {
    '/pvz': <PvzWorkspace />,
    '/seller': <SellerWorkspace />,
    '/delivery-model': <DeliveryWorkspace />,
    '/analytics': <AnalyticsWorkspace />,
};

export function App() {
    return (
        <Routes>
            <Route element={<AppLayout />}>
                <Route index element={<Navigate to="/delivery-model" replace />} />
                {workspaceRoutes.map((workspace) => (
                    <Route key={workspace.path} path={workspace.path} element={(
                        <Page title={workspace.title}>
                            <Suspense fallback={<QueryNotice message="Открываем раздел…" />}>{workspaces[workspace.path]}</Suspense>
                        </Page>
                    )} />
                ))}
                <Route path="*" element={(
                    <Page title="Страница не найдена">
                        <Link to="/">Открыть модель доставки</Link>
                    </Page>
                )} />
            </Route>
        </Routes>
    );
}
