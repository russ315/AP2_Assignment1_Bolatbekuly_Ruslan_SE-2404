# AP2 – Order / Payment / Notification (gRPC + EDA)

This repository implements Advanced Programming 2 coursework: Assignment 2 (gRPC between Order and Payment) and Assignment 3 (event-driven notifications via RabbitMQ).

## Assignment 3 – Messaging, reliability, idempotency

### Event flow

1. Client creates an order via REST including `customer_email`.
2. Order service calls Payment over **gRPC** (`AuthorizePayment` carries `customer_email`).
3. Payment persists the row; if status is **Authorized**, it publishes JSON to RabbitMQ exchange `notifications.events`, routing key `payment.completed`, after **publisher confirms** (message is persistent).
4. **Notification service** consumes from durable queue `payment.completed` with **manual ACK**: it acknowledges only after printing the required log line and recording `event_id` in PostgreSQL (`notification_db.processed_events`) for **idempotency**.

### Idempotency strategy

Duplicate deliveries share the same `event_id` (the payment id). Before logging, the consumer checks `processed_events`; if the id exists, it ACKs without printing again. After a successful log line, it inserts the id so retries after a crash may duplicate the log only in a narrow window (documented tradeoff); duplicate broker deliveries after the insert are suppressed.

### ACK logic

- Consumer uses `auto-ack = false`, prefetch 1.
- Success path: log → insert id → `Ack`.
- Invalid JSON / missing `event_id`: treated as poison → `Nack(false, false)` → message goes to the **dead-letter queue** (`payment.completed.dlq`) via queue arguments.
- Transient DB errors: `Nack(false, true)` to requeue.
- Optional DLQ demo: set env `NOTIFICATION_DLQ_DEMO_ORDER_ID` to an `order_id` value; matching messages `Nack(false, false)` into the DLQ.

### Run with Docker

```bash
docker compose up --build
```

Then create an order (example):

```bash
curl -s -X POST http://localhost:8080/orders -H "Content-Type: application/json" \
  -d "{\"customer_id\":\"c1\",\"customer_email\":\"user@example.com\",\"item_name\":\"Book\",\"amount\":9999}"
```

Watch notification logs in the `notification-service` container.

### Environment variables

| Service | Variable |
|--------|----------|
| Payment | `PAYMENT_RABBITMQ_URL` (required for publishing; omit only for local runs without notifications) |
| Notification | `NOTIFICATION_DATABASE_URL`, `NOTIFICATION_RABBITMQ_URL` |

See each service `.env.example`.

---

## Architecture Overview (Assignment 2)

### Before (Assignment 1)
- Order Service (REST) <-> Payment Service (REST)
- Weak contracts, loose typing
- HTTP-based communication

### After (Assignment 2)
- Order Service (REST + gRPC Client) <-> Payment Service (gRPC Server)
- Strong contracts with Protocol Buffers
- gRPC for inter-service communication
- REST maintained for external API access
- Server-side streaming for real-time order tracking

## Services

### Payment Service
- **gRPC Server**: `:50051` (default)
- **HTTP API**: `:8081` (default) - maintained for backward compatibility
- **Features**: Payment authorization, status retrieval
- **Bonus**: gRPC middleware interceptor for request logging

### Order Service
- **HTTP API**: `:8080` (default) - external REST endpoints
- **gRPC Streaming**: `:50052` (default) - real-time order updates
- **gRPC Client**: Connects to Payment Service for payment processing
- **Features**: Order creation, status tracking, real-time streaming

### Streaming Client
- **Purpose**: Demonstrate real-time order status updates
- **Usage**: `go run stream_client.go <order_id>`

## Protocol Buffers

### Proto Repository Structure
```
proto/
  payment.proto    - Payment service definitions
  order.proto      - Order service with streaming
  buf.yaml         - Buf configuration
  buf.gen.yaml     - Code generation config
  .github/workflows/generate.yml  - CI/CD for remote generation
```

### Generated Code
- **Target Repository**: https://github.com/russ315/ap2-proto-gen (when deployed)
- **Local Development**: Generated files copied to each service

## Environment Configuration

### Payment Service (.env)
```bash
PAYMENT_DATABASE_URL=postgres://user:password@localhost:5432/payment_db?sslmode=disable
PAYMENT_GRPC_ADDR=:50051
PAYMENT_HTTP_ADDR=:8081
```

### Order Service (.env)
```bash
ORDER_DATABASE_URL=postgres://user:password@localhost:5432/order_db?sslmode=disable
PAYMENT_GRPC_ADDR=localhost:50051
ORDER_GRPC_ADDR=:50052
ORDER_HTTP_ADDR=:8080
```

### Streaming Client (.env)
```bash
ORDER_GRPC_ADDR=localhost:50052
```

## Running the System

### Prerequisites
- Go 1.22+
- PostgreSQL
- Protocol Buffer compiler (protoc) or Buf

### Step 1: Setup Database
```bash
# Create databases
createdb payment_db
createdb order_db

# Run migrations
cd payment-service/migrations && psql -d payment_db -f migration.sql
cd order-service/migrations && psql -d order_db -f migration.sql
```

### Step 2: Generate Proto Files
```bash
cd proto
buf generate
```

### Step 3: Start Services
```bash
# Terminal 1: Payment Service
cd payment-service
cp .env.example .env
go run cmd/payment-service/main.go

# Terminal 2: Order Service
cd order-service
cp .env.example .env
go run cmd/order-service/main.go

# Terminal 3: Streaming Client (optional)
cd client
cp .env.example .env
go run stream_client.go <order_id>
```

## API Endpoints

### Order Service (REST)
- `POST /orders` - Create new order
- `GET /orders/{id}` - Get order by ID
- `DELETE /orders/{id}` - Cancel order

### Payment Service (gRPC)
- `AuthorizePayment` - Process payment authorization
- `GetPaymentStatus` - Get payment status by order ID

### Order Service (gRPC Streaming)
- `SubscribeToOrderUpdates` - Real-time order status updates

## Key Features Implemented

### 1. Contract-First Development
- Protocol Buffer definitions separate from implementation
- Remote code generation via GitHub Actions
- Clean Architecture preserved

### 2. gRPC Migration
- Payment Service: gRPC server implementation
- Order Service: gRPC client for payment calls
- Proper error handling with gRPC status codes

### 3. Server-Side Streaming
- Real-time order status updates
- Database-driven (not fake time-based updates)
- Multiple subscriber support

### 4. Environment Configuration
- No hardcoded addresses
- Configurable via environment variables
- Separate configs for each service

### 5. Bonus: gRPC Middleware
- Request logging interceptor
- Method name and duration tracking
- Both unary and streaming interceptors

## Architecture Diagram

```
Client (REST)     Streaming Client
       |                 |
       v                 v
+-------------------+   +-------------------+
|  Order Service    |   |  Order Service    |
|  HTTP: :8080      |   |  gRPC: :50052     |
|  gRPC Client      |   |  Streaming Server |
+--------+----------+   +-------------------+
         | gRPC
         v
+-------------------+
| Payment Service   |
| gRPC Server: :50051|
| HTTP: :8081       |
+-------------------+
         |
         v
+-------------------+
| PostgreSQL        |
| payment_db        |
| order_db          |
+-------------------+

Proto Repository:
proto/ -> GitHub Actions -> ap2-proto-gen/ (Generated Code)
```

## Testing

### Create an Order
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "customer_id": "cust123",
    "customer_email": "user@example.com",
    "item_name": "Laptop",
    "amount": 99900
  }'
```

### Stream Order Updates
```bash
cd client
go run stream_client.go <order_id_from_above>
```

### Update Order Status (in another terminal)
```bash
# This will trigger streaming updates
curl -X DELETE http://localhost:8080/orders/<order_id>
```

## Git History

The repository shows the evolution from REST (Assignment 1) to gRPC (Assignment 2):
- Main branch contains the gRPC implementation
- Previous commits show the REST-only version
- Clean separation between domain logic and transport layers

## Links

- **Proto Repository**: https://github.com/russ315/ap2-proto
- **Generated Code Repository**: https://github.com/russ315/ap2-proto-gen
- **Assignment Repository**: Current repository

## Grading Criteria

- **Contract-First Flow** (30%): Full automation via GitHub Actions
- **gRPC Implementation** (30%): Clean Architecture preserved
- **Proto Design & Config** (15%): Robust schema, environment variables
- **Streaming & DB Integration** (15%): Real-time database updates
- **Documentation & Git** (10%): Complete README, working links

## Bonus Features (+10%)

- **gRPC Middleware Interceptor**: Request logging with method name and duration
- **Comprehensive Error Handling**: Proper gRPC status codes
- **Graceful Shutdown**: Proper cleanup of gRPC connections
