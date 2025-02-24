package cardano

type MessageLocalStateAcquire struct {
	WithSubprotocol
}

type MessageLocalStateAcquired struct {
	WithSubprotocol
}

type MessageLocalStateFailure struct {
	WithSubprotocol
}

type MessageLocalStateQuery struct {
	WithSubprotocol
	Query any
}
type MessageLocalStateResult struct {
	WithSubprotocol
	Result any
}

type MessageLocalStateRelease struct {
	WithSubprotocol
}

type MessageLocalStateReacquire struct {
	WithSubprotocol
}

type MessageLocalStateDone struct {
	WithSubprotocol
}

type MessageLocalStateAcquireVolatileTip struct {
	WithSubprotocol
}

type MessageLocalStateReacquireVolatileTip struct {
	WithSubprotocol
}

type MessageLocalStateAcquireImmutableTip struct {
	WithSubprotocol
}

type MessageLocalStateReacquireImmutableTip struct {
	WithSubprotocol
}
