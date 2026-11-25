export function apiErrorMessage(error: unknown) {
    if (error instanceof Error) return error.message;

    if (error && typeof error === 'object') {
        if ('status' in error && (error.status === 401 || error.status === 403)) {
            return 'Сервер пока не предоставляет доступ к этим данным.';
        }
        if ('data' in error && error.data && typeof error.data === 'object') {
            const problem = error.data;
            if ('code' in problem && problem.code === 'stale_version') {
                return 'Статус заказа изменился. Откройте действие заново после обновления списка.';
            }
            if ('detail' in problem && typeof problem.detail === 'string') return problem.detail;
            if ('title' in problem && typeof problem.title === 'string') return problem.title;
        }
        if ('status' in error) {
            if (error.status === 404) return 'Запрошенные данные недоступны.';
            if (error.status === 'FETCH_ERROR' || error.status === 'TIMEOUT_ERROR' || error.status === 'PARSING_ERROR') {
                return 'API не ответил ожидаемым образом. Проверьте, что сервер запущен и адрес API настроен.';
            }
        }
    }

    return 'Не удалось выполнить запрос. Повторите попытку.';
}
