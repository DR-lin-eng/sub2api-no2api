package provider

import (
	"errors"
	"net"
)

// A dial failure proves the refund request was not delivered. Errors after a
// connection was established can hide an accepted refund and must retain the
// hold until authoritative query/manual reconciliation.
func refundTransportUncertain(err error) bool {
	var response interface{ HTTPStatusCode() int }
	if errors.As(err, &response) && response.HTTPStatusCode() >= 400 && response.HTTPStatusCode() < 500 {
		return false
	}
	var operation *net.OpError
	return !errors.As(err, &operation) || operation.Op != "dial"
}
