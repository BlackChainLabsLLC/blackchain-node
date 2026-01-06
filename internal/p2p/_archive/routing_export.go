package p2p

func (rt *RouteTable) RoutesToSend() []Route {
    // safe TTL-filtered broadcast copy
    return rt.Snapshot()
}

