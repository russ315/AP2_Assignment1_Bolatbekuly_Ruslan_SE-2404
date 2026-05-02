# AP2 Assignment 2 - Protocol Buffers Repository

This repository contains the Protocol Buffer definitions for the Order and Payment services in AP2 Assignment 2.

## Structure

- `payment.proto` - Payment service definitions
- `order.proto` - Order service definitions with streaming support
- `buf.yaml` - Buf configuration
- `buf.gen.yaml` - Code generation configuration

## Generated Code

The generated Go code is automatically published to: https://github.com/russ315/ap2-proto-gen

## Usage

Add to your service's `go.mod`:

```bash
go get github.com/russ315/ap2-proto-gen@v1.0.0
```

Import in your code:

```go
import (
    paymentv1 "github.com/russ315/ap2-proto-gen/gen/go/payment/v1"
    orderv1 "github.com/russ315/ap2-proto-gen/gen/go/order/v1"
)
```

## Services

### PaymentService
- `AuthorizePayment` - Process payment authorization
- `GetPaymentStatus` - Get payment status by order ID

### OrderService
- `SubscribeToOrderUpdates` - Server-side streaming for real-time order updates
