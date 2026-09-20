package native

import "fmt"

// io_close.go — the polymorphic IO.close. Like boru:net's one closeHandler
// over Socket/Listener/Service, it closes whichever resource the argument
// holds: a File handle (IO.open) or a Watcher (IO.watch). IO.unwatch stays
// as the Watcher-specific alias. The dispatch is structural (each asXxx
// unwrap checks the payload's Go type), so a single TAny signature admits
// both resource types and a non-resource value is declined cleanly.
func doCloseWord(args []Value, r *Registry) ([]Value, error) {
	if fh, ok := asFileHandle(args[0]); ok {
		return closeResult(r, fh.Close())
	}
	if wi, ok := asWatcherInfo(args[0]); ok {
		return closeResult(r, wi.Stop())
	}
	if li, ok := asLockInfo(args[0]); ok {
		return closeResult(r, li.Release())
	}
	if mi, ok := asMmapInfo(args[0]); ok {
		return closeResult(r, mi.Close())
	}
	return nil, r.BoruError("close_error",
		fmt.Sprintf("close: not a closable handle (got %s)", args[0].Parent), "close")
}

// closeResult maps a resource's close error onto the close word's result.
func closeResult(r *Registry, err error) ([]Value, error) {
	if err != nil {
		return nil, r.BoruError("close_error", fmt.Sprintf("close: %v", err), "close")
	}
	return nil, nil
}
