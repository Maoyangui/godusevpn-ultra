package browserenginehost

import "context"

type failedStartupOwner interface {
	Close() error
	Status() Status
	Done() <-chan struct{}
}

// retainFailedStartup closes the partially assembled session on the startup
// goroutine. Any incomplete or contradictory close result must keep its owner
// visible to the caller, including an empty Job with unknown persistent filters.
func retainFailedStartup(owner failedStartupOwner) bool {
	if owner == nil {
		return false
	}
	if owner.Close() != nil {
		return true
	}
	status := owner.Status()
	if status.State != "closed" || status.Ready || status.CanNavigate {
		return true
	}
	select {
	case <-owner.Done():
		return false
	default:
		return true
	}
}

// Startup cancellation never closes lifetime concurrently with resource
// assignment. The same startup goroutine observes it at side-effect boundaries
// and lets its deferred cleanup own all resources already acquired.
func startupCheckpoint(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrRefused
	}
	return nil
}
