import { Link, Route, Routes } from 'react-router';
import { Page } from '../components/Page/Page';
import { Workspaces } from '../components/Workspaces/Workspaces';
import { workspaceRoutes } from './workspaceRoutes';

export function App() {
    return (
        <Routes>
            <Route index element={<Workspaces />} />
            {workspaceRoutes.map(({ path, title }) => (
                <Route key={path} path={path} element={<Page title={title} />} />
            ))}
            <Route path="*" element={(
                <Page title="Страница не найдена">
                    <Link to="/">К разделам приложения</Link>
                </Page>
            )} />
        </Routes>
    );
}
