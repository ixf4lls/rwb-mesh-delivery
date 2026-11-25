export function operationIdFromLocation(location: string | null, requestUrl: string | undefined, operationId: string) {
    const error = new Error('API не вернул корректную ссылку на статус команды. Повторите проверку той же команды.');
    if (!location || !requestUrl) throw error;

    const request = new URL(requestUrl);
    const target = new URL(location, request);
    const orderPathIndex = request.pathname.lastIndexOf('/orders/');
    const basePath = request.pathname.slice(0, orderPathIndex);
    const expectedPath = `${basePath}/operations/${encodeURIComponent(operationId)}`;

    if (orderPathIndex === -1 || target.origin !== request.origin || target.pathname !== expectedPath || target.search || target.hash) {
        throw error;
    }

    return operationId;
}
