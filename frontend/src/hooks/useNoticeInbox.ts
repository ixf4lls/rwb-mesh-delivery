import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import { exceptionApi } from '../api/exceptionApi';

export function useNoticeInbox(demoRunId?: string) {
    const [open, setOpen] = useState(false);
    const [unread, setUnread] = useState(false);
    const notices = exceptionApi.useListNoticesInfiniteQuery(open ? { demoRunId, unread: unread || undefined } : skipToken,
        { pollingInterval: 15000 });
    const [mark, update] = exceptionApi.useReadNoticeMutation();

    return { open, setOpen, unread, setUnread, notices, update, mark: (notificationId: string) => void mark({ notificationId, demoRunId }) };
}
