export function operationIdFromLocation(location: string | null, requestUrl: string | undefined, operationId: string, resourcePath = '/orders') {
    const error = new Error('API не вернул корректную ссылку на статус команды. Повторите проверку той же команды.');
    if (!location || !requestUrl) throw error;

    const request = new URL(requestUrl);
    const target = new URL(location, request);
    const resourceIndex = request.pathname.lastIndexOf(resourcePath);
    const resourceSuffix = request.pathname.slice(resourceIndex);
    const basePath = request.pathname.slice(0, resourceIndex);
    const expectedPath = `${basePath}/operations/${encodeURIComponent(operationId)}`;

    if (resourceIndex === -1 || (resourceSuffix !== resourcePath && !resourceSuffix.startsWith(`${resourcePath}/`))
        || target.origin !== request.origin || target.pathname !== expectedPath || target.search || target.hash) {
        throw error;
    }

    return operationId;
}
