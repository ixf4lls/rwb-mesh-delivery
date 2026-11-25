import { Button } from '../Button/Button';
import styles from './Pagination.module.css';

type PaginationProps = {
    cursor?: string;
    nextCursor?: string;
    busy: boolean;
    onPrevious: () => void;
    onNext: (cursor: string) => void;
};

export function Pagination({ cursor, nextCursor, busy, onPrevious, onNext }: PaginationProps) {
    if (!cursor && !nextCursor) return null;

    return (
        <div className={styles.pagination}>
            <Button variant="secondary" disabled={busy || !cursor} onClick={onPrevious}>Назад</Button>
            <Button variant="secondary" disabled={busy || !nextCursor} onClick={() => nextCursor && onNext(nextCursor)}>Далее</Button>
        </div>
    );
}
