import { apiErrorMessage } from '../../functions/apiErrorMessage';
import { Button } from '../Button/Button';
import styles from './QueryNotice.module.css';

type QueryNoticeProps = {
    message?: string;
    error?: unknown;
    onRetry?: () => void;
};

export function QueryNotice({ message, error, onRetry }: QueryNoticeProps) {
    return (
        <div className={styles.notice}>
            <p role={error ? 'alert' : 'status'}>{error ? apiErrorMessage(error) : message}</p>
            {onRetry && <Button variant="secondary" onClick={onRetry}>Повторить</Button>}
        </div>
    );
}
