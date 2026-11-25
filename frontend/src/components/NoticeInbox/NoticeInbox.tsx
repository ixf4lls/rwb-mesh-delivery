import { formatDate } from '../../functions/formatDate';
import { useNoticeInbox } from '../../hooks/useNoticeInbox';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './NoticeInbox.module.css';

const noticeTypes = { route_changed: 'Маршрут изменён', reroute_blocked: 'Маршрут заблокирован', long_storage: 'Длительное хранение', storage_action: 'Решение по хранению' };

export function NoticeInbox({ demoRunId }: { demoRunId?: string }) {
    const view = useNoticeInbox(demoRunId);
    const rows = view.notices.currentData?.pages.flatMap(({ items }) => items.map((notice) => ({ id: notice.id,
        cells: [noticeTypes[notice.type], <div>{notice.message}<small>{notice.resourceId}</small></div>, formatDate(notice.createdAt),
            notice.readAt ? 'Прочитано' : <Button variant="secondary" disabled={view.update.isLoading} onClick={() => view.mark(notice.id)}>Отметить прочитанным</Button>],
    }))) ?? [];

    return (
        <details className={styles.panel} onToggle={(event) => view.setOpen(event.currentTarget.open)}>
            <summary>Уведомления</summary>
            <div className={styles.content}>
                <label><input type="checkbox" checked={view.unread} onChange={(event) => view.setUnread(event.currentTarget.checked)} /> Только непрочитанные</label>
                {view.notices.isError && <QueryNotice error={view.notices.error} onRetry={view.notices.refetch} />}
                {view.update.isError && <QueryNotice error={view.update.error} />}
                {view.notices.isFetching && !view.notices.currentData && <QueryNotice message="Загружаем уведомления…" />}
                {view.notices.currentData && <DataTable title="Журнал уведомлений" columns={['Событие', 'Сообщение', 'Время', 'Статус']}
                    rows={rows} emptyMessage="Уведомлений нет." />}
                {view.notices.hasNextPage && <div><Button variant="secondary" disabled={view.notices.isFetching}
                    onClick={() => void view.notices.fetchNextPage()}>Ещё уведомления</Button></div>}
            </div>
        </details>
    );
}
