import { formatDate } from '../../functions/formatDate';
import { useDemoScope } from '../../hooks/useDemoScope';
import { useNoticeInbox } from '../../hooks/useNoticeInbox';
import { Button } from '../Button/Button';
import { Dialog } from '../Dialog/Dialog';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './NoticeInbox.module.css';

const noticeTypes = { route_changed: 'Маршрут изменён', reroute_blocked: 'Маршрут заблокирован', long_storage: 'Длительное хранение', storage_action: 'Решение по хранению' };

export function NoticeInbox() {
    const { demoRunId, scopeReady } = useDemoScope();
    const view = useNoticeInbox(demoRunId);
    const notices = view.notices.currentData?.pages.flatMap(({ items }) => items) ?? [];

    return (
        <div className={styles.toolbar}>
            <Button variant="secondary" className={styles.trigger} aria-label="Открыть уведомления" aria-haspopup="dialog"
                disabled={!scopeReady} onClick={() => view.setOpen(true)}>
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4" />
                </svg>
                <span className={styles.tooltip} aria-hidden="true">Уведомления</span>
            </Button>
            <Dialog title="Уведомления" open={view.open} onClose={() => view.setOpen(false)}>
                <Button variant="ghost" className={styles.close} aria-label="Закрыть уведомления" onClick={() => view.setOpen(false)}>✕</Button>
                <div className={styles.content}>
                    <div className={styles.filters}>
                        <label><input type="checkbox" checked={view.unread} onChange={(event) => view.setUnread(event.currentTarget.checked)} /> Только непрочитанные</label>
                    </div>
                    {view.notices.isError && <QueryNotice error={view.notices.error} onRetry={view.notices.refetch} />}
                    {view.update.isError && <QueryNotice error={view.update.error} />}
                    {view.notices.isFetching && !view.notices.currentData && <QueryNotice message="Загружаем уведомления…" />}
                    {view.notices.currentData && notices.length === 0 && <div className={styles.empty}>Новых событий нет</div>}
                    <ul className={styles.list}>
                        {notices.map((notice) => (
                            <li key={notice.id} className={styles.notice} data-unread={!notice.readAt}>
                                <div className={styles.heading}><strong>{noticeTypes[notice.type]}</strong>{!notice.readAt && <span className={styles.dot} aria-label="Не прочитано" />}</div>
                                <p>{notice.message}</p>
                                <small>{notice.resourceId} · {formatDate(notice.createdAt)}</small>
                                {!notice.readAt && <Button variant="ghost" disabled={view.update.isLoading} onClick={() => view.mark(notice.id)}>Отметить прочитанным</Button>}
                            </li>
                        ))}
                    </ul>
                    {view.notices.hasNextPage && <Button variant="secondary" disabled={view.notices.isFetching} onClick={() => void view.notices.fetchNextPage()}>Ещё уведомления</Button>}
                </div>
            </Dialog>
        </div>
    );
}
