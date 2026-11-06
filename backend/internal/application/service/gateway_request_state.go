package service

import "hash/maphash"

type RequestBodyRef struct {
	data   []byte
	parsed *gatewayRequestBodyState
}

// NewRequestBodyRef borrows immutable data; replace it before reusing storage.
func NewRequestBodyRef(data []byte) *RequestBodyRef {
	return &RequestBodyRef{data: data}
}

// Bytes returns a read-only request view.
func (b *RequestBodyRef) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.data
}

func (b *RequestBodyRef) Len() int {
	if b == nil {
		return 0
	}
	return len(b.data)
}

func (b *RequestBodyRef) Replace(data []byte) {
	if b == nil {
		return
	}
	b.data = data
	b.parsed = nil
}

// gatewayRequestBodySeed keys body fingerprints for this process only.
var gatewayRequestBodySeed = maphash.MakeSeed()

// gatewayRequestBodyState is an immutable snapshot of a successful parse. It
// owns scalar strings and stores only offsets for large fields, so sharing it
// between attempts cannot retain an old request body or mutable request state.
type gatewayRequestBodyState struct {
	model, metadataUserID, outputEffort    string
	stream, hasSystem, thinkingEnabled     bool
	maxTokens                              int
	protocol                               string
	systemRange, messagesRange, inputRange jsonRange
	fingerprint                            uint64
}

func (p *ParsedRequest) saveBodyState() {
	p.Body.parsed = &gatewayRequestBodyState{
		model: p.Model, metadataUserID: p.MetadataUserID, outputEffort: p.OutputEffort,
		stream: p.Stream, hasSystem: p.HasSystem, thinkingEnabled: p.ThinkingEnabled, maxTokens: p.MaxTokens,
		protocol: p.protocol, systemRange: p.systemRange, messagesRange: p.messagesRange, inputRange: p.inputRange,
		fingerprint: maphash.Bytes(gatewayRequestBodySeed, p.Body.Bytes()),
	}
}

func (p *ParsedRequest) restoreBodyState(state *gatewayRequestBodyState) {
	p.Model, p.MetadataUserID, p.OutputEffort = state.model, state.metadataUserID, state.outputEffort
	p.Stream, p.HasSystem, p.ThinkingEnabled, p.MaxTokens = state.stream, state.hasSystem, state.thinkingEnabled, state.maxTokens
	p.protocol = state.protocol
	p.systemRange, p.messagesRange, p.inputRange = state.systemRange, state.messagesRange, state.inputRange
}

func (p *ParsedRequest) reusableBodyState(data []byte) *gatewayRequestBodyState {
	if p.Body == nil || p.Body.parsed == nil || p.Body.parsed.protocol != p.protocol {
		return nil
	}
	current := p.Body.Bytes()
	// Identity alone would trust the immutable borrowing contract. The content
	// fingerprint also catches in-place edits of shared storage, which then
	// reparse as before; it costs one hash pass instead of a full parse.
	if len(data) == 0 || len(data) != len(current) || &data[0] != &current[0] ||
		maphash.Bytes(gatewayRequestBodySeed, data) != p.Body.parsed.fingerprint {
		return nil
	}
	return p.Body.parsed
}
