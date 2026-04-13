# Database Setup Instructions

## Prerequisites
- PostgreSQL installed and running
- Access to create databases

## Step 1: Create Databases
```bash
# Connect to PostgreSQL
psql -U postgres

# Create databases
CREATE DATABASE payment_db;
CREATE DATABASE order_db;

# Exit psql
\q
```

## Step 2: Run Migrations

### Payment Service Database
```bash
psql -U postgres -d payment_db -f payment-service/migrations/001_payments.sql
```

### Order Service Database
```bash
psql -U postgres -d order_db -f order-service/migrations/001_orders.sql
psql -U postgres -d order_db -f order-service/migrations/002_idempotency.sql
```

## Step 3: Update Environment Variables

### Payment Service (.env)
```bash
PAYMENT_DATABASE_URL=postgres://postgres:password@localhost:5432/payment_db?sslmode=disable
PAYMENT_GRPC_ADDR=:50051
PAYMENT_HTTP_ADDR=:8081
```

### Order Service (.env)
```bash
ORDER_DATABASE_URL=postgres://postgres:password@localhost:5432/order_db?sslmode=disable
PAYMENT_GRPC_ADDR=localhost:50051
ORDER_GRPC_ADDR=:50052
ORDER_HTTP_ADDR=:8080
```

## Step 4: Verify Tables
```bash
# Check payment tables
psql -U postgres -d payment_db -c "\dt"

# Check order tables  
psql -U postgres -d order_db -c "\dt"
```

## Alternative: Using Docker
If you have Docker, you can use the provided setup:
```bash
cd docker
docker-compose up -d
```

This will create PostgreSQL instances and run the migrations automatically.
