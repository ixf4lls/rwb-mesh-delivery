# API operation inventory

Source of truth: `docs/openapi/frontend.yaml` and `internal.yaml` v0.1.0. 104 operations (41 public, 63 internal). Paths are relative to `/api/v1` or `/internal/v1`.

Every row maps to an implemented handler. HTTP verifies JWT, roles/service callers, parameters and request schemas. Gateway delegation requires a signed user context. Domain owners check resource scope, versions, business keys and physical state. Reads merge the immutable snapshot with run overlays; writes are transactional. Async commands persist Operation and job before responding. The worker commits domain changes and completion together. Operation reads enforce subject and run ownership.

The verification column identifies the owner test suite and related HTTP E2E scenarios. It does not claim a separate positive HTTP test for every internal route.

| Owner | Source and mutable state | Verification |
|---|---|---|
| inventory | products, batches, point-occupancy, inventory-reservations, inventory-events | TestImport / TestInventory; HTTP capacity/local/race |
| routing | cities, pickup-points, nodes, trips, quotes, route-holds | TestPlanning; HTTP delivery/mixed/incident |
| orders | orders, routes, order-events, inventory reservations and route holds | TestOrders; HTTP delivery/mixed/restart |
| finance | tariffs, postings, ledger-entries, batch-history | TestFinance; HTTP settlement/partial-return/storage |
| exceptions | returns, incidents, storage-cases, notifications; inventory and ledger operations | TestExceptions; HTTP partial-return/incident/storage |
| demo | global datasets, runs, contexts; scheduled-actions and model clock | TestImport / TestDemo; HTTP run-isolation/clock |

## Public API

| Method and path | operationId / handler | Owner | Access | Success |
|---|---|---|---|---|
| `GET /me` | `getCurrentSession` / `getCurrentSession` | gateway | pvz_operator, seller, dispatcher, demo_admin | 200 |
| `GET /cities` | `listCities` / `listCities` | routing | pvz_operator, seller, dispatcher, demo_admin | 200 |
| `GET /pickup-points` | `listPickupPoints` / `listPickupPoints` | routing | pvz_operator, seller, dispatcher, demo_admin | 200 |
| `GET /pickup-points/{pointId}/capacity` | `getPointCapacity` / `getPointCapacity` | inventory | pvz_operator, dispatcher | 200 |
| `GET /products` | `listProducts` / `listProducts` | inventory | seller, pvz_operator, dispatcher | 200 |
| `POST /products` | `createProduct` / `createProduct` | inventory | seller, dispatcher | 201 |
| `GET /batches` | `listBatches` / `listBatches` | inventory | pvz_operator, seller, dispatcher | 200 |
| `POST /batches` | `receiveBatch` / `receiveBatch` | inventory | pvz_operator, dispatcher | 201 |
| `GET /batches/{batchId}` | `getBatch` / `getBatch` | inventory | pvz_operator, seller, dispatcher | 200 |
| `GET /batches/{batchId}/storage-cost` | `getStorageCost` / `getStorageCost` | inventory | pvz_operator, seller, dispatcher | 200 |
| `POST /quotes` | `createQuote` / `createQuote` | routing | seller, pvz_operator, dispatcher | 201 |
| `POST /orders` | `createOrder` / `createOrder` | orders | seller, pvz_operator, dispatcher | 202 |
| `GET /orders` | `listOrders` / `listOrders` | orders | seller, pvz_operator, dispatcher | 200 |
| `GET /orders/{orderId}` | `getOrder` / `getOrder` | orders | seller, pvz_operator, dispatcher | 200 |
| `GET /orders/{orderId}/route` | `getOrderRoute` / `getOrderRoute` | orders | pvz_operator, dispatcher | 200 |
| `GET /orders/{orderId}/events` | `listOrderEvents` / `listOrderEvents` | orders | pvz_operator, dispatcher | 200 |
| `POST /orders/{orderId}/events` | `recordOrderEvent` / `recordOrderEvent` | orders | pvz_operator, dispatcher | 202 |
| `POST /returns` | `createReturn` / `createReturn` | exceptions | pvz_operator, dispatcher | 202 |
| `GET /returns` | `listReturns` / `listReturns` | exceptions | pvz_operator, dispatcher | 200 |
| `GET /returns/{returnId}` | `getReturn` / `getReturn` | exceptions | pvz_operator, dispatcher | 200 |
| `POST /returns/{returnId}/events` | `recordReturnEvent` / `recordReturnEvent` | exceptions | pvz_operator, dispatcher | 202 |
| `POST /incidents` | `createIncident` / `createIncident` | exceptions | pvz_operator, dispatcher | 201 |
| `GET /incidents` | `listIncidents` / `listIncidents` | exceptions | pvz_operator, dispatcher | 200 |
| `GET /incidents/{incidentId}` | `getIncident` / `getIncident` | exceptions | pvz_operator, dispatcher | 200 |
| `POST /incidents/{incidentId}/reroutes` | `rerouteIncident` / `rerouteIncident` | exceptions | dispatcher | 202 |
| `GET /storage-cases` | `listStorageCases` / `listStorageCases` | exceptions | seller, pvz_operator, dispatcher | 200 |
| `GET /storage-cases/{caseId}` | `getStorageCase` / `getStorageCase` | exceptions | seller, pvz_operator, dispatcher | 200 |
| `POST /storage-cases/{caseId}/decisions` | `decideStorageCase` / `decideStorageCase` | exceptions | dispatcher | 202 |
| `POST /storage-cases/{caseId}/transfer-events` | `recordStorageTransferEvent` / `recordStorageTransferEvent` | exceptions | pvz_operator, dispatcher | 202 |
| `GET /notifications` | `listNotifications` / `listNotifications` | exceptions | seller, pvz_operator, dispatcher | 200 |
| `PUT /notifications/{notificationId}/read` | `readNotification` / `readNotification` | exceptions | seller, pvz_operator, dispatcher | 200 |
| `GET /pickup-points/{pointId}/economics` | `getPointEconomics` / `getPointEconomics` | finance | pvz_operator, dispatcher | 200 |
| `GET /sellers/{sellerId}/economics` | `getSellerEconomics` / `getSellerEconomics` | finance | seller, dispatcher | 200 |
| `GET /sellers/{sellerId}/orders/{orderId}/economics` | `getSellerOrderEconomics` / `getSellerOrderEconomics` | finance | seller, dispatcher | 200 |
| `GET /marketplace/economics` | `getMarketplaceEconomics` / `getMarketplaceEconomics` | finance | dispatcher | 200 |
| `GET /ledger-entries` | `listLedgerEntries` / `listLedgerEntries` | finance | seller, pvz_operator, dispatcher | 200 |
| `GET /demo/datasets` | `listDatasets` / `listDatasets` | demo | demo_admin | 200 |
| `GET /demo/context` | `getDemoContext` / `getDemoContext` | demo | pvz_operator, seller, dispatcher, demo_admin | 200 |
| `POST /demo/runs` | `createDemoRun` / `createDemoRun` | demo | demo_admin | 201 |
| `POST /demo/clock/advances` | `advanceDemoClock` / `advanceDemoClock` | demo | demo_admin | 202 |
| `GET /operations/{operationId}` | `getOperation` / `getOperation` | orders | pvz_operator, seller, dispatcher, demo_admin | 200 |

## Internal API

| Method and path | operationId / handler | Owner | Access | Success |
|---|---|---|---|---|
| `GET /inventory/points/{pointId}/capacity` | `svcGetPointCapacity` / `getPointCapacity` | inventory | gateway, routing, exceptions | 200 |
| `GET /inventory/products` | `svcListProducts` / `listProducts` | inventory | gateway | 200 |
| `POST /inventory/products` | `svcCreateProduct` / `createProduct` | inventory | gateway | 201 |
| `GET /inventory/batches` | `svcListBatches` / `listBatches` | inventory | gateway, exceptions | 200 |
| `POST /inventory/batches` | `svcReceiveBatch` / `receiveBatch` | inventory | gateway | 201 |
| `GET /inventory/batches/{batchId}` | `svcGetBatch` / `getBatch` | inventory | gateway, exceptions | 200 |
| `GET /inventory/batches/{batchId}/storage-cost` | `svcGetStorageCost` / `getStorageCost` | inventory | gateway, exceptions | 200 |
| `POST /inventory/availability/search` | `svcSearchAvailability` / `svcSearchAvailability` | inventory | routing | 200 |
| `PUT /inventory/reservations/{orderId}` | `svcReserveOrderStock` / `svcReserveOrderStock` | inventory | orders | 200 |
| `GET /inventory/reservations/{orderId}` | `svcGetOrderReservation` / `svcGetOrderReservation` | inventory | orders, exceptions | 200 |
| `POST /inventory/reservations/{orderId}/events` | `svcApplyInventoryOrderEvent` / `svcApplyInventoryOrderEvent` | inventory | orders | 200 |
| `POST /inventory/movements/events` | `svcApplyStockMovement` / `svcApplyStockMovement` | inventory | exceptions | 200 |
| `POST /inventory/storage-accruals/search` | `svcListStorageAccruals` / `svcListStorageAccruals` | inventory | exceptions | 200 |
| `POST /orders/orders` | `svcCreateOrder` / `createOrder` | orders | gateway | 202 |
| `GET /orders/orders` | `svcListOrders` / `listOrders` | orders | gateway | 200 |
| `GET /orders/orders/{orderId}` | `svcGetOrder` / `getOrder` | orders | gateway, exceptions, finance | 200 |
| `GET /orders/orders/{orderId}/route` | `svcGetOrderRoute` / `getOrderRoute` | orders | gateway, exceptions | 200 |
| `GET /orders/orders/{orderId}/events` | `svcListOrderEvents` / `listOrderEvents` | orders | gateway | 200 |
| `POST /orders/orders/{orderId}/events` | `svcRecordOrderEvent` / `recordOrderEvent` | orders | gateway, demo | 202 |
| `POST /orders/orders/{orderId}/reroutes` | `svcReplaceOrderRoute` / `svcReplaceOrderRoute` | orders | exceptions | 202 |
| `GET /orders/operations/{operationId}` | `svcGetOrderOperation` / `getOperation` | orders | gateway, exceptions, demo | 200 |
| `GET /routing/cities` | `svcListCities` / `listCities` | routing | gateway | 200 |
| `GET /routing/pickup-points` | `svcListPickupPoints` / `listPickupPoints` | routing | gateway | 200 |
| `GET /routing/nodes/{nodeId}` | `svcGetNode` / `svcGetNode` | routing | inventory, orders, exceptions | 200 |
| `POST /routing/quotes` | `svcCreateQuote` / `createQuote` | routing | gateway | 201 |
| `GET /routing/quotes/{quoteId}` | `svcGetQuoteSnapshot` / `svcGetQuoteSnapshot` | routing | orders | 200 |
| `POST /routing/movement-quotes` | `svcQuoteMovement` / `svcQuoteMovement` | routing | orders, exceptions | 201 |
| `POST /routing/holds` | `svcHoldTripCapacity` / `svcHoldTripCapacity` | routing | orders, exceptions | 201 |
| `POST /routing/holds/{holdId}/events` | `svcUpdateTripHold` / `svcUpdateTripHold` | routing | orders, exceptions | 200 |
| `GET /finance/tariff-versions/effective` | `svcGetEffectiveTariff` / `svcGetEffectiveTariff` | finance | routing, inventory, exceptions | 200 |
| `GET /finance/tariff-versions/{tariffVersionId}` | `svcGetTariffVersion` / `svcGetTariffVersion` | finance | routing, inventory, orders, exceptions | 200 |
| `PUT /finance/postings/{sourceEventId}` | `svcPostLedgerEntries` / `svcPostLedgerEntries` | finance | orders, exceptions | 200 |
| `GET /finance/points/{pointId}/economics` | `svcGetPointEconomics` / `getPointEconomics` | finance | gateway | 200 |
| `GET /finance/sellers/{sellerId}/economics` | `svcGetSellerEconomics` / `getSellerEconomics` | finance | gateway | 200 |
| `GET /finance/sellers/{sellerId}/orders/{orderId}/economics` | `svcGetSellerOrderEconomics` / `getSellerOrderEconomics` | finance | gateway | 200 |
| `GET /finance/marketplace/economics` | `svcGetMarketplaceEconomics` / `getMarketplaceEconomics` | finance | gateway | 200 |
| `GET /finance/entries` | `svcListLedgerEntries` / `listLedgerEntries` | finance | gateway | 200 |
| `POST /exceptions/returns` | `svcCreateReturn` / `createReturn` | exceptions | gateway | 202 |
| `GET /exceptions/returns` | `svcListReturns` / `listReturns` | exceptions | gateway | 200 |
| `GET /exceptions/returns/{returnId}` | `svcGetReturn` / `getReturn` | exceptions | gateway | 200 |
| `POST /exceptions/returns/{returnId}/events` | `svcRecordReturnEvent` / `recordReturnEvent` | exceptions | gateway, demo | 202 |
| `POST /exceptions/incidents` | `svcCreateIncident` / `createIncident` | exceptions | gateway | 201 |
| `GET /exceptions/incidents` | `svcListIncidents` / `listIncidents` | exceptions | gateway | 200 |
| `GET /exceptions/incidents/{incidentId}` | `svcGetIncident` / `getIncident` | exceptions | gateway | 200 |
| `POST /exceptions/incidents/{incidentId}/reroutes` | `svcRerouteIncident` / `rerouteIncident` | exceptions | gateway | 202 |
| `GET /exceptions/storage-cases` | `svcListStorageCases` / `listStorageCases` | exceptions | gateway | 200 |
| `GET /exceptions/storage-cases/{caseId}` | `svcGetStorageCase` / `getStorageCase` | exceptions | gateway | 200 |
| `POST /exceptions/storage-cases/{caseId}/decisions` | `svcDecideStorageCase` / `decideStorageCase` | exceptions | gateway | 202 |
| `POST /exceptions/storage-cases/{caseId}/transfer-events` | `svcRecordStorageTransferEvent` / `recordStorageTransferEvent` | exceptions | gateway, demo | 202 |
| `POST /exceptions/storage-scans` | `svcScanLongStorage` / `svcScanLongStorage` | exceptions | demo, scheduler | 200 |
| `GET /exceptions/notifications` | `svcListNotifications` / `listNotifications` | exceptions | gateway | 200 |
| `PUT /exceptions/notifications/{notificationId}/read` | `svcReadNotification` / `readNotification` | exceptions | gateway | 200 |
| `GET /exceptions/operations/{operationId}` | `svcGetExceptionOperation` / `getOperation` | exceptions | gateway, demo | 200 |
| `GET /demo/datasets` | `svcListDatasets` / `listDatasets` | demo | gateway | 200 |
| `GET /demo/context` | `svcGetDemoContext` / `getDemoContext` | demo | gateway | 200 |
| `POST /demo/runs` | `svcCreateDemoRun` / `createDemoRun` | demo | gateway | 201 |
| `GET /demo/runs/{runId}` | `svcGetDemoRun` / `svcGetDemoRun` | demo | gateway, inventory, orders, routing, finance, exceptions | 200 |
| `POST /demo/clock/advances` | `svcAdvanceDemoClock` / `advanceDemoClock` | demo | gateway | 202 |
| `GET /demo/operations/{operationId}` | `svcGetDemoOperation` / `getOperation` | demo | gateway | 200 |
| `GET /orders/simulated-actions` | `svcListSimulatedOrderActions` / `svcListSimulatedOrderActions` | orders | demo | 200 |
| `GET /exceptions/simulated-actions` | `svcListSimulatedExceptionActions` / `svcListSimulatedExceptionActions` | exceptions | demo | 200 |
| `PUT /orders/orders/{orderId}/segment-blocks/{incidentId}` | `svcBlockOrderSegment` / `svcBlockOrderSegment` | orders | exceptions | 200 |
| `GET /finance/batches/{batchId}/storage-postings` | `svcGetPostedStorage` / `svcGetPostedStorage` | finance | inventory | 200 |
