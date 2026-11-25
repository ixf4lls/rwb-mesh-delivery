import type { Workspace } from '../../app/workspaceRoutes';
import { DataTable } from '../DataTable/DataTable';
import { Icon } from '../Icon/Icon';
import { Page } from '../Page/Page';
import styles from './WorkspacePage.module.css';

export function WorkspacePage({ workspace }: { workspace: Workspace }) {
    return (
        <Page title={workspace.title} description={workspace.description}>
            {'columns' in workspace ? (
                <DataTable title={workspace.contentTitle} columns={workspace.columns} rows={[]} emptyMessage={workspace.emptyMessage} />
            ) : (
                <div className={styles.network}>
                    <span className={styles.glyph}><Icon name={workspace.icon} /></span>
                    <h2>{workspace.contentTitle}</h2>
                    <p>{workspace.emptyMessage}</p>
                </div>
            )}
        </Page>
    );
}
