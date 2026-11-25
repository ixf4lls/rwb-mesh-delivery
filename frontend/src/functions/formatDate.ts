const dateTime = new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' });

export function formatDate(value: string) {
    const date = new Date(value);
    return Number.isFinite(date.getTime()) ? dateTime.format(date) : '—';
}
