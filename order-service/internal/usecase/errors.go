package usecase

import "errors"

var (
	ErrInvalidInput           = errors.New("invalid input")
	ErrOrderNotFound          = errors.New("order not found")
	ErrPaymentUnavailable     = errors.New("payment service unavailable")
	ErrCancelNotAllowed       = errors.New("only pending orders can be cancelled")
	ErrPaymentAlreadyRecorded = errors.New("payment already exists for order")
	ErrPaymentNotFound        = errors.New("payment not found for order")
	ErrPaymentInvalidArgument = errors.New("payment invalid argument")
)
